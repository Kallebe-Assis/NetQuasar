import type { LucideIcon } from "lucide-react";

export function RingGauge({
  pct,
  label,
  sub,
  color = "var(--accent)",
  size = 88,
}: {
  pct: number | null;
  label: string;
  sub?: string;
  color?: string;
  size?: number;
}) {
  const v = pct != null && Number.isFinite(pct) ? Math.min(100, Math.max(0, pct)) : null;
  const r = 36;
  const c = 2 * Math.PI * r;
  const dash = v != null ? (v / 100) * c : 0;
  return (
    <div className="mk-noc-gauge">
      <svg width={size} height={size} viewBox="0 0 88 88">
        <circle cx="44" cy="44" r={r} fill="none" stroke="var(--border)" strokeWidth="8" />
        <circle
          cx="44"
          cy="44"
          r={r}
          fill="none"
          stroke={color}
          strokeWidth="8"
          strokeLinecap="round"
          strokeDasharray={`${dash} ${c}`}
          transform="rotate(-90 44 44)"
        />
        <text x="44" y="48" textAnchor="middle" fill="currentColor" fontSize="16" fontWeight="700">
          {v != null ? `${Math.round(v)}%` : "—"}
        </text>
      </svg>
      <div className="mk-noc-gauge__meta">
        <span className="mk-noc-gauge__label">{label}</span>
        {sub ? <span className="mk-noc-gauge__sub mono">{sub}</span> : null}
      </div>
    </div>
  );
}

export function KpiCard({
  icon: Icon,
  title,
  children,
  accent,
  headAction,
}: {
  icon: LucideIcon;
  title: string;
  children: React.ReactNode;
  accent?: string;
  headAction?: React.ReactNode;
}) {
  return (
    <div className="mk-noc-kpi" style={accent ? { borderColor: accent } : undefined}>
      <div className="mk-noc-kpi__head">
        <Icon size={14} style={{ opacity: 0.85 }} />
        <span style={{ flex: 1 }}>{title}</span>
        {headAction}
      </div>
      <div className="mk-noc-kpi__body">{children}</div>
    </div>
  );
}

export function parsePercentValue(raw: unknown): number | null {
  if (raw == null || raw === "") return null;
  const n = typeof raw === "number" ? raw : Number(String(raw).replace(/[^\d.-]/g, ""));
  if (!Number.isFinite(n)) return null;
  if (n >= 0 && n <= 100) return n;
  if (n > 100 && n <= 1000) return n / 10;
  return n;
}
