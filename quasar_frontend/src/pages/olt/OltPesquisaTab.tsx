import { useMutation } from "@tanstack/react-query";
import { ChevronDown, FileText, Filter, RefreshCw, Server } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { DropdownMenu } from "../../components/DropdownMenu";
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
  telnet_only?: boolean;
};

type TelnetSerialResult = {
  ok: boolean;
  olt_id: string;
  olt_description?: string;
  serial?: string;
  command?: string;
  output?: string;
  gpon_onu?: string | null;
  pon?: number;
  onu?: number;
  parsed?: Record<string, string>;
  error?: string;
};

type SearchFilters = {
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
};

const EMPTY_FILTERS: SearchFilters = {
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
  const [selectedOltId, setSelectedOltId] = useState("");
  const [filters, setFilters] = useState<SearchFilters>(EMPTY_FILTERS);
  const [draftFilters, setDraftFilters] = useState<SearchFilters>(EMPTY_FILTERS);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [reportOpen, setReportOpen] = useState(false);
  const [reportTitle, setReportTitle] = useState("");
  const [reportSteps, setReportSteps] = useState<OltTelnetReportStep[]>([]);
  const [reportLoading, setReportLoading] = useState(false);
  const [errorModal, setErrorModal] = useState<ParsedApiError | null>(null);
  const [telnetResult, setTelnetResult] = useState<TelnetSerialResult | null>(null);
  const [telnetLoading, setTelnetLoading] = useState(false);

  const selectedOltLabel = useMemo(() => {
    if (!selectedOltId) return "Todas as OLTs";
    return olts.find((o) => o.id === selectedOltId)?.description ?? "OLT";
  }, [selectedOltId, olts]);

  function openErrorModal(e: unknown, title: string) {
    setErrorModal(parseApiErrorForModal(e, title));
  }

  const activeFilterCount = useMemo(() => {
    let n = 0;
    if (filters.model.trim()) n++;
    if (filters.online) n++;
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
    if (filters.model.trim()) body.model = filters.model.trim();
    if (selectedOltId) body.olt_id = selectedOltId;
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
  }, [debouncedQ, filters, selectedOltId]);

  const payloadKey = JSON.stringify(payload);
  useEffect(() => {
    searchMut.mutate(payload);
  }, [payloadKey]); // eslint-disable-line react-hooks/exhaustive-deps

  const runTelnetSerialSearch = useCallback(async (oltId: string, serial: string) => {
    setTelnetLoading(true);
    try {
      const res = await apiFetch<TelnetSerialResult>(`/api/v1/olt/devices/${oltId}/onu-serial-search`, {
        method: "POST",
        json: { serial },
      });
      setTelnetResult(res);
      if (!res.ok && res.error) {
        openErrorModal(new ApiError(res.error, 200, "TELNET_FAILED", res), "Erro na consulta telnet");
      }
    } catch (e) {
      setTelnetResult(null);
      openErrorModal(e, "Erro na consulta telnet");
    } finally {
      setTelnetLoading(false);
    }
  }, []);

  const telnetKey = `${selectedOltId}|${debouncedQ.trim()}`;
  useEffect(() => {
    const serial = debouncedQ.trim();
    if (!canMutate || !selectedOltId || serial.length < 3) {
      setTelnetResult(null);
      setTelnetLoading(false);
      return;
    }
    void runTelnetSerialSearch(selectedOltId, serial);
  }, [canMutate, telnetKey, runTelnetSerialSearch, selectedOltId, debouncedQ]);

  const snapshotResults = searchMut.data?.results ?? [];

  const displayResults = useMemo(() => {
    const list = [...snapshotResults];
    if (telnetResult?.ok && telnetResult.pon && telnetResult.onu) {
      const exists = list.some(
        (r) => r.olt_id === telnetResult.olt_id && r.pon === telnetResult.pon && r.onu === telnetResult.onu,
      );
      if (!exists) {
        const parsed = telnetResult.parsed ?? {};
        list.unshift({
          olt_id: telnetResult.olt_id,
          olt_description: telnetResult.olt_description,
          pon: telnetResult.pon,
          onu: telnetResult.onu,
          serial: telnetResult.serial ?? parsed.SN ?? debouncedQ.trim(),
          model: parsed.Modelo ?? parsed["Modelo reportado"],
          online: /online|working|up/i.test(parsed.Estado ?? parsed["Status ONU"] ?? ""),
          rx_pwr: parsed.RX,
          tx_pwr: parsed.TX,
          temp: parsed.Temperatura,
          voltage: parsed.Voltagem,
          if_name: telnetResult.gpon_onu ?? undefined,
          telnet_only: true,
        });
      }
    }
    return list;
  }, [snapshotResults, telnetResult, debouncedQ]);

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
      <div className="conn-toolbar olt-pesquisa-toolbar" style={{ marginBottom: 12 }}>
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

        <DropdownMenu
          align="start"
          minWidth={260}
          trigger={({ toggle }) => (
            <button type="button" className="btn olt-pesquisa-toolbar__olt-btn" onClick={toggle} title="Seleccionar OLT">
              <Server size={15} aria-hidden />
              <span className="olt-pesquisa-toolbar__olt-label">{selectedOltLabel}</span>
              <ChevronDown size={14} aria-hidden />
            </button>
          )}
        >
          {({ close }) => (
            <div>
              <button
                type="button"
                className="action-menu__item"
                onClick={() => {
                  setSelectedOltId("");
                  close();
                }}
              >
                Todas as OLTs
              </button>
              {olts.map((o) => (
                <button
                  key={o.id}
                  type="button"
                  className="action-menu__item"
                  onClick={() => {
                    setSelectedOltId(o.id);
                    close();
                  }}
                >
                  {o.description ?? o.id}
                </button>
              ))}
            </div>
          )}
        </DropdownMenu>

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

        <div className="conn-toolbar__spacer" aria-hidden />
        <PageCountPill label="ONUs encontradas" count={displayResults.length} />
      </div>

      {canMutate && selectedOltId && debouncedQ.trim().length >= 3 ? (
        <div className="card" style={{ padding: "10px 12px", marginBottom: 12, fontSize: 12 }}>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
            <div>
              <strong>Consulta telnet na OLT</strong>
              <span style={{ color: "var(--muted)", marginLeft: 8 }}>{selectedOltLabel}</span>
            </div>
            <button
              type="button"
              className="btn btn--icon"
              title="Repetir consulta telnet"
              disabled={telnetLoading}
              onClick={() => void runTelnetSerialSearch(selectedOltId, debouncedQ.trim())}
            >
              <RefreshCw size={15} className={telnetLoading ? "map-refresh-spin" : undefined} />
            </button>
          </div>
          {telnetLoading && !telnetResult ? (
            <p style={{ margin: "8px 0 0", color: "var(--muted)" }}>A consultar série via telnet…</p>
          ) : telnetResult ? (
            <div style={{ marginTop: 8 }}>
              <p style={{ margin: "0 0 6px", color: "var(--muted)" }}>
                Comando: <span className="mono">{telnetResult.command ?? EM_DASH}</span>
                {telnetResult.gpon_onu ? (
                  <>
                    {" "}
                    · Interface: <span className="mono">{telnetResult.gpon_onu}</span>
                  </>
                ) : null}
                {telnetResult.pon && telnetResult.onu ? (
                  <>
                    {" "}
                    · PON <span className="mono">{telnetResult.pon}</span> / ONU <span className="mono">{telnetResult.onu}</span>
                  </>
                ) : null}
              </p>
              {telnetResult.output ? (
                <pre
                  className="mono"
                  style={{
                    margin: 0,
                    maxHeight: 140,
                    overflow: "auto",
                    fontSize: 11,
                    padding: 8,
                    background: "var(--bg)",
                    borderRadius: 6,
                    border: "1px solid var(--border)",
                    whiteSpace: "pre-wrap",
                  }}
                >
                  {telnetResult.output}
                </pre>
              ) : (
                <p style={{ margin: 0, color: "var(--muted)" }}>Sem saída do equipamento.</p>
              )}
              {telnetResult.ok && telnetResult.pon && telnetResult.onu ? (
                <button
                  type="button"
                  className="btn"
                  style={{ marginTop: 8 }}
                  disabled={reportLoading}
                  onClick={() =>
                    void runReport({
                      olt_id: telnetResult.olt_id,
                      olt_description: telnetResult.olt_description,
                      pon: telnetResult.pon,
                      onu: telnetResult.onu,
                      serial: telnetResult.serial ?? debouncedQ.trim(),
                      if_name: telnetResult.gpon_onu ?? undefined,
                    })
                  }
                >
                  <FileText size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
                  Relatório completo telnet
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : canMutate && debouncedQ.trim().length >= 3 && !selectedOltId ? (
        <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 12px" }}>
          Seleccione uma OLT para consultar o número de série via telnet (comando configurado em Definições → Perfis OLT).
        </p>
      ) : null}

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
            {displayResults.map((r) => (
              <tr key={`${r.olt_id}-${r.pon}-${r.onu}-${r.serial ?? ""}-${r.telnet_only ? "t" : "s"}`}>
                <td>
                  {r.olt_description ?? EM_DASH}
                  {r.telnet_only ? (
                    <span className="badge" style={{ marginLeft: 6, fontSize: 10 }}>
                      telnet
                    </span>
                  ) : null}
                </td>
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
                    {r.pon && r.onu ? (
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Relatório telnet desta ONU"
                        disabled={reportLoading}
                        onClick={() => void runReport(r)}
                      >
                        <FileText size={15} />
                      </button>
                    ) : null}
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
        {!searchMut.isPending && displayResults.length === 0 ? (
          <p style={{ padding: 12, color: "var(--muted)", fontSize: 12 }}>
            Nenhuma ONU encontrada nos snapshots.
            {canMutate && debouncedQ.trim().length >= 3 && !selectedOltId
              ? " Seleccione uma OLT para pesquisar o serial via telnet."
              : " Actualize as OLTs em Equipamentos ou ajuste os filtros."}
          </p>
        ) : null}
      </div>

      {filtersOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setFiltersOpen(false)}>
          <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 520 }} onMouseDown={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Filtros de pesquisa ONU</h3>
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
