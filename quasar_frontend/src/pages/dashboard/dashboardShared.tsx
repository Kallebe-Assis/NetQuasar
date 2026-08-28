import type { ReactElement, ReactNode } from "react";
import { ResponsiveContainer } from "recharts";
import { DeferredMount } from "../../components/DeferredMount";
import { InfoHint } from "../../components/InfoHint";

export const CHART_COLORS = ["#58a6ff", "#3fb950", "#d29922", "#f85149", "#a371f7", "#79c0ff", "#ff7b72", "#56d364", "#ffa657"];

export type TopRow = { device_id: string; description: string; ip?: string | null; latency_ms: number };

export type DashboardTotals = {
  devices?: number;
  pops?: number;
  commercial_clients_sum?: number;
  monitoring_running?: boolean;
  telemetry_enabled_devices?: number;
  ping_enabled_devices?: number;
};

export type NamedCount = {
  category?: string;
  network_status?: string;
  operational_mode?: string;
  count?: number;
  pop_name?: string;
  locality_name?: string;
  alert_type?: string;
};

export type LatRank = { device_id?: string; description?: string; avg_latency_ms?: number; samples?: number };
export type OltOnu = {
  device_id?: string;
  description?: string;
  onu_count?: number;
  onu_online?: number;
  onu_offline?: number;
  brand?: string;
  snapshot_at?: string;
};

export type DashboardAnalytics = {
  generated_at?: string;
  days?: number;
  since?: string;
  totals?: DashboardTotals;
  devices_by_category?: NamedCount[];
  devices_by_network_status?: NamedCount[];
  devices_by_operational_mode?: NamedCount[];
  devices_by_pop?: Array<{ pop_id?: string; pop_name?: string; count?: number }>;
  devices_by_locality?: Array<{ locality_id?: string; locality_name?: string; count?: number }>;
  ping_ranking_worst_latency?: LatRank[];
  ping_ranking_best_latency?: LatRank[];
  telemetry_window?: { samples?: number };
  alerts_by_type_30d?: NamedCount[];
  alerts_open?: number;
  olt_onu_by_device?: OltOnu[];
  olt_onu_fleet_totals?: { onu_count?: number; onu_online?: number; onu_offline?: number };
};
export type OltCapacityPON = {
  olt_id: string;
  olt: string;
  pon_id: string;
  onu_total: number;
  usage_percent: number;
  near_saturation: boolean;
};
export type OltCapacity = {
  olt_rows: Array<{ olt_id: string; olt: string; onu_total: number; pon_count: number; near_saturation_pons: number; snapshot_at: string }>;
  pon_rows: OltCapacityPON[];
  trend_7d: Array<{ day: string; onu_total: number }>;
};

export function num(n: unknown): number {
  const x = Number(n);
  return Number.isFinite(x) ? x : 0;
}

export function fmtInt(n: unknown): string {
  const x = num(n);
  return new Intl.NumberFormat("pt-PT").format(Math.round(x));
}

export function fmt1(n: unknown): string {
  const x = num(n);
  return Number.isFinite(x) ? x.toFixed(1) : "—";
}

export function trunc(s: string, max = 22): string {
  const t = String(s ?? "").trim();
  return t.length <= max ? t : `${t.slice(0, max - 1)}…`;
}

export const tooltipStyle = {
  backgroundColor: "var(--panel2)",
  border: "1px solid var(--border)",
  borderRadius: "var(--radius)",
  fontSize: 12,
  color: "var(--text)",
};

export function Section({
  id,
  title,
  subtitle,
  children,
}: {
  id: string;
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="card" style={{ marginTop: 16, padding: "14px 16px 18px" }}>
      <h2
        style={{
          marginBottom: 12,
          color: "var(--text)",
          textTransform: "none",
          letterSpacing: 0,
          display: "flex",
          alignItems: "center",
          gap: 6,
          flexWrap: "wrap",
        }}
      >
        {title}
        {subtitle ? <InfoHint label={title}>{subtitle}</InfoHint> : null}
      </h2>
      {children}
    </section>
  );
}

export function ChartBox({ h, children }: { h: number; children: ReactElement }) {
  return (
    <DeferredMount minHeight={h} placeholder={<div style={{ height: h, borderRadius: 8, background: "var(--panel2)", opacity: 0.55 }} />}>
      <div style={{ width: "100%", height: h, minHeight: h }}>
        <ResponsiveContainer width="100%" height="100%">
          {children}
        </ResponsiveContainer>
      </div>
    </DeferredMount>
  );
}
