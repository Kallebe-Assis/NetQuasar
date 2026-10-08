/* NetQuasar — service worker (PWA).
 *
 * Regras (simples de propósito):
 *  - /api/* e qualquer coisa que não seja GET do mesmo domínio NUNCA passam pelo cache: alerta/ONU/cliente são dados ao vivo.
 *  - /assets/* (nomes com hash do Vite, imutáveis) → cache primeiro.
 *  - ícones e logos → cache primeiro (mudam raramente).
 *  - navegação (abrir/recarregar uma tela) → rede primeiro; sem rede, mostra /offline.html.
 *  - Atualização controlada: um SW novo ESPERA; o app mostra «Nova versão disponível» e só então manda SKIP_WAITING.
 *
 * Para desligar o PWA em emergência: publique um sw.js que chame self.registration.unregister() (este arquivo sai com no-cache).
 */
const VERSION = "nq-pwa-v1";
const STATIC_CACHE = VERSION + "-static";
const SHELL_CACHE = VERSION + "-shell";
const SHELL_URLS = ["/offline.html", "/pwa/icon-192.png"];
// teto de arquivos no cache de estáticos: cada deploy gera /assets/*.js novos; os mais antigos são descartados
const STATIC_MAX_ENTRIES = 120;

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(SHELL_CACHE).then((c) => c.addAll(SHELL_URLS)));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keep = new Set([STATIC_CACHE, SHELL_CACHE]);
      for (const name of await caches.keys()) {
        if (name.startsWith("nq-pwa-") && !keep.has(name)) await caches.delete(name);
      }
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("message", (event) => {
  if (event.data && event.data.type === "SKIP_WAITING") self.skipWaiting();
});

function isCacheFirst(url) {
  return url.pathname.startsWith("/assets/") || url.pathname.startsWith("/pwa/") || url.pathname.startsWith("/brand-logos/");
}

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname === "/health" || url.pathname === "/metrics") return;
  if (req.headers.has("range")) return;

  if (isCacheFirst(url)) {
    event.respondWith(
      caches.open(STATIC_CACHE).then(async (cache) => {
        const hit = await cache.match(req);
        if (hit) return hit;
        const res = await fetch(req);
        if (res.ok) {
          await cache.put(req, res.clone());
          const keys = await cache.keys(); // ordem de inserção
          for (let i = 0; i < keys.length - STATIC_MAX_ENTRIES; i++) await cache.delete(keys[i]);
        }
        return res;
      }),
    );
    return;
  }

  if (req.mode === "navigate") {
    event.respondWith(
      fetch(req).catch(async () => (await caches.match("/offline.html", { cacheName: SHELL_CACHE })) || Response.error()),
    );
  }
});
