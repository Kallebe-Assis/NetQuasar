import { useMutation, useQuery } from "@tanstack/react-query";
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
  telnet_report_at?: string;
  data_source_telnet?: boolean;
  phase_sta?: string;
  channel?: string;
};

type TelnetSerialMatch = {
  pon?: number;
  onu?: number;
  serial?: string;
  model?: string;
  profile?: string;
  gpon_onu?: string;
};

type TelnetSerialResult = {
  ok: boolean;
  mode?: "direct" | "list";
  olt_id: string;
  olt_description?: string;
  serial?: string;
  pon_filter?: number;
  command?: string;
  commands?: Array<{ command?: string; output?: string; pon?: number; ok?: boolean }>;
  output?: string;
  matches?: TelnetSerialMatch[];
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
  const [selectedPon, setSelectedPon] = useState(0);
  const [ponManual, setPonManual] = useState("");
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

  const oltDetailQ = useQuery({
    queryKey: ["olt", "pesquisa-pons", selectedOltId],
    enabled: Boolean(selectedOltId),
    queryFn: () =>
      apiFetch<{ pons_table?: Array<{ pon?: number; id?: string; name?: string }> }>(
        `/api/v1/olt/devices/${selectedOltId}`,
      ),
    staleTime: 60_000,
  });

  const ponOptions = useMemo(() => {
    const rows = oltDetailQ.data?.pons_table ?? [];
    const nums = rows
      .map((r) => {
        if (typeof r.pon === "number" && r.pon > 0) return r.pon;
        const name = String(r.name ?? r.id ?? "");
        const m = name.match(/GPON0\/(\d+)/i);
        return m ? Number.parseInt(m[1], 10) : 0;
      })
      .filter((n) => n > 0);
    return [...new Set(nums)].sort((a, b) => a - b);
  }, [oltDetailQ.data?.pons_table]);

  const effectivePon = useMemo(() => {
    const manual = Number.parseInt(ponManual.trim(), 10);
    if (Number.isFinite(manual) && manual > 0) return manual;
    return selectedPon;
  }, [ponManual, selectedPon]);

  const selectedPonLabel =
    effectivePon > 0 ? `PON ${effectivePon}` : ponManual.trim() ? `PON ${ponManual.trim()}` : "Todas as PONs";

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
    if (effectivePon > 0) body.pon = effectivePon;
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
  }, [debouncedQ, filters, selectedOltId, effectivePon]);

  const payloadKey = JSON.stringify(payload);
  useEffect(() => {
    searchMut.mutate(payload);
  }, [payloadKey]); // eslint-disable-line react-hooks/exhaustive-deps

  const runTelnetSerialSearch = useCallback(async (oltId: string, serial: string, pon: number) => {
    setTelnetLoading(true);
    try {
      const res = await apiFetch<TelnetSerialResult>(`/api/v1/olt/devices/${oltId}/onu-serial-search`, {
        method: "POST",
        json: { serial, pon: pon > 0 ? pon : undefined },
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

  const telnetKey = `${selectedOltId}|${effectivePon}|${debouncedQ.trim()}`;
  useEffect(() => {
    const serial = debouncedQ.trim();
    if (!canMutate || !selectedOltId || serial.length < 2) {
      setTelnetResult(null);
      setTelnetLoading(false);
      return;
    }
    void runTelnetSerialSearch(selectedOltId, serial, effectivePon);
  }, [canMutate, telnetKey, runTelnetSerialSearch, selectedOltId, effectivePon, debouncedQ]);

  const snapshotResults = searchMut.data?.results ?? [];

  const displayResults = useMemo(() => {
    const list = [...snapshotResults];
    const telnetMatches =
      telnetResult?.matches && telnetResult.matches.length > 0
        ? telnetResult.matches
        : telnetResult?.ok && telnetResult.pon && telnetResult.onu
          ? [
              {
                pon: telnetResult.pon,
                onu: telnetResult.onu,
                serial: telnetResult.serial,
                gpon_onu: telnetResult.gpon_onu ?? undefined,
              },
            ]
          : [];

    for (const match of telnetMatches) {
      if (!match.pon || !match.onu) continue;
      const exists = list.some(
        (r) => r.olt_id === telnetResult?.olt_id && r.pon === match.pon && r.onu === match.onu,
      );
      if (exists) continue;
      list.unshift({
        olt_id: telnetResult!.olt_id,
        olt_description: telnetResult!.olt_description,
        pon: match.pon,
        onu: match.onu,
        serial: match.serial ?? telnetResult?.serial ?? debouncedQ.trim(),
        model: match.model,
        if_name: match.gpon_onu ?? undefined,
        telnet_only: true,
      });
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
            placeholder="Número de série (parcial) ou modelo…"
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
                    setSelectedPon(0);
                    setPonManual("");
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
                    setSelectedPon(0);
                    setPonManual("");
                    close();
                  }}
                >
                  {o.description ?? o.id}
                </button>
              ))}
            </div>
          )}
        </DropdownMenu>

        {selectedOltId ? (
          <>
            <DropdownMenu
              align="start"
              minWidth={160}
              trigger={({ toggle }) => (
                <button type="button" className="btn olt-pesquisa-toolbar__olt-btn" onClick={toggle} title="Filtrar porta PON">
                  <span className="olt-pesquisa-toolbar__olt-label">{selectedPonLabel}</span>
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
                      setSelectedPon(0);
                      setPonManual("");
                      close();
                    }}
                  >
                    Todas as PONs
                  </button>
                  {ponOptions.map((p) => (
                    <button
                      key={p}
                      type="button"
                      className="action-menu__item"
                      onClick={() => {
                        setSelectedPon(p);
                        setPonManual(String(p));
                        close();
                      }}
                    >
                      PON {p}
                    </button>
                  ))}
                  {ponOptions.length === 0 ? (
                    <p style={{ padding: "8px 12px", margin: 0, fontSize: 11, color: "var(--muted)" }}>
                      Sem snapshot PON — informe o número ao lado ou actualize a OLT.
                    </p>
                  ) : null}
                </div>
              )}
            </DropdownMenu>
            <input
              className="input mono"
              type="text"
              inputMode="numeric"
              placeholder="PON nº"
              title="Número da porta PON (opcional). Com PON definida, a OLT lista as ONUs dessa porta e compara o serial parcialmente."
              value={ponManual}
              onChange={(e) => {
                setPonManual(e.target.value.replace(/[^\d]/g, ""));
                const n = Number.parseInt(e.target.value.replace(/[^\d]/g, ""), 10);
                setSelectedPon(Number.isFinite(n) && n > 0 ? n : 0);
              }}
              style={{ width: 72, minWidth: 72, padding: "6px 8px", fontSize: 12 }}
            />
          </>
        ) : null}

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

      {canMutate && selectedOltId && debouncedQ.trim().length >= 2 ? (
        <div className="card" style={{ padding: "10px 12px", marginBottom: 12, fontSize: 12 }}>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
            <div>
              <strong>Consulta telnet na OLT</strong>
              <span style={{ color: "var(--muted)", marginLeft: 8 }}>{selectedOltLabel}</span>
              {effectivePon > 0 ? (
                <span style={{ color: "var(--muted)", marginLeft: 8 }}>
                  · PON {effectivePon} — lista ONUs e compara serial parcial
                </span>
              ) : (
                <span style={{ color: "var(--muted)", marginLeft: 8 }}>· todas as PONs (ou pesquisa directa por serial)</span>
              )}
            </div>
            <button
              type="button"
              className="btn btn--icon"
              title="Repetir consulta telnet"
              disabled={telnetLoading}
              onClick={() => void runTelnetSerialSearch(selectedOltId, debouncedQ.trim(), effectivePon)}
            >
              <RefreshCw size={15} className={telnetLoading ? "map-refresh-spin" : undefined} />
            </button>
          </div>
          {telnetLoading && !telnetResult ? (
            <p style={{ margin: "8px 0 0", color: "var(--muted)" }}>A consultar série via telnet…</p>
          ) : telnetResult ? (
            <div style={{ marginTop: 8 }}>
              <p style={{ margin: "0 0 6px", color: "var(--muted)" }}>
                Modo: <span className="mono">{telnetResult.mode === "list" ? "listagem + filtro local" : "directo"}</span>
                {typeof telnetResult.matches?.length === "number" ? (
                  <>
                    {" "}
                    · Correspondências: <span className="mono">{telnetResult.matches.length}</span>
                  </>
                ) : null}
              </p>
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
              {telnetResult.ok && (telnetResult.matches?.length ?? 0) > 0 ? (
                <button
                  type="button"
                  className="btn"
                  style={{ marginTop: 8 }}
                  disabled={reportLoading}
                  onClick={() => {
                    const first = telnetResult.matches?.[0];
                    if (!first?.pon || !first?.onu) return;
                    void runReport({
                      olt_id: telnetResult.olt_id,
                      olt_description: telnetResult.olt_description,
                      pon: first.pon,
                      onu: first.onu,
                      serial: first.serial ?? telnetResult.serial ?? debouncedQ.trim(),
                      if_name: first.gpon_onu ?? telnetResult.gpon_onu ?? undefined,
                    });
                  }}
                >
                  <FileText size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
                  Relatório completo telnet
                </button>
              ) : telnetResult.ok && telnetResult.pon && telnetResult.onu ? (
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
      ) : canMutate && debouncedQ.trim().length >= 2 && !selectedOltId ? (
        <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 12px" }}>
          Seleccione uma OLT para consultar o número de série via telnet. Com uma PON definida, o sistema lista as ONUs
          dessa porta e compara o serial digitado (mesmo parcial, ex. <span className="mono">CF8F197A</span> em{" "}
          <span className="mono">ITBS:CF8F:197A</span>).
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
                  ) : r.data_source_telnet ? (
                    <span className="badge badge--ok" style={{ marginLeft: 6, fontSize: 10 }} title={r.telnet_report_at ? `Telnet ${r.telnet_report_at}` : undefined}>
                      CLI
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
                <td className="mono">
                  {fmtRx(r)}
                  {r.data_source_telnet ? (
                    <span className="badge badge--ok" style={{ marginLeft: 4, fontSize: 9 }} title="RX via telnet">
                      CLI
                    </span>
                  ) : null}
                </td>
                <td className="mono">
                  {r.tx_pwr ? formatSnmpMetricCell(r.tx_pwr) : EM_DASH}
                  {r.data_source_telnet && r.tx_pwr ? (
                    <span className="badge badge--ok" style={{ marginLeft: 4, fontSize: 9 }} title="TX via telnet">
                      CLI
                    </span>
                  ) : null}
                </td>
                <td className="mono">
                  {r.temp ? formatSnmpMetricCell(r.temp) : EM_DASH}
                  {r.data_source_telnet && r.temp ? (
                    <span className="badge badge--ok" style={{ marginLeft: 4, fontSize: 9 }} title="Temperatura via telnet">
                      CLI
                    </span>
                  ) : null}
                </td>
                <td className="mono">
                  {r.voltage ? formatSnmpMetricCell(r.voltage) : EM_DASH}
                  {r.data_source_telnet && r.voltage ? (
                    <span className="badge badge--ok" style={{ marginLeft: 4, fontSize: 9 }} title="Voltagem via telnet">
                      CLI
                    </span>
                  ) : null}
                </td>
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
            {canMutate && debouncedQ.trim().length >= 2 && !selectedOltId
              ? " Seleccione uma OLT para pesquisar o serial via telnet (com PON opcional)."
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
