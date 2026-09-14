import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Blend, ClockFading, Cpu, Sun, ThermometerSun } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { invalidateAlertListQueries, queryKeys } from "../../lib/queryKeys";
import { can, isAdminUser } from "../../lib/auth";

/** Pode editar os limiares globais de alertas (aba Alertas). */
export function canEditAlertThresholds(): boolean {
  return isAdminUser() || can("alerts.manage") || can("settings.monitoring");
}

type AlertThresholdMetric = {
  id: string;
  label: string;
  unit: string;
  scope: string;
  enabled?: boolean;
  operator: "gte" | "lte";
  green_min: string;
  warning_min: string;
  critical_min: string;
  /** Categorias de equipamento (base de dados) em minúsculas; vazio = todos. */
  apply_categories?: string[];
};

function equipScopeFromCategories(cats?: string[]): "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor" {
  const c = cats ?? [];
  if (c.length === 0) return "*";
  const low = c.map((x) => String(x).toLowerCase().trim());
  if (low.some((x) => x === "*" || x === "all" || x === "todos")) return "*";
  if (low.includes("olt") && low.length === 1) return "olt";
  if (low.includes("mikrotik") && low.length === 1) return "mikrotik";
  if (low.includes("bng") && low.length === 1) return "bng";
  if (low.includes("switch") && low.length === 1) return "switch";
  if (low.includes("servidor") || low.includes("outros")) return "servidor";
  return "*";
}

function categoriesFromEquipScope(scope: "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor"): string[] {
  switch (scope) {
    case "olt":
      return ["olt"];
    case "mikrotik":
      return ["mikrotik"];
    case "bng":
      return ["bng"];
    case "switch":
      return ["switch"];
    case "servidor":
      return ["servidor", "outros"];
    default:
      return [];
  }
}

/** Catálogo padrão de métricas da aba Configurações → Alertas.
 * IDs devem coincidir com os avaliadores no backend (`alertthresholds` / `monitorworker`).
 * operator: "gte" (≥) ou "lte" (≤) — nunca usar símbolos Unicode soltos (evita mojibake). */
function defaultAlertMetrics(): AlertThresholdMetric[] {
  return [
    { id: "cpu_usage_pct", label: "CPU utilizada", unit: "%", scope: "equipamento", enabled: true, operator: "gte", green_min: "50", warning_min: "75", critical_min: "90", apply_categories: [] },
    { id: "memory_usage_pct", label: "Memória utilizada", unit: "%", scope: "equipamento", enabled: true, operator: "gte", green_min: "55", warning_min: "75", critical_min: "90", apply_categories: [] },
    { id: "latency_ms", label: "Latência de resposta", unit: "ms", scope: "equipamento", enabled: true, operator: "gte", green_min: "50", warning_min: "120", critical_min: "220", apply_categories: [] },
    { id: "temperature_c", label: "Temperatura do equipamento", unit: "°C", scope: "equipamento", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: [] },
    { id: "uptime_minutes", label: "Uptime (minutos)", unit: "min", scope: "equipamento", enabled: true, operator: "lte", green_min: "120", warning_min: "60", critical_min: "15", apply_categories: [] },
    { id: "olt_pon_tx_dbm", label: "PON TX da OLT", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-8", warning_min: "-14", critical_min: "-20", apply_categories: ["olt"] },
    { id: "olt_pon_rx_dbm", label: "PON RX da OLT", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-10", warning_min: "-16", critical_min: "", apply_categories: ["olt"] },
    { id: "olt_onu_tx_dbm", label: "ONU TX por PON", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-8", warning_min: "-15", critical_min: "-20", apply_categories: ["olt"] },
    { id: "olt_onu_rx_dbm", label: "ONU RX por PON", unit: "dBm", scope: "olt_pon", enabled: true, operator: "lte", green_min: "-12", warning_min: "-20", critical_min: "", apply_categories: ["olt"] },
    { id: "olt_pon_temp_c", label: "Temperatura da PON", unit: "°C", scope: "olt_pon", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: ["olt"] },
    { id: "olt_onu_drop_count", label: "Variação de ONUs online (por PON)", unit: "ONUs", scope: "olt_pon", enabled: true, operator: "gte", green_min: "0", warning_min: "2", critical_min: "5", apply_categories: ["olt"] },
    { id: "olt_onu_drop_percent", label: "Variação de ONUs online (%)", unit: "%", scope: "olt_pon", enabled: true, operator: "gte", green_min: "0", warning_min: "10", critical_min: "25", apply_categories: ["olt"] },
    { id: "bng_pppoe_drop_count", label: "Queda de PPPoE online (entre coletas)", unit: "sessões", scope: "bng", enabled: true, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_ipv4_drop_count", label: "Queda de IPv4 online (entre coletas)", unit: "sessões", scope: "bng", enabled: true, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_ipv6_drop_count", label: "Queda de IPv6 online (entre coletas)", unit: "sessões", scope: "bng", enabled: true, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_total_drop_count", label: "Queda de total online (entre coletas)", unit: "sessões", scope: "bng", enabled: false, operator: "gte", green_min: "0", warning_min: "50", critical_min: "150", apply_categories: ["bng"] },
    { id: "bng_dual_stack_drop_count", label: "Queda de dual-stack (entre coletas)", unit: "sessões", scope: "bng", enabled: false, operator: "gte", green_min: "0", warning_min: "20", critical_min: "80", apply_categories: ["bng"] },
    { id: "mikrotik_pppoe_drop_count", label: "Queda de sessões PPPoE MikroTik (entre coletas)", unit: "sessões", scope: "mikrotik_pppoe", enabled: true, operator: "gte", green_min: "0", warning_min: "10", critical_min: "30", apply_categories: ["mikrotik"] },
    { id: "iface_down_count", label: "Mudança de interface UP→DOWN", unit: "evento", scope: "interface", enabled: true, operator: "gte", green_min: "0", warning_min: "1", critical_min: "1", apply_categories: [] },
    { id: "mikrotik_sfp_tx_dbm", label: "SFP — potência TX", unit: "dBm", scope: "mikrotik_sfp", enabled: true, operator: "lte", green_min: "-8", warning_min: "-13", critical_min: "-18", apply_categories: ["mikrotik"] },
    { id: "mikrotik_sfp_rx_dbm", label: "SFP — potência RX", unit: "dBm", scope: "mikrotik_sfp", enabled: true, operator: "lte", green_min: "-10", warning_min: "-15", critical_min: "", apply_categories: ["mikrotik"] },
    { id: "mikrotik_sfp_temp_c", label: "Temperatura do módulo SFP", unit: "°C", scope: "mikrotik_sfp", enabled: true, operator: "gte", green_min: "45", warning_min: "60", critical_min: "75", apply_categories: ["mikrotik"] },
    { id: "mikrotik_cpu_temp_c", label: "Temperatura da CPU MikroTik", unit: "°C", scope: "mikrotik", enabled: true, operator: "gte", green_min: "55", warning_min: "70", critical_min: "85", apply_categories: ["mikrotik"] },
  ];
}

/** Painel da aba Alertas: persiste a regra «Limiar global de alertas» (condition_json schema v1). */
export function AlertThresholdsPanel() {
  type RuleRow = {
    id: string;
    name: string;
    enabled: boolean;
    condition?: unknown;
  };
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-alert-threshold-rules"],
    queryFn: () => apiFetch<{ rules: RuleRow[] }>("/api/v1/alert-rules"),
  });
  const [rows, setRows] = useState<AlertThresholdMetric[]>(defaultAlertMetrics());
  const [enabled, setEnabled] = useState(true);
  const scopeOptions: { value: string; label: string }[] = [
    { value: "equipamento", label: "Equipamento" },
    { value: "olt_pon", label: "PON da OLT" },
    { value: "bng", label: "BNG (logins)" },
    { value: "mikrotik_pppoe", label: "PPPoE MikroTik" },
    { value: "mikrotik_sfp", label: "SFP da MikroTik" },
    { value: "mikrotik", label: "MikroTik (CPU / saúde)" },
    { value: "interface", label: "Interface de rede" },
    { value: "onu", label: "ONU" },
    { value: "custom", label: "Outro" },
  ];
  const metricKeyFromLabel = (label: string, fallback: string): string => {
    const normalized = String(label)
      .toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_+|_+$/g, "");
    return normalized || fallback;
  };

  const thresholdRule = (q.data?.rules ?? []).find((r) => r.name === "Limiar global de alertas");

  useEffect(() => {
    if (!thresholdRule) return;
    setEnabled(!!thresholdRule.enabled);
    const c = (thresholdRule.condition ?? {}) as { metrics?: AlertThresholdMetric[] };
    if (Array.isArray(c.metrics) && c.metrics.length > 0) {
      const parsed: AlertThresholdMetric[] = c.metrics.map((m, idx) => {
        const ac = (m as AlertThresholdMetric).apply_categories;
        const applyCats = Array.isArray(ac) ? ac.map((x) => String(x).toLowerCase().trim()) : [];
        return {
          id: String(m.id ?? "").trim() || `metrica_${idx + 1}`,
          label: String(m.label ?? "").trim(),
          unit: String(m.unit ?? "").trim(),
          scope: String(m.scope ?? "").trim(),
          enabled: m.enabled !== false,
          operator: (m.operator === "lte" ? "lte" : "gte") as "lte" | "gte",
          green_min: String(m.green_min ?? ""),
          warning_min: String(m.warning_min ?? ""),
          critical_min: String(m.critical_min ?? ""),
          apply_categories: applyCats,
        };
      });
      const merged = [...parsed];
      for (const def of defaultAlertMetrics()) {
        if (!merged.some((m) => m.id === def.id)) merged.push(def);
      }
      setRows(merged);
    }
  }, [thresholdRule]);

  const upsert = useMutation({
    mutationFn: async () => {
      const payload = {
        schema: "netquasar.alert_thresholds.v1",
        metrics: rows
          .map((r, idx) => {
            const label = String(r.label).trim();
            const fallback = `metrica_${idx + 1}`;
            const cats = Array.isArray(r.apply_categories) ? r.apply_categories : [];
            return {
              ...r,
              id: String(r.id).trim() || metricKeyFromLabel(label, fallback),
              label,
              unit: String(r.unit).trim(),
              scope: String(r.scope).trim(),
              enabled: r.enabled !== false,
              green_min: String(r.green_min).trim(),
              warning_min: String(r.warning_min).trim(),
              critical_min: String(r.critical_min).trim(),
              apply_categories: cats,
            };
          })
          .filter((r) => r.label),
      };
      if (thresholdRule?.id) {
        return apiFetch(`/api/v1/alert-rules/${thresholdRule.id}`, {
          method: "PATCH",
          json: { name: "Limiar global de alertas", enabled, condition: payload },
        });
      }
      return apiFetch("/api/v1/alert-rules", {
        method: "POST",
        json: { name: "Limiar global de alertas", enabled, condition: payload, channels: {} },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settingsAlertThresholdRules });
      qc.invalidateQueries({ queryKey: queryKeys.alertRules });
      void invalidateAlertListQueries(qc);
    },
  });

  const updateRow = (idx: number, patch: Partial<AlertThresholdMetric>) =>
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, ...patch } : r)));

  const addRow = () =>
    setRows((prev) => [
      ...prev,
      {
        id: "",
        label: "",
        unit: "",
        scope: "equipamento",
        enabled: true,
        operator: "gte",
        green_min: "",
        warning_min: "",
        critical_min: "",
        apply_categories: [],
      },
    ]);

  const removeRow = (idx: number) => setRows((prev) => prev.filter((_, i) => i !== idx));

  const metricCatalog = defaultAlertMetrics();
  const addFromCatalog = (catalogId: string) => {
    const p = metricCatalog.find((m) => m.id === catalogId);
    if (!p) return;
    setRows((prev) => [
      ...prev,
      {
        ...p,
        id: p.id,
        label: p.label,
        apply_categories: [...(p.apply_categories ?? [])],
      },
    ]);
  };

  const equipScopeOptions: { value: "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor"; label: string }[] = [
    { value: "*", label: "Todos" },
    { value: "olt", label: "Somente OLT" },
    { value: "mikrotik", label: "Somente MikroTik" },
    { value: "bng", label: "Somente BNG" },
    { value: "switch", label: "Somente Switch" },
    { value: "servidor", label: "Servidor e outros" },
  ];
  const unitOptions = ["%", "ms", "°C", "dBm", "min", "ONUs", "evt", "Mbps"];
  const [selectedCatalog, setSelectedCatalog] = useState("");

  const scopeLabel = (scope: string): string => scopeOptions.find((s) => s.value === scope)?.label ?? scope;
  const saveHint = "Salvo em banco na regra «Limiar global de alertas» (tabela alert_rules).";
  const metricIcon = (id: string) => {
    const k = String(id).toLowerCase();
    if (k.includes("mikrotik")) return <img src="/MT_Symbol_Black.svg" alt="" width={14} height={14} />;
    if (k.includes("cpu")) return <Cpu size={14} aria-hidden />;
    if (k.includes("temperature") || k.includes("temp")) return <ThermometerSun size={14} aria-hidden />;
    if (k.includes("uptime")) return <ClockFading size={14} aria-hidden />;
    if (k.includes("sfp") || k.includes("_tx_") || k.includes("_rx_")) return <Blend size={14} aria-hidden />;
    if (k.includes("olt") || k.includes("onu") || k.includes("pon")) return <Sun size={14} aria-hidden />;
    return <Cpu size={14} aria-hidden />;
  };

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card alert-rules-card">
      <div className="alert-rules-head">
        <div>
          <h2 style={{ marginBottom: 6 }}>Configuração de Alertas</h2>
          <p style={{ color: "var(--muted)", fontSize: 13, margin: 0 }}>
            Defina por linha: tipo de equipamento, métrica, operador (≥ / ≤) e faixas <span style={{ color: "#3fb950" }}>Normal</span>,{" "}
            <span style={{ color: "#d29922" }}>Atenção</span> e <span style={{ color: "#f85149" }}>Crítico</span>.
            Os limiares são avaliados no ciclo automático de telemetria do worker e no refresh manual.
          </p>
        </div>
        <label className="row" style={{ gap: 8 }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          <span style={{ fontSize: 13 }}>Perfil ativo</span>
        </label>
      </div>

      <div className="alert-rules-toolbar">
        <div className="field" style={{ margin: 0, minWidth: 320 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>Adicionar métrica padrão</label>
          <select
            className="input"
            value={selectedCatalog}
            onChange={(e) => {
              const v = e.target.value;
              setSelectedCatalog(v);
              if (v) {
                addFromCatalog(v);
                setSelectedCatalog("");
              }
            }}
          >
            <option value="">Selecionar…</option>
            {metricCatalog.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label} ({scopeLabel(m.scope)})
              </option>
            ))}
          </select>
        </div>
        <button type="button" className="btn" onClick={addRow}>
          Novo critério
        </button>
      </div>

      <div className="alert-rules-grid-wrap">
        <table className="alert-rules-grid">
          <thead>
            <tr>
              <th>Métrica</th>
              <th>Equipamento</th>
              <th>Tipo de dado</th>
              <th>Condição</th>
              <th>Normal</th>
              <th>Atenção</th>
              <th>Crítico</th>
              <th>Habilitado</th>
              <th>Ações</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, idx) => (
              <tr key={`criterion-${idx}-${r.id || "new"}`}>
                <td>
                  <div className="alert-rules-metric-wrap">
                    <span className="alert-rules-metric-icon">{metricIcon(r.id)}</span>
                    <input className="input alert-rules-input-metric" value={r.label} onChange={(e) => updateRow(idx, { label: e.target.value })} placeholder="Nome da métrica" />
                  </div>
                </td>
                <td>
                  <select
                    className="input alert-rules-input-equip"
                    value={equipScopeFromCategories(r.apply_categories)}
                    onChange={(e) => updateRow(idx, { apply_categories: categoriesFromEquipScope(e.target.value as "*" | "olt" | "mikrotik" | "bng" | "switch" | "servidor") })}
                  >
                    {equipScopeOptions.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <select className="input alert-rules-input-unit" value={r.unit} onChange={(e) => updateRow(idx, { unit: e.target.value })}>
                    <option value="">-</option>
                    {unitOptions.map((u) => (
                      <option key={u} value={u}>
                        {u}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <div className="alert-rules-cond">
                    <select className="input alert-rules-input-scope" value={r.scope} onChange={(e) => updateRow(idx, { scope: e.target.value })}>
                      {scopeOptions.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                    <select className="input alert-rules-input-op" value={r.operator} onChange={(e) => updateRow(idx, { operator: e.target.value as "gte" | "lte" })}>
                      <option value="gte">≥ (maior ou igual)</option>
                      <option value="lte">≤ (menor ou igual)</option>
                    </select>
                  </div>
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.green_min} onChange={(e) => updateRow(idx, { green_min: e.target.value })} placeholder="0" />
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.warning_min} onChange={(e) => updateRow(idx, { warning_min: e.target.value })} placeholder="0" />
                </td>
                <td>
                  <input className="input mono alert-rules-input-num" value={r.critical_min} onChange={(e) => updateRow(idx, { critical_min: e.target.value })} placeholder="0" />
                </td>
                <td style={{ textAlign: "center" }}>
                  <label className="toggle" htmlFor={`rule-enabled-${idx}`} style={{ justifyContent: "center" }}>
                    <span className="toggle__track">
                      <input
                        id={`rule-enabled-${idx}`}
                        type="checkbox"
                        role="switch"
                        className="toggle__input"
                        checked={r.enabled !== false}
                        onChange={(e) => updateRow(idx, { enabled: e.target.checked })}
                      />
                      <span className="toggle__thumb" aria-hidden />
                    </span>
                  </label>
                </td>
                <td>
                  <button type="button" className="btn btn--danger btn--icon" aria-label="Remover regra" title="Remover" onClick={() => removeRow(idx)}>
                    <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden>
                      <path
                        d="M9 3h6l1 2h4v2H4V5h4l1-2zm1 6h2v9h-2V9zm4 0h2v9h-2V9zM7 9h2v9H7V9z"
                        fill="currentColor"
                      />
                    </svg>
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="row" style={{ marginTop: 16, gap: 10, flexWrap: "wrap", alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={upsert.isPending} onClick={() => upsert.mutate()}>
          Salvar alterações
        </button>
        <span style={{ fontSize: 12, color: "var(--muted)" }}>{saveHint}</span>
      </div>
      {upsert.isError && <div className="msg msg--err">{(upsert.error as Error).message}</div>}
      {upsert.isSuccess && (
        <div className="msg msg--ok">
          Critérios salvos com sucesso. O monitoramento consulta estes valores para decidir se abre, atualiza ou resolve alertas.
        </div>
      )}
    </div>
  );
}
