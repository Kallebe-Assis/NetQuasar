import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MapPin, Pencil, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { ActionMenu } from "../../components/ActionMenu";
import { ConfirmModal } from "../../components/ConfirmModal";
import { LocationMapModal, infraVariantToMapKind, type LocationMapPreview } from "../../components/LocationMapModal";
import { OltInterfaceSelects } from "../../components/OltInterfaceSelects";
import { formatOltPonLabel, matchOltByTransmitter, type OltPonCatalog } from "../../lib/oltPonInterfaces";
import { MaintenanceStatusCell } from "../../components/MaintenanceStatusCell";
import { PageCountPill } from "../../components/PageCountPill";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { apiFetch, downloadBlob } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { queryKeys } from "../../lib/queryKeys";
import { toastErr, toastLoading, toastOk } from "../../lib/operationToast";
import { type ConnectionsTabId } from "../../lib/connectionsFilters";
import {
  filterInfrastructureRows,
  NETWORK_INFRA_GC_MS,
  NETWORK_INFRA_STALE_MS,
} from "../../lib/networkInfraCache";
import { pageCachedQueryOptions, wrapPageCachedQueryFn } from "../../lib/pageDataCache";
import { buildExcelCsvBlob } from "../../lib/excelCsv";
import {
  INFRA_CSV_TEMPLATES,
  INFRA_IMPORT_BATCH_SIZE,
  parseInfraCsvFile,
  type InfraVariant,
} from "../../lib/infraCsvImport";
import {
  CABLE_STATUSES,
  fmtCoord,
  formatSplitterDisplay,
  normalizeSplitterInput,
  parseCoordInput,
  type NetworkCable,
  type NetworkCto,
  type NetworkPole,
  type NetworkSpliceBox,
  cableStatusLabel,
} from "../../lib/networkInfrastructure";
import {
  buildDefaultSplicePairs,
  buildDefaultSplitterPorts,
  CABLE_FIBER_COUNTS,
  isCableFiberCount,
  type SplicePair,
  type SplitterPort,
} from "../../lib/fiberSplitter";
import { CableFibersModal } from "../../components/CableFibersModal";
import { SpliceBoxModal } from "../../components/SpliceBoxModal";
import {
  CoordFields,
  FiberColorSelect,
  LocalitySelect,
  MaintenanceSwitch,
  ProjectSelect,
} from "./ConnectionsFormFields";
import { ConnectionsPager } from "./ConnectionsPager";
import { ConnectionsTabToolbar } from "./ConnectionsTabToolbar";
import type { ConnectionsTabProps } from "./shared";
import { useConnectionsLookups } from "./useConnectionsLookups";
import { usePagedRows } from "./usePagedRows";

type Variant = InfraVariant;

type DuplicatePolicy = "replace" | "ignore";

type ImportFailRow = {
  line?: number;
  index?: number;
  description?: string;
  error: string;
};

type ImportReport = {
  imported: number;
  skipped: number;
  failed: ImportFailRow[];
  fileName?: string;
};

type Props = ConnectionsTabProps & {
  variant: Variant;
  tabId: ConnectionsTabId;
};

const VARIANT_META: Record<
  Variant,
  {
    label: string;
    singular: string;
    api: string;
    bulkApi: string;
    listKey: string;
    queryKey: readonly string[];
    idPrefix: string;
    needsLocality: boolean;
  }
> = {
  cto: {
    label: "CTOs",
    singular: "CTO",
    api: "/api/v1/commercial/network/ctos",
    bulkApi: "/api/v1/commercial/network/ctos/bulk",
    listKey: "ctos",
    queryKey: queryKeys.networkCtos,
    idPrefix: "CTO",
    needsLocality: true,
  },
  splice: {
    label: "Caixas de emenda",
    singular: "Caixa de emenda",
    api: "/api/v1/commercial/network/splice-boxes",
    bulkApi: "/api/v1/commercial/network/splice-boxes/bulk",
    listKey: "splice_boxes",
    queryKey: queryKeys.networkSpliceBoxes,
    idPrefix: "Emenda",
    needsLocality: false,
  },
  cable: {
    label: "Cabos",
    singular: "Cabo",
    api: "/api/v1/commercial/network/cables",
    bulkApi: "/api/v1/commercial/network/cables/bulk",
    listKey: "cables",
    queryKey: queryKeys.networkCables,
    idPrefix: "Cabo",
    needsLocality: false,
  },
  pole: {
    label: "Postes",
    singular: "Poste",
    api: "/api/v1/commercial/network/poles",
    bulkApi: "/api/v1/commercial/network/poles/bulk",
    listKey: "poles",
    queryKey: queryKeys.networkPoles,
    idPrefix: "Poste",
    needsLocality: true,
  },
};

type Row = NetworkCto | NetworkSpliceBox | NetworkCable | NetworkPole;

function ctoInterfaceCell(r: Record<string, unknown>): string {
  const pon = Number(r.pon);
  if (!Number.isFinite(pon) || pon <= 0) return "—";
  return formatOltPonLabel(pon, String(r.pon_description ?? "").trim(), "");
}

function ctoVlanCell(r: Record<string, unknown>): string {
  if (r.vlan == null || r.vlan === "") return "—";
  return String(r.vlan);
}

function boolLabel(v: boolean): string {
  return v ? "sim" : "nao";
}

function projectNumFromRow(r: Record<string, unknown>): string {
  const lbl = r.project_label as string | undefined;
  if (!lbl) return "";
  const m = /^(\d+)/.exec(lbl);
  return m ? m[1] : "";
}

function rowToCsvRow(variant: Variant, row: Row): string[] {
  const r = row as Record<string, unknown>;
  const lat = r.latitude != null ? String(r.latitude) : "";
  const lon = r.longitude != null ? String(r.longitude) : "";
  const proj = projectNumFromRow(r);
  if (variant === "cto") {
    return [
      row.description,
      lat,
      lon,
      String(formatSplitterDisplay(r.splitter as string | null)),
      String(r.transmitter ?? ""),
      String(r.fiber_color ?? ""),
      String(r.locality_name ?? ""),
      proj,
      boolLabel(Boolean(r.needs_maintenance)),
      String(r.notes ?? ""),
    ];
  }
  if (variant === "splice") {
    return [
      row.description,
      lat,
      lon,
      r.fiber_count != null ? String(r.fiber_count) : "",
      boolLabel(Boolean(r.needs_maintenance)),
      String(r.notes ?? ""),
      proj,
    ];
  }
  if (variant === "cable") {
    return [
      row.description,
      String(r.cable_type ?? ""),
      r.fiber_count != null ? String(r.fiber_count) : "",
      String(r.status ?? "ativo"),
      lat,
      lon,
      proj,
    ];
  }
  return [
    row.description,
    String(r.pole_type ?? ""),
    String(r.locality_name ?? ""),
    lat,
    lon,
    proj,
  ];
}

export function InfrastructureTab({
  variant,
  tabId: _tabId,
  canMutate,
  filters,
  prefs,
  onSearchChange,
  onOpenFilters,
  onOpenSettings,
  activeFilterCount,
}: Props) {
  const meta = VARIANT_META[variant];
  const qc = useQueryClient();
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const csvRef = useRef<HTMLInputElement>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, string | boolean>>({});
  const [importOpen, setImportOpen] = useState(false);
  const [importPolicy, setImportPolicy] = useState<DuplicatePolicy>("replace");
  const [importReport, setImportReport] = useState<ImportReport | null>(null);
  const [importErrorsOpen, setImportErrorsOpen] = useState(false);
  const [importFileName, setImportFileName] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const [importProgress, setImportProgress] = useState<{
    total: number;
    processed: number;
    imported: number;
    skipped: number;
  } | null>(null);
  const [mapPreview, setMapPreview] = useState<LocationMapPreview | null>(null);
  const [cableFibersRow, setCableFibersRow] = useState<NetworkCable | null>(null);
  const [spliceBoxRow, setSpliceBoxRow] = useState<NetworkSpliceBox | null>(null);
  const [selectedCtoIds, setSelectedCtoIds] = useState<string[]>([]);
  const [linkOpen, setLinkOpen] = useState(false);
  const [linkForm, setLinkForm] = useState({ olt_device_id: "", transmitter: "", pon: "" });

  const debouncedQ = useDebouncedValue(filters.q, 320);
  const filterKey = useMemo(
    () => JSON.stringify({ filters, q: debouncedQ, sortDir: prefs.sortDir }),
    [filters, debouncedQ, prefs.sortDir],
  );

  const listQ = useQuery({
    queryKey: meta.queryKey,
    queryFn: wrapPageCachedQueryFn(meta.queryKey, async () => apiFetch<Record<string, Row[]>>(meta.api)),
    ...pageCachedQueryOptions<Record<string, Row[]>>(meta.queryKey, NETWORK_INFRA_STALE_MS, NETWORK_INFRA_GC_MS),
    placeholderData: keepPreviousData,
  });

  const { localities, projects } = useConnectionsLookups(formOpen);

  const oltsQ = useQuery({
    queryKey: queryKeys.oltDevices,
    queryFn: () => apiFetch<{ olts: OltPonCatalog[] }>("/api/v1/olt/devices"),
    enabled: variant === "cto",
    staleTime: 5 * 60 * 1000,
  });
  const olts = oltsQ.data?.olts ?? [];

  const rows = listQ.data?.[meta.listKey] ?? [];

  const filteredRows = useMemo(() => {
    const out = filterInfrastructureRows(rows, variant, filters, debouncedQ);
    return [...out].sort((a, b) =>
      prefs.sortDir === "asc" ? a.display_number - b.display_number : b.display_number - a.display_number,
    );
  }, [rows, variant, filters, debouncedQ, prefs.sortDir]);

  const { safePage, totalPages, pageRows, setPage, rangeFrom, rangeTo } = usePagedRows(
    filteredRows,
    prefs.pageSize,
    filterKey,
  );

  useEffect(() => {
    setSelectedCtoIds([]);
    setLinkOpen(false);
  }, [variant, filterKey]);

  function emptyForm(): Record<string, string | boolean> {
    const base: Record<string, string | boolean> = {
      description: "",
      latitude: "",
      longitude: "",
      project_id: "",
      needs_maintenance: false,
      notes: "",
    };
    if (variant === "cto") {
      return { ...base, splitter: "", transmitter: "", olt_device_id: "", pon: "", fiber_color: "Desconhecido", locality_id: "" };
    }
    if (variant === "splice") {
      return { ...base, fiber_count: "12", box_model: "emenda" };
    }
    if (variant === "cable") {
      return { ...base, cable_type: "", fiber_count: "", status: "ativo" };
    }
    return { ...base, pole_type: "", locality_id: "" };
  }

  function rowToForm(row: Row): Record<string, string | boolean> {
    const r = row as Record<string, unknown>;
    const f = emptyForm();
    f.description = String(r.description ?? "");
    f.latitude = r.latitude != null ? String(r.latitude) : "";
    f.longitude = r.longitude != null ? String(r.longitude) : "";
    f.project_id = r.project_id ? String(r.project_id) : "";
    if ("needs_maintenance" in r) f.needs_maintenance = Boolean(r.needs_maintenance);
    if ("notes" in r && r.notes) f.notes = String(r.notes);
    if (variant === "cto") {
      f.splitter = r.splitter ? normalizeSplitterInput(String(r.splitter)) ?? String(r.splitter) : "";
      f.transmitter = r.transmitter ? String(r.transmitter) : "";
      f.olt_device_id = r.olt_device_id ? String(r.olt_device_id) : "";
      f.pon = r.pon != null && Number(r.pon) > 0 ? String(r.pon) : "";
      f.fiber_color = r.fiber_color ? String(r.fiber_color) : "Desconhecido";
      f.locality_id = r.locality_id ? String(r.locality_id) : "";
    }
    if (variant === "splice") {
      if (r.fiber_count != null) f.fiber_count = String(r.fiber_count);
      f.box_model = r.box_model === "distribuicao" ? "distribuicao" : "emenda";
    }
    if (variant === "cable") {
      f.cable_type = r.cable_type ? String(r.cable_type) : "";
      f.fiber_count = r.fiber_count != null ? String(r.fiber_count) : "";
      f.status = r.status ? String(r.status) : "ativo";
    }
    if (variant === "pole") {
      f.pole_type = r.pole_type ? String(r.pole_type) : "";
      f.locality_id = r.locality_id ? String(r.locality_id) : "";
    }
    return f;
  }

  function formToPayload() {
    const lat = parseCoordInput(String(form.latitude));
    const lon = parseCoordInput(String(form.longitude));
    const payload: Record<string, unknown> = {
      description: String(form.description).trim(),
      latitude: lat,
      longitude: lon,
      project_id: String(form.project_id).trim() || null,
    };
    if (variant === "cto" || variant === "splice") {
      payload.needs_maintenance = Boolean(form.needs_maintenance);
      payload.notes = String(form.notes).trim() || null;
    }
    if (variant === "cto") {
      payload.splitter = normalizeSplitterInput(String(form.splitter));
      const matched = matchOltByTransmitter(olts, String(form.transmitter), String(form.olt_device_id) || null);
      payload.transmitter = String(form.transmitter).trim() || matched?.description?.trim() || null;
      payload.olt_device_id = String(form.olt_device_id).trim() || matched?.id || null;
      const ponN = Number(String(form.pon).trim());
      payload.pon = Number.isFinite(ponN) && ponN > 0 ? ponN : null;
      payload.fiber_color = String(form.fiber_color).trim() || "Desconhecido";
      payload.locality_id = String(form.locality_id).trim() || null;
    }
    if (variant === "splice") {
      const fc = String(form.fiber_count).trim();
      payload.fiber_count = fc ? Number(fc) : null;
      payload.box_model = String(form.box_model || "emenda");
    }
    if (variant === "cable") {
      payload.cable_type = String(form.cable_type).trim() || null;
      const fc = String(form.fiber_count).trim();
      payload.fiber_count = fc ? Number(fc) : null;
      payload.status = String(form.status).trim() || "ativo";
    }
    if (variant === "pole") {
      payload.pole_type = String(form.pole_type).trim() || null;
      payload.locality_id = String(form.locality_id).trim() || null;
    }
    return payload;
  }

  const saveMut = useMutation({
    mutationFn: async () => {
      const payload = formToPayload();
      if (!payload.description) throw new Error("Descrição obrigatória.");
      if (!payload.project_id) throw new Error("Projeto obrigatório.");
      if (editId) {
        return apiFetch(`${meta.api}/${editId}`, { method: "PATCH", json: payload });
      }
      return apiFetch(meta.api, { method: "POST", json: payload });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...meta.queryKey] });
      qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      setFormOpen(false);
      setEditId(null);
      setForm(emptyForm());
      toastOk(pushToast, editId ? `${meta.singular} actualizada.` : `${meta.singular} criada.`);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar."),
  });

  const linkOltMut = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/commercial/network/ctos/link-olt", {
        method: "POST",
        json: {
          ids: selectedCtoIds,
          olt_device_id: linkForm.olt_device_id,
          pon: Number(linkForm.pon),
        },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...meta.queryKey] });
      setLinkOpen(false);
      setSelectedCtoIds([]);
      toastOk(pushToast, selectedCtoIds.length > 1 ? "CTOs vinculadas à interface." : "CTO vinculada à interface.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao vincular interface."),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => apiFetch(`${meta.api}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [meta.queryKey] });
      setDeleteId(null);
      toastOk(pushToast, `${meta.singular} removida.`);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao remover."),
  });

  async function runCsvImport(file: File, policy: DuplicatePolicy) {
    setImporting(true);
    setImportFileName(file.name);
    setImportProgress(null);
    const loadingId = toastLoading(pushToast, `A importar «${file.name}»…`);
    try {
      const { rows, errors: parseErrors } = await parseInfraCsvFile(variant, file);
      const total = rows.length;
      let imported = 0;
      let skipped = 0;
      const failed: ImportFailRow[] = parseErrors.map((e) => ({
        line: e.line,
        description: e.description,
        error: e.error,
      }));

      if (total === 0 && failed.length > 0) {
        throw new Error(failed[0]?.error ?? "Nenhuma linha válida no CSV");
      }

      setImportProgress({ total, processed: 0, imported: 0, skipped: 0 });

      for (let offset = 0; offset < rows.length; offset += INFRA_IMPORT_BATCH_SIZE) {
        const batch = rows.slice(offset, offset + INFRA_IMPORT_BATCH_SIZE);
        const res = await apiFetch<{
          imported?: number;
          skipped?: number;
          failed?: Array<{ index?: number; line?: number; description?: string; error: string }>;
        }>(meta.bulkApi, {
          method: "POST",
          json: {
            items: batch.map((r) => r.payload),
            duplicate_policy: policy,
          },
        });

        imported += res.imported ?? 0;
        skipped += res.skipped ?? 0;
        for (const f of res.failed ?? []) {
          const idx = f.index ?? -1;
          const src = idx >= 0 && idx < batch.length ? batch[idx] : null;
          failed.push({
            line: src?.line ?? f.line,
            description: f.description ?? src?.payload.description,
            error: f.error,
          });
        }

        setImportProgress({
          total,
          processed: Math.min(offset + batch.length, total),
          imported,
          skipped,
        });
      }

      await qc.invalidateQueries({ queryKey: [...meta.queryKey] });
      await qc.invalidateQueries({ queryKey: queryKeys.networkProjects });

      const report: ImportReport = { imported, skipped, failed, fileName: file.name };
      setImportReport(report);
      setImportOpen(false);

      if (failed.length > 0) {
        setImportErrorsOpen(true);
        pushToast({
          tone: "info",
          text: `Importação concluída com ${failed.length} erro(s). ${imported} importado(s)${skipped ? `, ${skipped} ignorado(s)` : ""}.`,
          autoMs: 12_000,
        });
      } else {
        toastOk(
          pushToast,
          `Importação concluída: ${imported} ${meta.label.toLowerCase()}${skipped ? ` · ${skipped} ignorado(s)` : ""}.`,
        );
      }
    } catch (e) {
      toastErr(pushToast, e, "Falha na importação CSV.");
    } finally {
      setImporting(false);
      setImportFileName(null);
      setImportProgress(null);
      dismissToast(loadingId);
    }
  }

  function downloadTemplate() {
    const tpl = INFRA_CSV_TEMPLATES[variant];
    downloadBlob(tpl.fileName, buildExcelCsvBlob([tpl.headers, tpl.sample]));
  }

  function exportCsv() {
    const tpl = INFRA_CSV_TEMPLATES[variant];
    const dataRows = filteredRows.map((row) => rowToCsvRow(variant, row));
    const stamp = new Date().toISOString().slice(0, 10);
    const slug = variant === "splice" ? "caixas_emenda" : `${variant}s`;
    downloadBlob(`${slug}_${stamp}.csv`, buildExcelCsvBlob([tpl.headers, ...dataRows]));
    toastOk(pushToast, `Exportados ${dataRows.length} registo(s).`);
  }

  async function reloadFromDb() {
    try {
      const r = await listQ.refetch();
      if (r.error) {
        toastErr(pushToast, r.error);
      } else {
        toastOk(pushToast, `${meta.label} recarregados da base de dados.`);
      }
    } catch (e) {
      toastErr(pushToast, e);
    }
  }

  if (listQ.isPending && !listQ.data) return <p>A carregar {meta.label.toLowerCase()}…</p>;
  if (listQ.isError && !listQ.data) return <div className="msg msg--err">{errorMessageFromUnknown(listQ.error)}</div>;

  return (
    <>
      <ConnectionsTabToolbar
        search={filters.q}
        onSearchChange={onSearchChange}
        searchPlaceholder="Descrição, ID…"
        onOpenFilters={onOpenFilters}
        onOpenSettings={onOpenSettings}
        activeFilterCount={activeFilterCount}
        onReload={() => void reloadFromDb()}
        reloading={listQ.isFetching}
        reloadTitle={`Recarregar ${meta.label.toLowerCase()} da base de dados`}
      >
        <PageCountPill label={meta.label} count={filteredRows.length} />
        {canMutate ? (
          <>
            <button
              type="button"
              className="btn btn--primary"
              onClick={() => {
                setEditId(null);
                setForm(emptyForm());
                setFormOpen(true);
              }}
            >
              Nova {meta.singular.toLowerCase()}
            </button>
            {variant === "cto" ? (
              <button
                type="button"
                className="btn"
                disabled={selectedCtoIds.length === 0}
                title={selectedCtoIds.length === 0 ? "Seleccione uma ou mais CTOs na tabela" : "Vincular as CTOs seleccionadas a uma interface da OLT"}
                onClick={() => {
                  setLinkForm({ olt_device_id: "", transmitter: "", pon: "" });
                  setLinkOpen(true);
                }}
              >
                Vincular interface{selectedCtoIds.length ? ` (${selectedCtoIds.length})` : ""}
              </button>
            ) : null}
            <ActionMenu
              align="start"
              title="CSV — importar, modelo e exportar"
              items={[
                { id: "template", label: "Baixar modelo CSV", onClick: downloadTemplate },
                {
                  id: "import",
                  label: importing ? "Importação em curso…" : "Importar CSV…",
                  disabled: importing,
                  onClick: () => setImportOpen(true),
                },
                {
                  id: "export",
                  label: `Exportar ${meta.label.toLowerCase()} cadastrados`,
                  disabled: filteredRows.length === 0,
                  onClick: exportCsv,
                },
                ...(importReport && importReport.failed.length > 0
                  ? [
                      {
                        id: "errors",
                        label: `Ver erros da importação (${importReport.failed.length})`,
                        onClick: () => setImportErrorsOpen(true),
                      },
                    ]
                  : []),
              ]}
            />
            <input
              ref={csvRef}
              type="file"
              accept=".csv,text/csv"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                e.target.value = "";
                if (f) void runCsvImport(f, importPolicy);
              }}
            />
          </>
        ) : (
          <button type="button" className="btn" disabled={filteredRows.length === 0} onClick={exportCsv}>
            Exportar CSV
          </button>
        )}
      </ConnectionsTabToolbar>

      <div className="table-wrap">
        <table className="conn-table conn-table--center" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              {variant === "cto" && canMutate ? (
                <th style={{ width: 36 }}>
                  <input
                    type="checkbox"
                    checked={pageRows.length > 0 && pageRows.every((r) => selectedCtoIds.includes(String(r.id)))}
                    onChange={(e) => {
                      const ids = pageRows.map((r) => String(r.id));
                      if (e.target.checked) {
                        setSelectedCtoIds((prev) => Array.from(new Set([...prev, ...ids])));
                      } else {
                        const drop = new Set(ids);
                        setSelectedCtoIds((prev) => prev.filter((id) => !drop.has(id)));
                      }
                    }}
                    aria-label="Seleccionar página"
                  />
                </th>
              ) : null}
              <th>ID</th>
              <th>Descrição</th>
              {variant === "cto" ? (
                <>
                  <th>Splitter</th>
                  <th>Transmissor</th>
                  <th>Interface</th>
                  <th>VLAN</th>
                  <th>Cor fibra</th>
                  <th>Localidade</th>
                  <th>Manutenção</th>
                </>
              ) : null}
              {variant === "splice" ? (
                <>
                  <th>Modelo</th>
                  <th>Fibras</th>
                  <th>Manutenção</th>
                </>
              ) : null}
              {variant === "cable" ? (
                <>
                  <th>Tipo</th>
                  <th>Fibras</th>
                  <th>Status</th>
                </>
              ) : null}
              {variant === "pole" ? (
                <>
                  <th>Tipo</th>
                  <th>Localidade</th>
                </>
              ) : null}
              <th>Projeto</th>
              <th className="mono">Coord.</th>
              <th style={{ width: canMutate ? 110 : 48 }} />
            </tr>
          </thead>
          <tbody>
            {pageRows.map((row) => {
              const r = row as Record<string, unknown>;
              const hasCoords = r.latitude != null && r.longitude != null;
              const lat = hasCoords ? Number(r.latitude) : NaN;
              const lng = hasCoords ? Number(r.longitude) : NaN;
              return (
                <tr key={String(r.id)}>
                  {variant === "cto" && canMutate ? (
                    <td>
                      <input
                        type="checkbox"
                        checked={selectedCtoIds.includes(String(r.id))}
                        onChange={(e) => {
                          const id = String(r.id);
                          setSelectedCtoIds((prev) =>
                            e.target.checked ? (prev.includes(id) ? prev : [...prev, id]) : prev.filter((x) => x !== id),
                          );
                        }}
                        aria-label={`Seleccionar ${row.description}`}
                      />
                    </td>
                  ) : null}
                  <td className="mono">
                    {meta.idPrefix} {row.display_number}
                  </td>
                  <td>{row.description}</td>
                  {variant === "cto" ? (
                    <>
                      <td>{formatSplitterDisplay(r.splitter as string | null)}</td>
                      <td>{(r.transmitter as string) ?? "—"}</td>
                      <td>{ctoInterfaceCell(r)}</td>
                      <td>{ctoVlanCell(r)}</td>
                      <td>{(r.fiber_color as string) || "Desconhecido"}</td>
                      <td>{(r.locality_name as string) ?? "—"}</td>
                      <td>
                        <MaintenanceStatusCell needsMaintenance={Boolean(r.needs_maintenance)} />
                      </td>
                    </>
                  ) : null}
                  {variant === "splice" ? (
                    <>
                      <td>{r.box_model === "distribuicao" ? "Distribuição" : "Emenda"}</td>
                      <td>
                        {r.box_model === "distribuicao"
                          ? formatSplitterDisplay(r.splitter as string | null | undefined)
                          : ((r.fiber_count as number) ?? "—")}
                      </td>
                      <td>
                        <MaintenanceStatusCell needsMaintenance={Boolean(r.needs_maintenance)} />
                      </td>
                    </>
                  ) : null}
                  {variant === "cable" ? (
                    <>
                      <td>{(r.cable_type as string) ?? "—"}</td>
                      <td>{(r.fiber_count as number) ?? "—"}</td>
                      <td>{cableStatusLabel(String(r.status ?? ""))}</td>
                    </>
                  ) : null}
                  {variant === "pole" ? (
                    <>
                      <td>{(r.pole_type as string) ?? "—"}</td>
                      <td>{(r.locality_name as string) ?? "—"}</td>
                    </>
                  ) : null}
                  <td>{(r.project_label as string) ?? "—"}</td>
                  <td className="mono">
                    {hasCoords ? `${fmtCoord(lat)}, ${fmtCoord(lng)}` : "—"}
                  </td>
                  <td>
                    <div className="conn-row-actions">
                      {hasCoords ? (
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Ver no mapa"
                          onClick={() =>
                            setMapPreview({
                              title: row.description,
                              subtitle: `${meta.idPrefix} ${row.display_number}`,
                              lat,
                              lng,
                              kind: infraVariantToMapKind(variant),
                            })
                          }
                        >
                          <MapPin size={15} />
                        </button>
                      ) : null}
                      {variant === "cable" ? (
                        <button
                          type="button"
                          className="btn btn--sm"
                          title="Configurar fibras"
                          onClick={() => setCableFibersRow(r as NetworkCable)}
                        >
                          Fibras
                        </button>
                      ) : null}
                      {variant === "splice" ? (
                        <button
                          type="button"
                          className="btn btn--sm"
                          title="Configurar interior"
                          onClick={() => setSpliceBoxRow(r as NetworkSpliceBox)}
                        >
                          Interior
                        </button>
                      ) : null}
                      {canMutate ? (
                        <>
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Editar"
                            onClick={() => {
                              setEditId(String(r.id));
                              setForm(rowToForm(row));
                              setFormOpen(true);
                            }}
                          >
                            <Pencil size={15} />
                          </button>
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Remover"
                            onClick={() => setDeleteId(String(r.id))}
                          >
                            <Trash2 size={15} />
                          </button>
                        </>
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <ConnectionsPager
        safePage={safePage}
        totalPages={totalPages}
        total={filteredRows.length}
        rangeFrom={rangeFrom}
        rangeTo={rangeTo}
        onPrev={() => setPage((p) => p - 1)}
        onNext={() => setPage((p) => p + 1)}
      />

      {formOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !saveMut.isPending && setFormOpen(false)}>
          <div
            className="modal conn-form-modal conn-form-modal--infra"
            role="dialog"
            aria-modal="true"
            aria-labelledby="infra-form-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="conn-form-modal__head">
              <h2 id="infra-form-title">{editId ? `Editar ${meta.singular}` : `Nova ${meta.singular}`}</h2>
              <p>
                {editId
                  ? "O ID numérico é permanente e não pode ser alterado."
                  : "O ID será atribuído automaticamente na criação."}
              </p>
            </div>
            <div className="conn-form-modal__body">
              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Identificação</h3>
                <div className="conn-form-modal__grid">
                  <div className="conn-form-modal__field field--full">
                    <span className="conn-form-modal__field-label">Descrição *</span>
                    <input className="input" value={String(form.description)} onChange={(e) => setForm({ ...form, description: e.target.value })} />
                  </div>
                  <ProjectSelect
                    value={String(form.project_id)}
                    projects={projects}
                    onChange={(id) => setForm({ ...form, project_id: id })}
                  />
                </div>
              </section>

              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Localização</h3>
                <div className="conn-form-modal__grid">
                  <CoordFields
                    latitude={String(form.latitude)}
                    longitude={String(form.longitude)}
                    onChange={(lat, lon) => setForm({ ...form, latitude: lat, longitude: lon })}
                  />
                </div>
              </section>

              {variant === "cto" ? (
                <section className="conn-form-modal__section">
                  <h3 className="conn-form-modal__section-title">CTO / fibra</h3>
                  <div className="conn-form-modal__grid">
                    <LocalitySelect
                      value={String(form.locality_id)}
                      localities={localities}
                      onChange={(id) => setForm({ ...form, locality_id: id })}
                    />
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Splitter</span>
                      <input
                        className="input"
                        value={String(form.splitter)}
                        onChange={(e) => setForm({ ...form, splitter: e.target.value })}
                        placeholder="ex. 1x8"
                      />
                    </div>
                    <OltInterfaceSelects
                      olts={olts}
                      oltDeviceId={String(form.olt_device_id ?? "")}
                      transmitter={String(form.transmitter ?? "")}
                      pon={String(form.pon ?? "")}
                      disabled={saveMut.isPending}
                      onChange={(next) => setForm({ ...form, ...next })}
                    />
                    <FiberColorSelect
                      value={String(form.fiber_color)}
                      onChange={(v) => setForm({ ...form, fiber_color: v })}
                    />
                    <div className="conn-form-modal__field field--full">
                      <span className="conn-form-modal__field-label">Observações</span>
                      <textarea className="input" rows={2} value={String(form.notes)} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
                    </div>
                    <div className="conn-form-modal__field field--full">
                      <MaintenanceSwitch
                        checked={Boolean(form.needs_maintenance)}
                        onChange={(v) => setForm({ ...form, needs_maintenance: v })}
                      />
                    </div>
                  </div>
                </section>
              ) : null}

              {variant === "splice" ? (
                <section className="conn-form-modal__section">
                  <h3 className="conn-form-modal__section-title">Caixa de emenda</h3>
                  <div className="conn-form-modal__grid">
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Modelo</span>
                      <select
                        className="input"
                        value={String(form.box_model || "emenda")}
                        onChange={(e) => setForm({ ...form, box_model: e.target.value })}
                      >
                        <option value="emenda">Emenda</option>
                        <option value="distribuicao">Distribuição</option>
                      </select>
                    </div>
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Quantidade de fibras</span>
                      <select
                        className="input"
                        value={String(form.fiber_count)}
                        onChange={(e) => setForm({ ...form, fiber_count: e.target.value })}
                      >
                        <option value="">—</option>
                        {CABLE_FIBER_COUNTS.map((n) => (
                          <option key={n} value={String(n)}>
                            {n} fibras
                          </option>
                        ))}
                        {form.fiber_count &&
                        !isCableFiberCount(Number(form.fiber_count)) &&
                        String(form.fiber_count).trim() !== "" ? (
                          <option value={String(form.fiber_count)}>{String(form.fiber_count)} fibras (atual)</option>
                        ) : null}
                      </select>
                    </div>
                    <div className="conn-form-modal__field field--full">
                      <span className="conn-form-modal__field-label">Observações</span>
                      <textarea className="input" rows={2} value={String(form.notes)} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
                    </div>
                    <div className="conn-form-modal__field field--full">
                      <MaintenanceSwitch
                        checked={Boolean(form.needs_maintenance)}
                        onChange={(v) => setForm({ ...form, needs_maintenance: v })}
                      />
                    </div>
                    {editId ? (
                      <div className="conn-form-modal__field field--full">
                        <button
                          type="button"
                          className="btn btn--primary"
                          onClick={() => {
                            const row = rows.find((x) => x.id === editId) as NetworkSpliceBox | undefined;
                            if (row) setSpliceBoxRow(row);
                          }}
                        >
                          Configurar interior
                        </button>
                      </div>
                    ) : null}
                  </div>
                </section>
              ) : null}

              {variant === "cable" ? (
                <section className="conn-form-modal__section">
                  <h3 className="conn-form-modal__section-title">Cabo</h3>
                  <div className="conn-form-modal__grid">
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Tipo</span>
                      <input className="input" value={String(form.cable_type)} onChange={(e) => setForm({ ...form, cable_type: e.target.value })} />
                    </div>
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Quantidade de fibras</span>
                      <select
                        className="input"
                        value={String(form.fiber_count)}
                        onChange={(e) => setForm({ ...form, fiber_count: e.target.value })}
                      >
                        <option value="">—</option>
                        {CABLE_FIBER_COUNTS.map((n) => (
                          <option key={n} value={String(n)}>
                            {n} fibras
                          </option>
                        ))}
                        {form.fiber_count &&
                        !isCableFiberCount(Number(form.fiber_count)) &&
                        String(form.fiber_count).trim() !== "" ? (
                          <option value={String(form.fiber_count)}>{String(form.fiber_count)} fibras (atual)</option>
                        ) : null}
                      </select>
                    </div>
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Status</span>
                      <select className="input" value={String(form.status)} onChange={(e) => setForm({ ...form, status: e.target.value })}>
                        {CABLE_STATUSES.map((s) => (
                          <option key={s.value} value={s.value}>
                            {s.label}
                          </option>
                        ))}
                      </select>
                    </div>
                    {editId ? (
                      <div className="conn-form-modal__field field--full">
                        <button
                          type="button"
                          className="btn btn--primary"
                          onClick={() => {
                            const row = rows.find((r) => r.id === editId) as NetworkCable | undefined;
                            if (row) {
                              setCableFibersRow({
                                ...row,
                                fiber_count: String(form.fiber_count).trim()
                                  ? Number(form.fiber_count)
                                  : row.fiber_count,
                              });
                            }
                          }}
                        >
                          Configurar fibras
                        </button>
                      </div>
                    ) : null}
                  </div>
                </section>
              ) : null}

              {variant === "pole" ? (
                <section className="conn-form-modal__section">
                  <h3 className="conn-form-modal__section-title">Poste</h3>
                  <div className="conn-form-modal__grid">
                    <div className="conn-form-modal__field">
                      <span className="conn-form-modal__field-label">Tipo</span>
                      <input className="input" value={String(form.pole_type)} onChange={(e) => setForm({ ...form, pole_type: e.target.value })} />
                    </div>
                    <LocalitySelect
                      value={String(form.locality_id)}
                      localities={localities}
                      onChange={(id) => setForm({ ...form, locality_id: id })}
                    />
                  </div>
                </section>
              ) : null}
            </div>
            <div className="conn-form-modal__foot">
              <button type="button" className="btn" onClick={() => setFormOpen(false)} disabled={saveMut.isPending}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" onClick={() => saveMut.mutate()} disabled={saveMut.isPending}>
                Guardar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {importOpen && canMutate ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !importing && setImportOpen(false)}>
          <div className="modal conn-import-modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 440 }}>
            <h3 style={{ marginTop: 0 }}>Importar CSV — {meta.label}</h3>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Use o modelo (separador <strong>;</strong>). Duplicados identificados pela descrição. Lotes de {INFRA_IMPORT_BATCH_SIZE}.
            </p>
            <div className="field" style={{ marginTop: 12 }}>
              <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer" }}>
                <input type="radio" name={`import-dup-${variant}`} checked={importPolicy === "replace"} onChange={() => setImportPolicy("replace")} disabled={importing} />
                Substituir registo existente
              </label>
              <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer", marginTop: 6 }}>
                <input type="radio" name={`import-dup-${variant}`} checked={importPolicy === "ignore"} onChange={() => setImportPolicy("ignore")} disabled={importing} />
                Ignorar linha duplicada
              </label>
            </div>
            {importing ? (
              <div className="conn-import-modal__loading" role="status">
                <span className="page-toast__spinner" aria-hidden />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <strong>A importar…</strong>
                  {importFileName ? <div style={{ fontSize: 12, color: "var(--muted)", marginTop: 4 }}>{importFileName}</div> : null}
                  {importProgress && importProgress.total > 0 ? (
                    <div className="conn-import-progress">
                      <div className="conn-import-progress__bar" aria-hidden>
                        <div
                          className="conn-import-progress__fill"
                          style={{ width: `${Math.round((importProgress.processed / importProgress.total) * 100)}%` }}
                        />
                      </div>
                      <div className="conn-import-progress__meta">
                        <span>
                          {importProgress.processed} / {importProgress.total} linhas
                        </span>
                        <span>
                          {importProgress.imported} ok
                          {importProgress.skipped ? ` · ${importProgress.skipped} ignorados` : ""}
                        </span>
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
            ) : null}
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
              <button type="button" className="btn" disabled={importing} onClick={() => setImportOpen(false)}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" disabled={importing} onClick={() => csvRef.current?.click()}>
                {importing ? "A importar…" : "Escolher ficheiro…"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {importErrorsOpen && importReport && importReport.failed.length > 0 ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setImportErrorsOpen(false)}>
          <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 720 }}>
            <h3 style={{ marginTop: 0 }}>Erros na importação CSV</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", marginBottom: 12 }}>
              {importReport.fileName ? (
                <>
                  Ficheiro: <strong>{importReport.fileName}</strong> —{" "}
                </>
              ) : null}
              {importReport.imported} importado(s)
              {importReport.skipped ? `, ${importReport.skipped} ignorado(s)` : ""}, {importReport.failed.length} com erro.
            </p>
            <div className="table-wrap" style={{ maxHeight: 360, overflow: "auto" }}>
              <table style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th style={{ width: 56 }}>Linha</th>
                    <th>Descrição</th>
                    <th>Motivo</th>
                  </tr>
                </thead>
                <tbody>
                  {importReport.failed.map((row, idx) => (
                    <tr key={`${row.line ?? row.index ?? idx}-${row.description ?? idx}`}>
                      <td className="mono">{row.line ?? (row.index != null ? row.index + 1 : "—")}</td>
                      <td>{row.description ?? "—"}</td>
                      <td style={{ color: "var(--danger, #c44)" }}>{row.error}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
              <button type="button" className="btn btn--primary" onClick={() => setImportErrorsOpen(false)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleteId ? (
        <ConfirmModal
          open
          title={`Remover ${meta.singular}`}
          message="Esta acção não pode ser desfeita. O ID numérico não será reutilizado."
          confirmLabel="Remover"
          danger
          onCancel={() => setDeleteId(null)}
          onConfirm={() => deleteMut.mutate(deleteId)}
          busy={deleteMut.isPending}
        />
      ) : null}

      <LocationMapModal preview={mapPreview} onClose={() => setMapPreview(null)} />

      {cableFibersRow ? (
        <CableFibersModal
          open
          cableId={cableFibersRow.id}
          cableName={cableFibersRow.description}
          fiberCount={cableFibersRow.fiber_count}
          ports={
            cableFibersRow.fiber_ports && cableFibersRow.fiber_ports.length > 0
              ? buildDefaultSplitterPorts(
                  cableFibersRow.fiber_count && cableFibersRow.fiber_count > 0
                    ? cableFibersRow.fiber_count
                    : cableFibersRow.fiber_ports.length,
                  cableFibersRow.fiber_ports.map((p) => ({
                    port: Number(p.port) || 0,
                    color: String(p.color ?? ""),
                    color_hex: String(p.color_hex ?? "#64748b"),
                    label: String(p.label ?? ""),
                    status: (p.status as SplitterPort["status"]) || "livre",
                    note: String(p.note ?? ""),
                    destination: String(p.destination ?? ""),
                  })),
                )
              : null
          }
          canEdit={canMutate}
          onClose={() => setCableFibersRow(null)}
          onSaved={() => {
            void qc.invalidateQueries({ queryKey: meta.queryKey });
          }}
        />
      ) : null}

      {spliceBoxRow ? (
        <SpliceBoxModal
          open
          spliceId={spliceBoxRow.id}
          spliceName={spliceBoxRow.description}
          boxModel={spliceBoxRow.box_model}
          fiberCount={spliceBoxRow.fiber_count}
          splitter={spliceBoxRow.splitter}
          feedFiberColor={spliceBoxRow.fiber_color}
          ports={
            spliceBoxRow.splitter_ports && spliceBoxRow.splitter_ports.length > 0
              ? buildDefaultSplitterPorts(
                  spliceBoxRow.fiber_count && spliceBoxRow.fiber_count > 0
                    ? spliceBoxRow.fiber_count
                    : spliceBoxRow.splitter_ports.length,
                  spliceBoxRow.splitter_ports.map((p) => ({
                    port: Number(p.port) || 0,
                    color: String(p.color ?? ""),
                    color_hex: String(p.color_hex ?? "#64748b"),
                    label: String(p.label ?? ""),
                    status: (p.status as SplitterPort["status"]) || "livre",
                    note: String(p.note ?? ""),
                    destination: String(p.destination ?? ""),
                  })),
                )
              : null
          }
          pairs={
            spliceBoxRow.splice_pairs && spliceBoxRow.splice_pairs.length > 0
              ? buildDefaultSplicePairs(
                  spliceBoxRow.fiber_count && spliceBoxRow.fiber_count > 0
                    ? spliceBoxRow.fiber_count
                    : spliceBoxRow.splice_pairs.length,
                  spliceBoxRow.splice_pairs.map((p) => ({
                    port: Number(p.port) || 0,
                    left_color: String(p.left_color ?? ""),
                    left_color_hex: String(p.left_color_hex ?? "#64748b"),
                    right_color: String(p.right_color ?? ""),
                    right_color_hex: String(p.right_color_hex ?? "#64748b"),
                    status: (p.status as SplicePair["status"]) || "livre",
                    note: String(p.note ?? ""),
                    destination: String(p.destination ?? ""),
                  })),
                )
              : null
          }
          canEdit={canMutate}
          onClose={() => setSpliceBoxRow(null)}
          onSaved={() => {
            void qc.invalidateQueries({ queryKey: meta.queryKey });
          }}
        />
      ) : null}

      {linkOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !linkOltMut.isPending && setLinkOpen(false)}>
          <div
            className="modal conn-form-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="cto-link-olt-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="conn-form-modal__head">
              <h2 id="cto-link-olt-title">Vincular interface da OLT</h2>
              <p>
                {selectedCtoIds.length} CTO{selectedCtoIds.length === 1 ? "" : "s"} serão vinculadas à interface
                seleccionada. A VLAN é a da PON cadastrada na OLT.
              </p>
            </div>
            <div className="conn-form-modal__body">
              <div className="conn-form-modal__grid">
                <OltInterfaceSelects
                  olts={olts}
                  oltDeviceId={linkForm.olt_device_id}
                  transmitter={linkForm.transmitter}
                  pon={linkForm.pon}
                  disabled={linkOltMut.isPending}
                  onChange={setLinkForm}
                />
              </div>
            </div>
            <div className="conn-form-modal__foot">
              <button type="button" className="btn" disabled={linkOltMut.isPending} onClick={() => setLinkOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={linkOltMut.isPending || !linkForm.olt_device_id || !linkForm.pon}
                onClick={() => linkOltMut.mutate()}
              >
                {linkOltMut.isPending ? "A vincular…" : "Vincular"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
