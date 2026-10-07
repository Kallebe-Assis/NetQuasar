import { useMemo, useState } from "react";
import { History, RefreshCw } from "lucide-react";
import { Segmented, ToolPanel } from "./hubsoftAdminKit";
import { AuditLogTable } from "../../components/AuditLogTable";
import type { AuditRowView } from "../../lib/auditPresentation";
import { apiFetch } from "../../lib/api";
import { downloadCsv } from "./hubsoftCsv";
import { useConsultaToast } from "./hubsoftConsulta";
import { Download } from "lucide-react";

const ENTITY_TYPES = {
  client: "hubsoft_bulk_import_client",
  service: "hubsoft_bulk_import_service",
} as const;

/**
 * Histórico da importação em massa — "quantos e quais foram importados" — lido do log de auditoria
 * geral do sistema (ops_audit_log), que a própria importação já grava, linha a linha. Só leitura.
 */
// Ações do log que contam como sucesso (a linha cumpriu o que devia: criou, adicionou, corrigiu o
// login ou confirmou que já existia) — antes só "created" contava e o resto aparecia como erro.
const OK_ACTIONS = new Set(["created", "service_added", "login_repair", "already_exists"]);

export function HubsoftBulkImportHistory() {
  const { notify } = useConsultaToast();
  const [kind, setKind] = useState<"client" | "service">("client");
  const [rows, setRows] = useState<AuditRowView[] | null>(null);
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const r = await apiFetch<{ items: AuditRowView[] }>(
        `/api/v1/ops/audit?entity_type=${ENTITY_TYPES[kind]}&limit=2000`,
      );
      setRows(r.items);
      notify({ ok: true }, null, `Consulta realizada com sucesso — ${r.items.length} registro(s).`);
    } catch (e) {
      notify(null, e);
    } finally {
      setLoading(false);
    }
  }

  const stats = useMemo(() => {
    if (!rows) return null;
    const ok = rows.filter((r) => OK_ACTIONS.has(r.action)).length;
    return { total: rows.length, ok, failed: rows.length - ok };
  }, [rows]);

  function exportCsv() {
    if (!rows) return;
    const head = ["Quando", "Linha", "Identificação", "Resultado", "Mensagem", "id_cliente", "id_cliente_servico", "Usuário"];
    const data = rows.map((r) => {
      const a = r.after_data ?? {};
      return [
        new Date(r.created_at).toLocaleString("pt-BR"),
        String(a.line ?? ""),
        String(a.label ?? ""),
        OK_ACTIONS.has(r.action) ? "OK" : "Erro",
        String(a.message ?? ""),
        String(a.id_cliente ?? ""),
        String(a.id_cliente_servico ?? ""),
        r.actor ?? "",
      ];
    });
    downloadCsv(`historico-importacao-${kind}-${new Date().toISOString().slice(0, 10)}.csv`, head, data);
  }

  return (
    <ToolPanel
      icon={<History size={20} />}
      title="Histórico de importações"
      subtitle="Cada linha importada (com sucesso ou não) fica registrada aqui, com data, quem rodou e o id criado na HubSoft."
    >
      <div className="hsa-panel__body hsa-panel__body--pad">
        <Segmented
          label="Tipo de importação"
          value={kind}
          onChange={(k) => {
            setKind(k);
            setRows(null);
          }}
          options={[
            { value: "client", label: "Clientes novos" },
            { value: "service", label: "Serviços adicionais" },
          ]}
        />
        <div className="hsa-actions">
          <button type="button" className="btn btn--primary" disabled={loading} onClick={() => void load()}>
            <RefreshCw size={14} className={loading ? "map-refresh-spin" : undefined} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
            {loading ? "Consultando…" : "Consultar histórico"}
          </button>
          {rows ? (
            <button type="button" className="btn" disabled={rows.length === 0} onClick={exportCsv}>
              <Download size={14} style={{ marginRight: 6, verticalAlign: -2 }} aria-hidden />
              Exportar CSV
            </button>
          ) : null}
          {stats ? (
            <span className="hsa-spacer hsa-muted">
              {stats.total} registro(s) — <span style={{ color: "var(--ok)" }}>{stats.ok} ok</span>,{" "}
              <span style={{ color: stats.failed ? "var(--err)" : undefined }}>{stats.failed} com erro</span>
            </span>
          ) : null}
        </div>
        {rows === null ? (
          <div className="hubsoft-empty">Nenhuma consulta feita ainda.</div>
        ) : (
          <AuditLogTable rows={rows} emptyMessage="Nenhuma importação registrada ainda." />
        )}
      </div>
    </ToolPanel>
  );
}
