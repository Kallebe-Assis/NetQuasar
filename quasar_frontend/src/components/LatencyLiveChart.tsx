import { useMemo } from "react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { LatencySample } from "../hooks/useContinuousIcmpPing";

type Props = {
  points: LatencySample[];
  height?: number;
  ariaLabel?: string;
  color?: string;
};

function fmtTime(t: number): string {
  try {
    return new Date(t).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return "";
  }
}

export function LatencyLiveChart({
  points,
  height = 160,
  ariaLabel = "Latência ICMP ao longo do tempo",
  color = "var(--accent, #0ea5e9)",
}: Props) {
  const data = useMemo(
    () =>
      points.map((p) => ({
        t: p.t,
        label: fmtTime(p.t),
        ms: p.ok && p.ms != null ? p.ms : null,
        loss: p.ok ? null : 0,
      })),
    [points],
  );

  if (points.length === 0) {
    return <p style={{ color: "var(--muted)", fontSize: 12, margin: 0 }}>A aguardar amostras de latência…</p>;
  }

  return (
    <div style={{ width: "100%", height }} role="img" aria-label={ariaLabel}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 10, fill: "var(--muted)" }} minTickGap={28} interval="preserveStartEnd" />
          <YAxis
            tick={{ fontSize: 10, fill: "var(--muted)" }}
            width={40}
            unit=" ms"
            domain={[0, (dataMax: number) => Math.max(10, Math.ceil((dataMax || 0) * 1.15))]}
            allowDecimals={false}
          />
          <Tooltip
            contentStyle={{
              background: "var(--panel)",
              border: "1px solid var(--border)",
              borderRadius: 8,
              fontSize: 12,
            }}
            formatter={(value) => {
              if (value == null || typeof value !== "number") return ["timeout / falha", "Latência"];
              return [`${value} ms`, "Latência"];
            }}
            labelFormatter={(_, payload) => {
              const row = payload?.[0]?.payload as { label?: string } | undefined;
              return row?.label ?? "";
            }}
          />
          <Line
            type="monotone"
            dataKey="ms"
            stroke={color}
            strokeWidth={2}
            dot={false}
            connectNulls={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
