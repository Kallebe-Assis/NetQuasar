/** Apresentação de alertas resolvidos — duração, POP e valor normalizado. */

function metaObj(meta: unknown): Record<string, unknown> {
  if (meta == null) return {};
  if (typeof meta === "object" && !Array.isArray(meta)) return meta as Record<string, unknown>;
  if (typeof meta === "string") {
    try {
      const p = JSON.parse(meta) as unknown;
      if (p && typeof p === "object" && !Array.isArray(p)) return p as Record<string, unknown>;
    } catch {
      /* ignore */
    }
  }
  return {};
}

function numFmt(n: unknown, suffix = ""): string | null {
  const x = typeof n === "number" ? n : Number(n);
  if (!Number.isFinite(x)) return null;
  const s = Number.isInteger(x) ? String(x) : x.toFixed(1);
  return `${s}${suffix}`;
}

export function formatAlertDuration(activeSince?: string | null, closedAt?: string | null): string {
  if (!activeSince || !closedAt) return "—";
  const a = new Date(activeSince).getTime();
  const b = new Date(closedAt).getTime();
  if (!Number.isFinite(a) || !Number.isFinite(b) || b < a) return "—";
  let sec = Math.floor((b - a) / 1000);
  const days = Math.floor(sec / 86400);
  sec %= 86400;
  const hours = Math.floor(sec / 3600);
  sec %= 3600;
  const mins = Math.floor(sec / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days} d`);
  if (hours > 0) parts.push(`${hours} h`);
  if (mins > 0 || parts.length === 0) parts.push(`${mins} min`);
  return parts.join(" ");
}

export function formatAlertResolvedValue(
  type: string | null | undefined,
  meta: unknown,
  message?: string | null,
): string {
  const m = metaObj(meta);
  const t = String(type ?? "").trim();

  const direct = m.resolved_value ?? m.resolved_value_text;
  if (direct != null && String(direct).trim() !== "") return String(direct).trim();

  const verify = metaObj(m.verify);
  const collected = metaObj(m.collected);

  if (t === "latency_high" || verify.latency_ms != null || collected.latency_ms != null) {
    const v = numFmt(verify.latency_ms ?? collected.latency_ms, " ms");
    if (v) return v;
  }

  if (verify.dbm != null || m.dbm != null) {
    const v = numFmt(verify.dbm ?? m.dbm, " dBm");
    if (v) return v;
  }

  const metricVal = verify.value ?? collected.value ?? m.value;
  const metricId = String(verify.metric_id ?? collected.metric_id ?? m.metric_id ?? "").toLowerCase();
  if (metricVal != null) {
    if (metricId.includes("cpu") || t.includes("cpu")) {
      const v = numFmt(metricVal, "%");
      if (v) return v;
    }
    if (metricId.includes("mem") || t.includes("memory")) {
      const v = numFmt(metricVal, "%");
      if (v) return v;
    }
    if (metricId.includes("temp") || t.includes("temperature")) {
      const v = numFmt(metricVal, " °C");
      if (v) return v;
    }
    const v = numFmt(metricVal);
    if (v) return v;
  }

  if (verify.uptime_minutes != null) {
    const v = numFmt(verify.uptime_minutes, " min");
    if (v) return v;
  }

  if (t === "ping_unreachable") {
    if (verify.reach_ok === true || collected.reach_ok === true || (verify.probe as { ok?: boolean })?.ok) {
      return "Online";
    }
  }

  const msg = String(message ?? "");
  const mLat = msg.match(/(\d+(?:[.,]\d+)?)\s*ms/i);
  if (mLat && (t === "latency_high" || t === "latency_degraded")) return `${mLat[1].replace(",", ".")} ms`;
  const mDbm = msg.match(/(-?\d+(?:[.,]\d+)?)\s*dBm/i);
  if (mDbm) return `${mDbm[1].replace(",", ".")} dBm`;
  const mPct = msg.match(/(\d+(?:[.,]\d+)?)\s*%/);
  if (mPct && (t.includes("cpu") || t.includes("memory") || t === "telemetry_threshold")) {
    return `${mPct[1].replace(",", ".")}%`;
  }

  return "—";
}

export function formatAlertPopName(popName?: string | null, meta?: unknown): string {
  const p = String(popName ?? "").trim();
  if (p) return p;
  const m = metaObj(meta);
  const fromMeta = String(m.pop_name ?? m.pop ?? "").trim();
  return fromMeta || "—";
}
