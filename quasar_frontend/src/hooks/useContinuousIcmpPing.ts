import { useEffect, useRef, useState } from "react";
import { apiFetch } from "../lib/api";

export type IcmpPingTarget = {
  id: string;
  host: string;
  label?: string;
};

export type LatencySample = {
  t: number;
  ms: number | null;
  ok: boolean;
  error?: string;
};

export type ContinuousIcmpLatest = {
  ok: boolean;
  ms: number | null;
  error?: string;
  at: number;
};

type Opts = {
  targets: IcmpPingTarget[];
  /** Quando false, para o loop e limpa o histórico. */
  enabled: boolean;
  /** Intervalo mínimo entre o início de cada ciclo (ms). Default 1000. */
  minIntervalMs?: number;
  timeoutMs?: number;
  maxPoints?: number;
};

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException("Aborted", "AbortError"));
      return;
    }
    const t = window.setTimeout(() => resolve(), ms);
    const onAbort = () => {
      window.clearTimeout(t);
      reject(new DOMException("Aborted", "AbortError"));
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

async function pingOnce(host: string, timeoutMs: number): Promise<{ ok: boolean; ms: number | null; error?: string }> {
  try {
    const res = await apiFetch<{ ok?: boolean; rtt_ms?: number; error?: string }>("/api/v1/tools/icmp/ping", {
      method: "POST",
      json: { host, timeout_ms: timeoutMs },
      timeoutMs: timeoutMs + 1500,
    });
    const ok = res.ok === true;
    const ms = typeof res.rtt_ms === "number" && Number.isFinite(res.rtt_ms) ? res.rtt_ms : null;
    return { ok, ms: ok ? ms : null, error: ok ? undefined : res.error || "Sem resposta" };
  } catch (e) {
    return { ok: false, ms: null, error: e instanceof Error ? e.message : String(e) };
  }
}

/**
 * Ping ICMP contínuo no servidor, com intervalo mínimo entre ciclos e histórico para gráfico.
 * Cada ciclo pinge todos os alvos em paralelo.
 */
export function useContinuousIcmpPing({
  targets,
  enabled,
  minIntervalMs = 1000,
  timeoutMs = 3000,
  maxPoints = 90,
}: Opts) {
  const [series, setSeries] = useState<Record<string, LatencySample[]>>({});
  const [latest, setLatest] = useState<Record<string, ContinuousIcmpLatest>>({});
  const [running, setRunning] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);

  const targetsKey = targets.map((t) => `${t.id}:${t.host}`).join("|");
  const targetsRef = useRef(targets);
  targetsRef.current = targets;

  useEffect(() => {
    if (!enabled || targets.length === 0) {
      setRunning(false);
      setSeries({});
      setLatest({});
      setLastError(null);
      return;
    }

    const ac = new AbortController();
    const interval = Math.max(1000, minIntervalMs);
    const to = Math.min(15000, Math.max(500, timeoutMs));

    setRunning(true);
    setSeries({});
    setLatest({});
    setLastError(null);

    (async () => {
      while (!ac.signal.aborted) {
        const started = Date.now();
        const list = targetsRef.current.filter((t) => t.host.trim());
        if (list.length === 0) break;

        const results = await Promise.all(
          list.map(async (t) => {
            const r = await pingOnce(t.host.trim(), to);
            return { id: t.id, ...r };
          }),
        );
        if (ac.signal.aborted) break;

        const now = Date.now();
        setSeries((prev) => {
          const next = { ...prev };
          for (const r of results) {
            const sample: LatencySample = { t: now, ms: r.ms, ok: r.ok, error: r.error };
            const arr = [...(next[r.id] ?? []), sample];
            next[r.id] = arr.length > maxPoints ? arr.slice(arr.length - maxPoints) : arr;
          }
          return next;
        });
        setLatest((prev) => {
          const next = { ...prev };
          for (const r of results) {
            next[r.id] = { ok: r.ok, ms: r.ms, error: r.error, at: now };
          }
          return next;
        });
        const failed = results.find((r) => !r.ok && r.error);
        setLastError(failed?.error ?? null);

        const wait = Math.max(0, interval - (Date.now() - started));
        try {
          await sleep(wait, ac.signal);
        } catch {
          break;
        }
      }
      if (!ac.signal.aborted) setRunning(false);
    })();

    return () => {
      ac.abort();
      setRunning(false);
    };
  }, [enabled, targetsKey, minIntervalMs, timeoutMs, maxPoints]);

  return { series, latest, running, lastError };
}
