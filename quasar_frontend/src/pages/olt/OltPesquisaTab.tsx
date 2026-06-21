import { useMutation } from "@tanstack/react-query";
import { FileText, Filter } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { PageCountPill } from "../../components/PageCountPill";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { ApiError, apiFetch } from "../../lib/api";
import { parseApiErrorForModal, type ParsedApiError } from "../../lib/apiErrors";
import { OltOnuTelnetReportModal } from "./OltOnuTelnetReportModal";
import type { OltTelnetReportStep } from "../../lib/oltTelnetReportFormat";
import { EM_DASH, formatSnmpMetricCell } from "../../lib/formatDisplay";

export type OltOnuSearchResult = {
  olt_id: string;
  olt_description?: string | null;
  olt_ip?: string | null;
  olt_brand?: string | null;
  olt_model?: string | null;
  locality_name?: string | null;
  pon?: number;
  onu?: number;
  serial?: string;
  model?: string;
  online?: boolean;
  rx_dbm?: number;
  rx_pwr?: string;
  tx_pwr?: string;
  temp?: string;
  voltage?: string;
  if_index?: number;
  if_name?: string;
  snapshot_at?: string;
};

type SearchFilters = {
  serial: string;
  model: string;
  online: "" | "true" | "false";
  rx_dbm_min: string;
  rx_dbm_max: string;
  tx_dbm_min: string;
  tx_dbm_max: string;
  temp_min: string;
  temp_max: string;
  voltage_min: string;
  voltage_max: string;
  olt_id: string;
};

const EMPTY_FILTERS: SearchFilters = {
  serial: "",
  model: "",
  online: "",
  rx_dbm_min: "",
  rx_dbm_max: "",
  tx_dbm_min: "",
  tx_dbm_max: "",
  temp_min: "",
  temp_max: "",
  voltage_min: "",
  voltage_max: "",
  olt_id: "",
};

function parseOptFloat(s: string): number | undefined {
  const t = s.trim().replace(",", ".");
  if (!t) return undefined;
  const n = Number(t);
  return Number.isFinite(n) ? n : undefined;
}

function fmtRx(r: OltOnuSearchResult): string {
  if (typeof r.rx_dbm === "number" && Number.isFinite(r.rx_dbm)) return r.rx_dbm.toFixed(2);
  return r.rx_pwr ? formatSnmpMetricCell(r.rx_pwr) : EM_DASH;
}

type Props = {
  canMutate: boolean;
  olts: Array<{ id: string; description?: string | null }>;
};

export function OltPesquisaTab({ canMutate, olts }: Props) {
  const [q, setQ] = useState("");
  const debouncedQ = useDebouncedValue(q, 320);
  const [filters, setFilters] = useState<SearchFilters>(EMPTY_FILTERS);
  const [draftFilters, setDraftFilters] = useState<SearchFilters>(EMPTY_FILTERS);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [reportOpen, setReportOpen] = useState(false);
  const [reportTitle, setReportTitle] = useState("");
  const [reportSteps, setReportSteps] = useState<OltTelnetReportStep[]>([]);
  const [reportLoading, setReportLoading] = useState(false);
  const [errorModal, setErrorModal] = useState<ParsedApiError | null>(null);

  function openErrorModal(e: unknown, title: string) {
    setErrorModal(parseApiErrorForModal(e, title));
  }

  const activeFilterCount = useMemo(() => {
    let n = 0;
    if (filters.serial.trim()) n++;
    if (filters.model.trim()) n++;
    if (filters.online) n++;
    if (filters.olt_id) n++;
    if (filters.rx_dbm_min.trim() || filters.rx_dbm_max.trim()) n++;
    if (filters.tx_dbm_min.trim() || filters.tx_dbm_max.trim()) n++;
    if (filters.temp_min.trim() || filters.temp_max.trim()) n++;
    if (filters.voltage_min.trim() || filters.voltage_max.trim()) n++;
    return n;
  }, [filters]);

  const searchMut = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      apiFetch<{ results: OltOnuSearchResult[]; total: number }>("/api/v1/olt/onu-search", { method: "POST", json: body }),
    onError: (e) => openErrorModal(e, "Erro na pesquisa de ONUs"),
  });

  const payload = useMemo(() => {
    const body: Record<string, unknown> = { q: debouncedQ.trim() };
    if (filters.serial.trim()) body.serial = filters.serial.trim();
    if (filters.model.trim()) body.model = filters.model.trim();
    if (filters.olt_id) body.olt_id = filters.olt_id;
    if (filters.online === "true") body.online = true;
    if (filters.online === "false") body.online = false;
    const rxMin = parseOptFloat(filters.rx_dbm_min);
    const rxMax = parseOptFloat(filters.rx_dbm_max);
    const txMin = parseOptFloat(filters.tx_dbm_min);
    const txMax = parseOptFloat(filters.tx_dbm_max);
    const tempMin = parseOptFloat(filters.temp_min);
    const tempMax = parseOptFloat(filters.temp_max);
    const voltMin = parseOptFloat(filters.voltage_min);
    const voltMax = parseOptFloat(filters.voltage_max);
    if (rxMin != null) body.rx_dbm_min = rxMin;
    if (rxMax != null) body.rx_dbm_max = rxMax;
    if (txMin != null) body.tx_dbm_min = txMin;
    if (txMax != null) body.tx_dbm_max = txMax;
    if (tempMin != null) body.temp_min = tempMin;
    if (tempMax != null) body.temp_max = tempMax;
    if (voltMin != null) body.voltage_min = voltMin;
    if (voltMax != null) body.voltage_max = voltMax;
    return body;
  }, [debouncedQ, filters]);

  const payloadKey = JSON.stringify(payload);
  useEffect(() => {
    searchMut.mutate(payload);
  }, [payloadKey]); // eslint-disable-line react-hooks/exhaustive-deps

  const results = searchMut.data?.results ?? [];

  async function runReport(row: OltOnuSearchResult) {
    if (!canMutate) return;
    setReportLoading(true);
    setReportOpen(true);
    setReportTitle(`${row.olt_description ?? "OLT"} — PON ${row.pon ?? "?"} / ONU ${row.onu ?? "?"} ${row.serial ? `(${row.serial})` : ""}`);
    setReportSteps([]);
    try {
      const res = await apiFetch<{
        ok: boolean;
        output?: string;
        error?: string;
        commands?: OltTelnetReportStep[];
      }>(
        `/api/v1/olt/devices/${row.olt_id}/onu-report`,
        {
          method: "POST",
          json: {
            pon: row.pon ?? 0,
            onu: row.onu ?? 0,
            serial: row.serial ?? "",
            if_index: row.if_index ?? 0,
            if_name: row.if_name ?? "",
          },
        },
      );
      const steps = Array.isArray(res.commands) && res.commands.length > 0
        ? res.commands.map((c) => ({
            command: String(c.command ?? ""),
            ok: c.ok !== false,
            output: c.output ?? "",
            error: c.error,
          }))
        : res.output
          ? [{ command: "relatório", ok: res.ok, output: res.output }]
          : [];
      setReportSteps(steps);
      if (!res.ok) {
        openErrorModal(new ApiError(res.error || "Falha no relatório telnet.", 200, "TELNET_FAILED", res), "Erro no relatório telnet");
      }
    } catch (e) {
      setReportOpen(false);
      openErrorModal(e, "Erro no relatório telnet");
    } finally {
      setReportLoading(false);
    }
  }

  return (
    <>
      <div className="conn-toolbar" style={{ marginBottom: 12 }}>
        <PageCountPill label="ONUs encontradas" count={results.length} />
        <div className="conn-toolbar__spacer" aria-hidden />
        <label className="conn-toolbar__search">
          <input
            className="input"
            type="search"
            placeholder="Número de série ou modelo…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            autoComplete="off"
          />
        </label>
        <button
          type="button"
          className="btn btn--icon"
          style={activeFilterCount > 0 ? { outline: "2px solid var(--primary)", outlineOffset: 1 } : undefined}
          title={activeFilterCount > 0 ? `Filtros (${activeFilterCount} activos)` : "Filtros de potência, temperatura e voltagem"}
          aria-label={activeFilterCount > 0 ? `Filtros (${activeFilterCount} activos)` : "Filtros"}
          onClick={() => {
            setDraftFilters(filters);
            setFiltersOpen(true);
          }}
        >
          <Filter size={16} />
        </button>
      </div>

      {searchMut.isPending && !searchMut.data ? <p>A pesquisar ONUs…</p> : null}

      <div className="table-wrap">
        <table className="conn-table conn-table--center" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              <th>OLT</th>
              <th>PON</th>
              <th>ONU</th>
              <th>Série</th>
              <th>Modelo</th>
              <th>Status</th>
              <th>RX</th>
              <th>TX</th>
              <th>Temp.</th>
              <th>Voltagem</th>
              {canMutate ? <th style={{ width: 56 }} /> : null}
            </tr>
          </thead>
          <tbody>
            {results.map((r) => (
              <tr key={`${r.olt_id}-${r.pon}-${r.onu}-${r.serial ?? ""}`}>
                <td>{r.olt_description ?? EM_DASH}</td>
                <td className="mono">{r.pon ?? EM_DASH}</td>
                <td className="mono">{r.onu ?? EM_DASH}</td>
                <td className="mono">{r.serial ?? EM_DASH}</td>
                <td>{r.model ?? EM_DASH}</td>
                <td>
                  {r.online === true ? (
                    <span className="badge badge--ok">Online</span>
                  ) : r.online === false ? (
                    <span className="badge badge--err">Offline</span>
                  ) : (
                    EM_DASH
                  )}
                </td>
                <td className="mono">{fmtRx(r)}</td>
                <td className="mono">{r.tx_pwr ? formatSnmpMetricCell(r.tx_pwr) : EM_DASH}</td>
                <td className="mono">{r.temp ? formatSnmpMetricCell(r.temp) : EM_DASH}</td>
                <td className="mono">{r.voltage ? formatSnmpMetricCell(r.voltage) : EM_DASH}</td>
                {canMutate ? (
                  <td>
                    <button
                      type="button"
                      className="btn btn--icon"
                      title="Relatório telnet desta ONU"
                      disabled={reportLoading}
                      onClick={() => void runReport(r)}
                    >
                      <FileText size={15} />
                    </button>
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
        {!searchMut.isPending && results.length === 0 ? (
          <p style={{ padding: 12, color: "var(--muted)", fontSize: 12 }}>
            Nenhuma ONU encontrada nos snapshots. Actualize as OLTs em Equipamentos ou ajuste os filtros.
          </p>
        ) : null}
      </div>

      {filtersOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setFiltersOpen(false)}>
          <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 520 }} onMouseDown={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Filtros de pesquisa ONU</h3>
            <div className="field">
              <label>OLT</label>
              <select className="input" value={draftFilters.olt_id} onChange={(e) => setDraftFilters({ ...draftFilters, olt_id: e.target.value })}>
                <option value="">Todas</option>
                {olts.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.description ?? o.id}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label>Série (contém)</label>
              <input className="input" value={draftFilters.serial} onChange={(e) => setDraftFilters({ ...draftFilters, serial: e.target.value })} />
            </div>
            <div className="field">
              <label>Modelo (contém)</label>
              <input className="input" value={draftFilters.model} onChange={(e) => setDraftFilters({ ...draftFilters, model: e.target.value })} />
            </div>
            <div className="field">
              <label>Status</label>
              <select className="input" value={draftFilters.online} onChange={(e) => setDraftFilters({ ...draftFilters, online: e.target.value as SearchFilters["online"] })}>
                <option value="">Qualquer</option>
                <option value="true">Online</option>
                <option value="false">Offline</option>
              </select>
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
              <div className="field">
                <label>RX mín. (dBm)</label>
                <input className="input mono" value={draftFilters.rx_dbm_min} onChange={(e) => setDraftFilters({ ...draftFilters, rx_dbm_min: e.target.value })} />
              </div>
              <div className="field">
                <label>RX máx. (dBm)</label>
                <input className="input mono" value={draftFilters.rx_dbm_max} onChange={(e) => setDraftFilters({ ...draftFilters, rx_dbm_max: e.target.value })} />
              </div>
              <div className="field">
                <label>TX mín.</label>
                <input className="input mono" value={draftFilters.tx_dbm_min} onChange={(e) => setDraftFilters({ ...draftFilters, tx_dbm_min: e.target.value })} />
              </div>
              <div className="field">
                <label>TX máx.</label>
                <input className="input mono" value={draftFilters.tx_dbm_max} onChange={(e) => setDraftFilters({ ...draftFilters, tx_dbm_max: e.target.value })} />
              </div>
              <div className="field">
                <label>Temp. mín.</label>
                <input className="input mono" value={draftFilters.temp_min} onChange={(e) => setDraftFilters({ ...draftFilters, temp_min: e.target.value })} />
              </div>
              <div className="field">
                <label>Temp. máx.</label>
                <input className="input mono" value={draftFilters.temp_max} onChange={(e) => setDraftFilters({ ...draftFilters, temp_max: e.target.value })} />
              </div>
              <div className="field">
                <label>Voltagem mín.</label>
                <input className="input mono" value={draftFilters.voltage_min} onChange={(e) => setDraftFilters({ ...draftFilters, voltage_min: e.target.value })} />
              </div>
              <div className="field">
                <label>Voltagem máx.</label>
                <input className="input mono" value={draftFilters.voltage_max} onChange={(e) => setDraftFilters({ ...draftFilters, voltage_max: e.target.value })} />
              </div>
            </div>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
              <button type="button" className="btn" onClick={() => setDraftFilters(EMPTY_FILTERS)}>
                Limpar
              </button>
              <button type="button" className="btn" onClick={() => setFiltersOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                onClick={() => {
                  setFilters(draftFilters);
                  setFiltersOpen(false);
                }}
              >
                Aplicar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {errorModal ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setErrorModal(null)}>
          <div className="modal" role="alertdialog" aria-modal="true" style={{ maxWidth: 520 }} onMouseDown={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0, color: "var(--danger, #c62828)" }}>{errorModal.title}</h3>
            <div className="msg msg--err" style={{ marginBottom: 12, whiteSpace: "pre-wrap" }}>
              {errorModal.message}
            </div>
            {errorModal.code ? (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 12px" }}>
                Código: <span className="mono">{errorModal.code}</span>
                {errorModal.status ? ` · HTTP ${errorModal.status}` : null}
              </p>
            ) : null}
            <div className="row" style={{ justifyContent: "flex-end" }}>
              <button type="button" className="btn btn--primary" onClick={() => setErrorModal(null)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {reportOpen ? (
        <OltOnuTelnetReportModal
          open={reportOpen}
          loading={reportLoading}
          title={reportTitle}
          steps={reportSteps}
          onClose={() => setReportOpen(false)}
        />
      ) : null}
    </>
  );
}
