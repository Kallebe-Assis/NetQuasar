# NetQuasar Mobile — plano de execução (PWA)

Documento de trabalho para transformar o NetQuasar em um **PWA instalável** (Android primeiro, iPhone também), com
**notificação push de alertas** e uma interface pensada para o celular. Cada fase tem um checklist: marque `[x]` ao concluir
e anote o que mudou na seção «Diário de execução» no fim.

- **Quem usa:** o dono e a equipe (NOC, suporte, campo). Não é para clientes.
- **Plataformas:** Android = principal; iPhone = suportado (com as limitações do iOS, ver §6).
- **Endereço público:** `https://netquasar.origemdigital.com.br/` (HTTPS já existe — requisito do PWA e do push).
- **Prioridade dada pelo dono:** o **push de alertas é essencial**. Ordem das telas: **Monitoramento → Alertas →
  Equipamentos → Dashboard → Configurações → Integrações → Ferramentas → Mapa**.

---

## 1. Ponto de partida (o que já existe)

| Item | Estado |
|------|--------|
| PWA | **Não existe de verdade.** Hoje é só um atalho do navegador: não há `manifest`, service worker nem ícones de app. |
| Hospedagem | A SPA é embutida no binário Go (`internal/embedui`), servida com fallback para `index.html`. Arquivos fora de `/assets/` saem com `no-cache` — bom para `sw.js` e `manifest`. |
| Responsividade | Existe `styles/responsive.css` (1105 linhas) e `lib/mobileTables.ts` (tabelas viram cartões ≤ 767 px), mas a interface é **desktop-first** (44 páginas, 224 componentes). |
| Menu | `app/navConfig.ts` + `app/ShellLayout.tsx` (menu lateral com submódulos). |
| Alertas | Disparo central em `alertnotify.SendMonitoringTelegramAndPatchMeta` / `SendResolutionTelegramAndPatchMeta` (Telegram). No navegador, `AlertNotificationWatcher.tsx` consulta a API e mostra toast/som **só com a página aberta**. |
| Preferências por usuário | `users.preferences` (`/api/v1/me/preferences`): tema, toast, som. |

**Conclusão:** o PWA resolve instalação, tela cheia e push; **não** conserta a interface — por isso o plano separa
«infra PWA + push» (Fases 1–2) de «interface mobile» (Fases 3–4), que é a maior parte do trabalho.

---

## 2. Decisões de arquitetura

1. **Mesmo projeto, mesmo código.** O PWA é a própria SPA React (`quasar_frontend`), sem app nativo e sem segundo repositório. A atualização chega junto com o `docker compose` de sempre.
2. **Casca mobile dedicada, não "responsivizar tudo".** Em telas ≤ 767 px o app usa uma **casca mobile** (barra inferior, cabeçalho simples, listas em cartões). No desktop **nada muda**. Telas complexas (BGP, topologia…) só ganham a casca; as de uso diário ganham layout próprio.
3. **Service worker escrito à mão** (sem dependência nova), com regras simples e seguras:
   - `/api/*` **nunca** entra em cache (dados ao vivo);
   - `/assets/*` (nome com hash) → cache-first;
   - navegação → rede primeiro; sem rede, mostra a casca em cache + aviso «sem conexão».
4. **Atualização controlada.** O SW novo espera; o app mostra «Nova versão disponível — Atualizar» (evita tela quebrada com JS antigo + API nova).
5. **Push com Web Push (VAPID)**, enviado pelo backend Go — sem Firebase/terceiros. Um novo pacote `pushnotify` é chamado nos **mesmos pontos** em que o Telegram já é disparado (§5).
6. **Permissões respeitadas.** Push só para quem tem acesso a alertas; filtros por severidade por usuário.

---

## 3. Fases

Legenda: ☐ a fazer · ☑ feito.

### Fase 1 — Base PWA (instalável) ☑

Objetivo: o Chrome/Android oferece «Instalar app»; abre em tela cheia, com ícone próprio, e sobrevive a falta de rede mostrando uma tela clara.

- [x] `public/manifest.webmanifest` (nome, cores, `display: standalone`, ícones, atalhos para Alertas / Monitoramento / OLT / Consulta HubSoft)
- [x] Ícones gerados do logo: 192, 512, **maskable** 512 e `apple-touch-icon` 180 (`public/pwa/`)
- [x] `index.html`: `<link rel="manifest">`, `apple-touch-icon`, `apple-mobile-web-app-title`, status bar do iOS
- [x] `public/sw.js` (cache de casca/estáticos, nunca `/api`) + `public/offline.html`
- [x] `src/pwa/registerServiceWorker.ts` + aviso «Nova versão disponível»
- [x] Banner/botão «Instalar app» (Android: `beforeinstallprompt`; iPhone: instruções «Compartilhar → Adicionar à Tela de Início») em `src/pwa/`
- [x] `embedui`: `Content-Type: application/manifest+json` para `.webmanifest`
- [x] Verificação: build, manifest e SW servidos, registro do SW no navegador

**Para testar no celular (depois de reconstruir a imagem):** abra `https://netquasar.origemdigital.com.br/` no Chrome do Android, faça login, e use o banner «Instalar» ou o menu ⋮ → **Instalar app** (apague antes o atalho antigo da tela inicial). Para ver o aviso de atualização, publique uma segunda versão e reabra o app.

**Aceite:** no Android, menu do Chrome mostra «Instalar app»; instalado, abre sem barra do navegador; o ícone é o do NetQuasar; desligar a rede mostra a tela offline em vez de erro do navegador.

### Fase 2 — Notificação push de alertas ☐ (essencial)

Objetivo: alerta novo/resolvido chega no celular **mesmo com o app fechado**.

Backend
- [ ] Migração `156_push_subscriptions.sql`: `push_subscriptions` (`id`, `user_id`, `endpoint` único, `p256dh`, `auth`, `user_agent`, `created_at`, `last_ok_at`, `last_error`, `disabled_at`) e preferências de push por usuário (severidades, resolvidos on/off, silêncio noturno opcional)
- [ ] Chaves **VAPID**: gerar uma vez, guardar em `.env` (`NETQUASAR_VAPID_PUBLIC_KEY`, `NETQUASAR_VAPID_PRIVATE_KEY`, `NETQUASAR_VAPID_SUBJECT=mailto:…`); sem chaves → push desligado, sistema segue normal
- [ ] Pacote `internal/pushnotify` (biblioteca `webpush-go`): `Send(ctx, userIDs, payload)`, remove inscrições mortas (HTTP 404/410), nunca bloqueia o ciclo de alertas (fila com timeout)
- [ ] Rotas: `GET /api/v1/push/public-key`, `POST /api/v1/push/subscribe`, `DELETE /api/v1/push/subscribe`, `POST /api/v1/push/test` (autenticadas)
- [ ] Gancho nos pontos centrais de alerta (`alertnotify.Send*TelegramAndPatchMeta`): novo alerta → push para os usuários elegíveis (permissão de alertas + severidade ≥ preferência); resolução → push opcional
- [ ] Testes de unidade (montagem do payload, filtro por severidade, limpeza de inscrições inválidas)

Frontend
- [ ] `sw.js`: tratar `push` (título, corpo, ícone, `tag` por alerta para não duplicar) e `notificationclick` (abre `/alerts` no alerta certo)
- [ ] Configurações → Alertas → **«Notificações no celular»**: ativar/desativar neste aparelho, botão «Enviar teste», severidades e resolvidos
- [ ] Tratamento de permissão negada/não suportada (mensagem clara, sem quebrar)
- [ ] Badge no ícone (contador de alertas abertos) onde o sistema suportar

**Aceite:** com o app **fechado**, um alerta crítico gera notificação no Android em poucos segundos; tocar nela abre o alerta. No iPhone funciona com o app **instalado na Tela de Início** (iOS ≥ 16.4).

### Fase 3 — Casca mobile ☐

Objetivo: navegar bem no celular, sem tocar nas telas internas ainda.

- [ ] Detecção de «modo celular» (≤ 767 px ou PWA standalone em tela estreita) — hook `useIsMobileShell`
- [ ] `MobileShell`: cabeçalho enxuto (título da tela, busca, sino de alertas com contador) + **barra inferior** com 5 atalhos: Monitoramento · Alertas · Equipamentos · Dashboard · **Mais** (abre a lista completa: Configurações, Integrações, Ferramentas, Mapa…)
- [ ] «Mais» reaproveita `navConfig.ts` (submódulos continuam a existir), com busca
- [ ] Áreas seguras do iPhone/Android (`env(safe-area-inset-*)`), alvos de toque ≥ 44 px, sem zoom acidental em campos
- [ ] Componentes base reutilizáveis: `MobileCard`, `MobileList`, `StatusPill`, `BottomSheet` (filtros/ações), cabeçalho fixo com filtros recolhíveis
- [ ] Pull-to-refresh nas listas ao vivo

**Aceite:** todas as rotas abrem dentro da casca mobile, sem rolagem horizontal da página, com navegação por uma mão.

### Fase 4 — Telas por prioridade ☐

Cada tela entra pronta (cartões, filtros em `BottomSheet`, ações com confirmação) e só então a próxima. Para cada uma: revisar no celular real (Android), tema claro/escuro, rede lenta.

| # | Tela | Foco no celular |
|---|------|-----------------|
| 1 | **Monitoramento** | Resumo por severidade no topo, lista de equipamentos/itens em cartões com estado colorido, filtro rápido (só com problema) |
| 2 | **Alertas** | Lista por severidade com «reconhecer / ignorar / verificar» por gesto ou menu; detalhe em tela cheia; abrir direto pelo toque na notificação |
| 3 | **Equipamentos** (MikroTik, OLT, Switch, BNG, BGP) | Visão geral em cartões; OLT: pesquisa de ONU por serial, PON, sinal; botão «Atualizar PON» |
| 4 | **Dashboard** | Indicadores empilhados, gráficos leves, atualização manual + automática |
| 5 | **Configurações** | Lista de seções → tela de cada seção; formulários em coluna única |
| 6 | **Integrações** (HubSoft, consulta de cliente) | Busca de cliente com resultado em cartão; abas Financeiro / Atendimentos / O.S. como seções; relatórios em modo resumo |
| 7 | **Ferramentas** (todas as abas) | Uma ferramenta por tela; resultado em texto/cartões; copiar resultado |
| 8 | **Mapa** | Mapa em tela cheia, filtros em `BottomSheet`, localização do usuário, toque abre o detalhe do ponto |
| — | **Localidades e POPs** | Entram junto com «Equipamentos» (uso frequente); lista em cartões |

**Aceite por tela:** cabe em 360 px de largura, sem rolagem horizontal, ações principais ao alcance do polegar, estado de carregando/vazio/erro claros.

### Fase 5 — Acabamento ☐

- [ ] Modo offline útil: última tela de Alertas/Dashboard visível «sem conexão» (somente leitura, marcada como desatualizada)
- [ ] Desempenho: carregar só o necessário no celular (já há rotas com `lazy`), medir com Lighthouse (alvo PWA ≥ 90, desempenho ≥ 70 em 4G)
- [ ] Acessibilidade e `prefers-reduced-motion`
- [ ] Guia curto para a equipe: como instalar (Android / iPhone) e ativar as notificações

---

## 4. Como cada fase é entregue e testada

1. Alterar o código → `npx tsc --noEmit -p .` e `npm run build` (o projeto não tem ESLint: conferir a **ordem dos hooks** manualmente) → `go build ./... && go test ./internal/...` quando houver backend.
2. Reconstruir a imagem e reiniciar o container (a UI é embutida no binário; mudança de código só vai ao ar depois disso).
3. Testar no celular **Android real** pelo domínio público (o service worker só funciona em HTTPS ou `localhost`) e depois no iPhone.
4. Ao mudar o service worker: conferir em **DevTools → Application → Service Workers** (ou `chrome://serviceworker-internals`) que a versão nova aparece e o aviso «Nova versão» funciona.

**Reverter o PWA** se algo der errado: publicar um `sw.js` que se desregistra (`self.registration.unregister()`); como `sw.js` sai com `no-cache`, os aparelhos o recebem na próxima abertura.

---

## 5. Desenho do push (Fase 2)

```
alerta novo/resolvido ──► alertnotify (Telegram, já existe) ──┐
                                                               ├─► pushnotify.Dispatch(alerta)
                                                               │      ├─ usuários com permissão de alertas
                                                               │      ├─ filtro de severidade / resolvidos / silêncio
                                                               │      └─ webpush-go (VAPID) → FCM (Android) / Apple (iOS) / Mozilla
navegador: sw.js  ◄── evento "push" ──► showNotification(título, corpo, tag=alerta) ──► clique abre /alerts?...
```

- **Conteúdo do payload:** título (severidade + tipo), corpo (equipamento, IP, valor), `tag` = id do alerta (atualização substitui a notificação em vez de empilhar), `url` de destino. Sem dados sensíveis além do que o Telegram já envia.
- **Inscrição** vinculada ao usuário **e ao aparelho** (`endpoint`); sair da conta ou desativar remove a inscrição.
- **Falhas:** HTTP 404/410 do serviço de push → inscrição apagada; outros erros → registrar em `last_error` e tentar de novo só no próximo alerta.
- **Volume:** alertas em rajada são agrupados (ex.: «5 novos alertas críticos») para não inundar o celular.

---

## 6. Limitações e cuidados

- **iPhone:** push só funciona com o app **adicionado à Tela de Início** (iOS ≥ 16.4), e a permissão precisa ser pedida por um toque do usuário. Não há instalação automática: o app mostra o passo a passo.
- **Android:** economia de bateria agressiva (Xiaomi, Samsung…) pode atrasar o push; incluir no guia «desativar otimização de bateria para o Chrome/NetQuasar».
- **Cache:** nunca guardar respostas de `/api` no service worker; senão o NOC veria alerta antigo achando que é atual.
- **Segurança:** o push não substitui o login; abrir a notificação exige sessão válida. Chaves VAPID privadas ficam só no servidor (`.env`), nunca no repositório.
- **Telas muito densas** (topologia, BGP completo) continuam melhores no computador; no celular ficam em modo resumo.

---

## 6b. Guia de estilos (para evitar conflitos de interface)

**Ordem e responsabilidade dos arquivos CSS** (a ordem de importação em `main.tsx` É a ordem da cascata — o último ganha em empate):

| Arquivo | Papel | Mexa aqui quando… |
|---------|-------|--------------------|
| `themes.css` | Tokens (cores, sombras) claro/escuro | mudar uma cor do sistema inteiro |
| `global.css` | Componentes base (desktop) | criar/alterar um componente |
| `responsive.css` | Adaptação ≤ 1023 px / ≤ 767 px dos componentes base | um componente quebra em tela estreita |
| `accent-skin.css` | **Destaque azul** em todas as larguras (`--accent-grad`, abas em pílula, menu, botão primário, avisos) | mudar o «look» (não a estrutura) |
| `mobile-skin.css` | Só celular: cartões achatados, chips rolantes, menu de filtros em folha | ajuste exclusivo do celular |
| `pwa.css` | Avisos de instalar/atualizar o app | — |

**Regras**
1. Cor só por token (`var(--accent)`, `var(--ok)`…); nunca hex solto — senão o tema escuro/claro quebra.
2. Mesma regra em dois arquivos = bug futuro. Se `accent-skin.css` já define (ex.: rolagem das abas), apague a cópia dos outros.
3. Estilo inline (`style={{ padding: 14 }}`) só perde para `!important` — nas peles móveis isso é usado de propósito e comentado.
4. Barra de abas nova: use a classe `.tabs` (botões **ou** links `<a>`) — ela já rola na horizontal e destaca a ativa; nunca `flex-wrap: wrap` para abas.
5. Estado da aba/tela que o menu lateral controla (`?tab=`) deve vir **da URL** (`useTabSearchParam`), nunca de um `useState` inicial — senão a tela não acompanha o menu.
6. Camadas (`z-index`): aviso do app 45 · modais 50–60 · menu lateral 95–100 · barra superior 110 · folha de filtros 120 · toast 140 · mapa 1000 · menus flutuantes 10050.

---

## 7. Diário de execução

| Data | Fase | O que foi feito |
|------|------|-----------------|
| 2026-10-07 | — | Plano criado. |
| 2026-10-07 | 1 | Verificado no navegador (build de produção): manifest servido como `application/manifest+json`, ícones 200, SW ativo e controlando a página, `/api` fora do cache, `sw.js` com `no-cache`, tela «Sem conexão» exibida com o servidor desligado. Base PWA: manifest, ícones, `sw.js`, `offline.html`, registro com aviso de atualização, banner de instalação, MIME do manifest no `embedui`. |
| 2026-10-07 | 1 | Revisão visual/código da Fase 1: aviso do PWA passou para `z-index: 45` (abaixo de modais, menu e toasts), botões do aviso com fonte 14 px / 40 px de altura (o `.btn--sm` global é 11 px), layout testado em 320/375 px nos dois temas sem rolagem horizontal; cache de estáticos limitado a 120 arquivos; registro do SW robusto se `load` já passou; ouvinte `beforeinstallprompt` movido para a abertura do app; favicon trocado de 1,3 MB para o ícone de 34 KB. |
| 2026-10-08 | 3 (adiantado) | **Densidade compacta no celular** após os prints do dono (fonte/botões grandes, abas em 3 linhas, cartões longos). Causa: o 2º bloco `@media (max-width:767px)` de `responsive.css` desfazia o 1º (botões/campos 40 px, campos 16 px, body 13 px). Agora: botões 34 px (30 px os `btn--sm`), campos 13 px (16 px só no iOS, por causa do zoom), body 12 px, abas em **uma linha com rolagem**, indicadores `.stat` 3 por linha, cartões de tabela com borda única e **recolhidos** (título + 4 campos; faixa «Ver mais» expande — `data-card-keep` na `<table>` muda o número), setas de ordenação `↕` fora dos rótulos. Pendente: revisar tela a tela (Equipamentos, Integrações, ONUs, BGP aninhado) e a casca com barra inferior. |
| 2026-10-08 | 3 (adiantado) | **Rodada 2 de compactação.** (1) Textos explicativos somem no celular: classes de descrição/dica (`.tools-panel__desc`, `.hsa-step__hint`, `.hsa-panel__sub`…) por CSS e parágrafos «muted» longos (≥ 100 caracteres, sem botão/link, fora de modal/aviso) marcados por `lib/mobileHints.ts` (`data-mobile-keep` mantém um texto). (2) Cartões com ações (opções, atualizar ONU…): título à esquerda e botões de 28 px à direita **na mesma linha**; o cartão deixa de crescer por causa dos botões. (3) Indicadores (`.stat`, `.mk-noc-kpi-row` do BNG/MikroTik) em grade de **2 colunas** — antes o CSS forçava 1 coluna por causa do `grid-template-columns` inline. |
| 2026-10-08 | 3 (adiantado) | **Rodada 3 + pele azul** (`styles/mobile-skin.css`, só ≤ 767 px). Correções dos prints: cartões aninhados do BGP/BNG achatados (paddings inline com `!important`, painel que só embrulha cartões sem moldura), menu de filtros do Alertas vira folha na base da tela (saía pela esquerda), filtros do Eventos em 2 colunas, gráficos vazios sem altura fixa, barras com 3+ botões pequenos (períodos) viram **chips numa linha rolável**. Pele inspirada no modelo aprovado: abas em pílulas (ativa em gradiente azul), botão primário em gradiente azul, avisos informativos em azul claro, topo com fio azul, indicadores Online (verde) / Offline (vermelho) na OLT (`stat--ok` / `stat--err`). **Próximo:** ícones nas abas, pílula «Monitoramento ativo» no topo, barra de navegação inferior (Fase 3 completa). |
| 2026-10-08 | 3 (adiantado) | **Destaque azul também na web** (`styles/accent-skin.css`, todas as larguras; `mobile-skin.css` ficou só com o que é do celular). Token `--accent-grad` (claro e escuro, com contraste do texto `--on-accent` nos dois). Tela atual no menu lateral e sublink ativo em azul cheio; abas em pílulas com a ativa azul (`.tabs`, `.conn-tabs`, `.mon-cfg__tab`, abas de automações, cabeçalho HubSoft, Sobre); botão primário em gradiente; foco dos campos, caixas de seleção, avisos informativos, filete no título da página e no topo dos indicadores, indicadores verde/vermelho (`stat--ok`/`stat--err`), cabeçalho de tabela e seleção de texto com o mesmo azul. |
| 2026-10-08 | 3 (adiantado) | **Rodada 4.** (1) Tags de estado da integração (`Ativa`, `Teste OK`, `Sessão ativa`…) no canto direito, na MESMA linha do nome — cartão da lista (`IntegrationsHubPage`) e cabeçalho (`IntegrationNav` ganhou a prop `badges`). (2) Barras de abas (`.tabs`, cabeçalho HubSoft, `.hubsoft-tabs`, `.hsa-tabs`) = **uma linha com rolagem horizontal**, sem grade nem quebra, na web e no celular; `lib/scrollTabs.ts` centraliza a aba ativa. Configurações: `OverflowTabs` mostra TODAS as abas rolando em tela ≤ 1023 px (o «···» fica só na web larga) e as 15 abas agora aparecem **agrupadas por divisor** (Sistema · Monitoramento · Equipamentos · Frota). (3) Menu lateral: o botão de grupo (Equipamentos/Mapa/Frota) não recebia as medidas compactas de `.sidebar a` no celular (fonte maior/desalinhado) — agora compartilha as mesmas regras. |
| 2026-10-08 | 3 (adiantado) | **Rodada 5 + auditoria de conflitos.** (1) **Bug:** em Configurações, trocar de aba pelo menu lateral mudava a URL mas não a tela — a aba era um `useState` inicial; agora é derivada de `?tab=` (a página não remonta ao navegar entre submódulos). Auditadas as demais páginas com `?tab=`: BGP/BNG/OLT/POPs/Ferramentas usam `useTabSearchParam`, Conexões e Frota já reagem à URL, Config HubSoft deriva de `?aba=`. (2) **Abas de Configurações vazando pela margem:** a linha invisível de medição do `OverflowTabs` não tinha o estilo das abas (padding/borda), subestimando a largura; agora usa a classe `.tabs` e a posição real de cada aba (inclui divisores), recalcula com a fonte carregada e o «···» vira «Outras telas (N)». Testado com o componente real a 1280 px (10 abas + «Outras telas (5)», sem vazamento) e a 1000 px (rolagem). (3) **Conflitos corrigidos:** abas feitas de `<a>` (IntegrationNav) agora também são pílulas; filete do título acompanha o título centralizado da HubSoft; anel de foco azul não desenha quadrado em checkbox/radio; regras duplicadas de abas removidas de `responsive.css` (fonte única em `accent-skin.css`); sombra do chip ativo não é mais cortada pela rolagem. |
| 2026-10-08 | — | **Nova tela de login** (modelo aprovado): layout dividido — à esquerda marca, promessa, 3 destaques e ilustração animada de fibras ópticas (`components/login/LoginFiberArt.tsx`, canvas leve: fibras que balançam, pulsos de luz, pontas que respiram, partículas; pausa fora da tela/aba escondida; `prefers-reduced-motion` = quadro parado); à direita cartão de vidro com borda em gradiente que se desloca, logo flutuante com halo, campos com ícone (e-mail/cadeado) e olho para mostrar a senha, botão roxo com reflexo, «Esqueceu a senha?» (orienta a pedir ao admin — não há fluxo de redefinição por e-mail) e «Manter-me conectado». Animações de entrada escalonadas. ≤ 960 px a apresentação some e fica só o cartão. **«Manter-me conectado»** é real: marcado = token no `localStorage` (como sempre foi, é o padrão); desmarcado = `sessionStorage` (sai ao fechar o navegador) — `auth.ts` (`getAuthToken`/`saveAuthToken(token, remember)`/`clearSession`). Estilos em `styles/login.css`. |
