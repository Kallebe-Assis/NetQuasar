import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plug, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useDebouncedValue } from "../../hooks/useDebouncedValue";
import { ActionMenu } from "../../components/ActionMenu";
import { ConfirmModal } from "../../components/ConfirmModal";
import { PageCountPill } from "../../components/PageCountPill";
import { apiFetch, ApiError, downloadBlob } from "../../lib/api";
import { errorMessageFromUnknown } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { queryKeys } from "../../lib/queryKeys";
import { CONN_IMPORT_BATCH_SIZE, parseConnectionsCsvFile } from "../../lib/connCsvImport";
import { buildExcelCsvBlob } from "../../lib/excelCsv";
import { toastErr, toastLoading, toastOk } from "../../lib/operationToast";
import type { IntegrationSummary } from "../../integrations/types";
import type { ConnectionsTabProps } from "../connections/shared";
import { ConnectionsPager } from "../connections/ConnectionsPager";

export type ClientConnection = {
  id: string;
  display_number: number;
  client_name: string;
  address?: string | null;
  neighborhood?: string | null;
  login: string;
  password?: string | null;
  ip_address?: string | null;
  connection_kind: "pppoe" | "dhcp";
  medium_type?: "fibra" | "radio" | "cabo_utp" | null;
  sales_plan?: string | null;
  onu_mac_sn?: string | null;
  rx_dbm?: string | null;
  tx_dbm?: string | null;
  transmitter?: string | null;
  cto?: string | null;
  port?: string | null;
  latitude?: number | null;
  longitude?: number | null;
};

type ConnConflict = {
  id: string;
  display_number: number;
  client_name: string;
  login: string;
  ip_address?: string | null;
};

type DuplicatePolicy = "replace" | "ignore";

type ImportFailRow = {
  line?: number;
  index?: number;
  login?: string;
  error: string;
  details?: unknown;
};

type ImportReport = {
  imported: number;
  skipped: number;
  failed: ImportFailRow[];
  fileName?: string;
};

type IntegrationLookupClient = {
  name?: string;
  document?: string;
  ipv4?: string;
  address?: string;
  services?: Array<{ login?: string; ipv4?: string; plano_venda?: string; status?: string }>;
};

const EMPTY_FORM = {
  client_name: "",
  address: "",
  neighborhood: "",
  login: "",
  password: "",
  ip_address: "",
  connection_kind: "pppoe" as "pppoe" | "dhcp",
  medium_type: "" as "" | "fibra" | "radio" | "cabo_utp",
  sales_plan: "",
  onu_mac_sn: "",
  rx_dbm: "",
  tx_dbm: "",
  transmitter: "",
  cto: "",
  port: "",
  latitude: "",
  longitude: "",
};

function fmtCoord(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return "—";
  return v.toFixed(5);
}

function connToForm(c: ClientConnection) {
  return {
    client_name: c.client_name ?? "",
    address: c.address ?? "",
    neighborhood: c.neighborhood ?? "",
    login: c.login ?? "",
    password: c.password ?? "",
    ip_address: c.ip_address ?? "",
    connection_kind: c.connection_kind ?? "pppoe",
    medium_type: (c.medium_type ?? "") as "" | "fibra" | "radio" | "cabo_utp",
    sales_plan: c.sales_plan ?? "",
    onu_mac_sn: c.onu_mac_sn ?? "",
    rx_dbm: c.rx_dbm ?? "",
    tx_dbm: c.tx_dbm ?? "",
    transmitter: c.transmitter ?? "",
    cto: c.cto ?? "",
    port: c.port ?? "",
    latitude: c.latitude != null ? String(c.latitude) : "",
    longitude: c.longitude != null ? String(c.longitude) : "",
  };
}

function formToPayload(f: typeof EMPTY_FORM) {
  const lat = f.latitude.trim() ? Number(f.latitude.replace(",", ".")) : null;
  const lon = f.longitude.trim() ? Number(f.longitude.replace(",", ".")) : null;
  return {
    client_name: f.client_name.trim(),
    address: f.address.trim() || null,
    neighborhood: f.neighborhood.trim() || null,
    login: f.login.trim(),
    password: f.password.trim() || null,
    ip_address: f.ip_address.trim() || null,
    connection_kind: f.connection_kind,
    medium_type: f.medium_type || null,
    sales_plan: f.sales_plan.trim() || null,
    onu_mac_sn: f.onu_mac_sn.trim() || null,
    rx_dbm: f.rx_dbm.trim() || null,
    tx_dbm: f.tx_dbm.trim() || null,
    transmitter: f.transmitter.trim() || null,
    cto: f.cto.trim() || null,
    port: f.port.trim() || null,
    latitude: lat,
    longitude: lon,
  };
}

function formatImportFailMessage(row: ImportFailRow): string {
  const loc = row.line != null ? `Linha ${row.line}` : row.index != null ? `Registo ${row.index + 1}` : "—";
  const login = row.login ? ` · ${row.login}` : "";
  let extra = "";
  if (row.details && typeof row.details === "object") {
    const d = row.details as { login_conflict?: ConnConflict; ip_conflict?: ConnConflict };
    const parts: string[] = [];
    if (d.login_conflict) parts.push(`login #${d.login_conflict.display_number}`);
    if (d.ip_conflict) parts.push(`IP #${d.ip_conflict.display_number}`);
    if (parts.length) extra = ` (${parts.join(", ")})`;
  }
  return `${loc}${login}: ${row.error}${extra}`;
}

type SortKey =
  | "display_number"
  | "client_name"
  | "login"
  | "connection_kind"
  | "medium_type"
  | "sales_plan"
  | "ip_address"
  | "coords"
  | "cto_port";

const CSV_HEADER = [
  "nome_cliente",
  "endereco",
  "bairro",
  "login",
  "senha",
  "ip",
  "tipo_conexao",
  "meio",
  "plano",
  "mac_sn",
  "rx",
  "tx",
  "transmissor",
  "cto",
  "porta",
  "latitude",
  "longitude",
];

function connToCsvRow(c: ClientConnection): string[] {
  return [
    c.client_name,
    c.address ?? "",
    c.neighborhood ?? "",
    c.login,
    c.password ?? "",
    c.ip_address ?? "",
    c.connection_kind,
    c.medium_type ?? "",
    c.sales_plan ?? "",
    c.onu_mac_sn ?? "",
    c.rx_dbm ?? "",
    c.tx_dbm ?? "",
    c.transmitter ?? "",
    c.cto ?? "",
    c.port ?? "",
    c.latitude != null ? String(c.latitude) : "",
    c.longitude != null ? String(c.longitude) : "",
  ];
}

function sortConnections(rows: ClientConnection[], key: SortKey, dir: "asc" | "desc"): ClientConnection[] {
  const mul = dir === "asc" ? 1 : -1;
  const cmpStr = (a: string | null | undefined, b: string | null | undefined) =>
    String(a ?? "").localeCompare(String(b ?? ""), "pt", { sensitivity: "base" }) * mul;
  const sorted = [...rows];
  sorted.sort((a, b) => {
    switch (key) {
      case "display_number":
        return (a.display_number - b.display_number) * mul;
      case "client_name":
        return cmpStr(a.client_name, b.client_name);
      case "login":
        return cmpStr(a.login, b.login);
      case "connection_kind":
        return cmpStr(a.connection_kind, b.connection_kind);
      case "medium_type":
        return cmpStr(a.medium_type, b.medium_type);
      case "sales_plan":
        return cmpStr(a.sales_plan, b.sales_plan);
      case "ip_address":
        return cmpStr(a.ip_address, b.ip_address);
      case "coords": {
        const al = a.latitude ?? -999;
        const bl = b.latitude ?? -999;
        if (al !== bl) return (al - bl) * mul;
        return ((a.longitude ?? -999) - (b.longitude ?? -999)) * mul;
      }
      case "cto_port":
        return cmpStr([a.cto, a.port].filter(Boolean).join("/"), [b.cto, b.port].filter(Boolean).join("/"));
      default:
        return 0;
    }
  });
  return sorted;
}

function conflictSummary(loginC?: ConnConflict | null, ipC?: ConnConflict | null): string {
  const parts: string[] = [];
  if (loginC) {
    parts.push(`login «${loginC.login}» (#${loginC.display_number} — ${loginC.client_name})`);
  }
  if (ipC) {
    parts.push(`IPv4 ${ipC.ip_address ?? "—"} (#${ipC.display_number} — ${ipC.client_name})`);
  }
  return parts.join(" · ");
}

type Props = ConnectionsTabProps;

export function CommercialConnectionsTab({ canMutate, filters, prefs }: Props) {
  const qc = useQueryClient();
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const csvRef = useRef<HTMLInputElement>(null);
  const debouncedQ = useDebouncedValue(filters.q.trim(), 320);
  const kindQ = filters.logins.connection_kind;
  const [formOpen, setFormOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState(EMPTY_FORM);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [dupModal, setDupModal] = useState<{
    login_conflict?: ConnConflict | null;
    ip_conflict?: ConnConflict | null;
  } | null>(null);
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
  const [sortKey, setSortKey] = useState<SortKey>("display_number");
  const [sortDir, setSortDir] = useState<"asc" | "desc">(prefs.sortDir);
  const pageSize = prefs.pageSize;
  const [page, setPage] = useState(0);

  useEffect(() => {
    setSortDir(prefs.sortDir);
  }, [prefs.sortDir]);
  const [lookupOpen, setLookupOpen] = useState(false);
  const [lookupLogin, setLookupLogin] = useState("");
  const [lookupAll, setLookupAll] = useState(true);
  const [lookupIds, setLookupIds] = useState<string[]>([]);

  const list = useQuery({
    queryKey: [...queryKeys.clientConnections, kindQ, debouncedQ],
    queryFn: () => {
      const params = new URLSearchParams();
      if (kindQ) params.set("connection_kind", kindQ);
      if (debouncedQ) params.set("q", debouncedQ);
      const qs = params.toString();
      return apiFetch<{ connections: ClientConnection[] }>(`/api/v1/commercial/connections${qs ? `?${qs}` : ""}`);
    },
    placeholderData: keepPreviousData,
  });

  const integrationsQ = useQuery({
    queryKey: queryKeys.integrations,
    queryFn: () => apiFetch<{ integrations: IntegrationSummary[] }>("/api/v1/integrations"),
    enabled: lookupOpen,
  });

  const connections = list.data?.connections ?? [];

  const connectionsFiltered = useMemo(() => {
    let rows = connections;
    const medium = filters.logins.medium_type?.trim();
    const ctoText = filters.logins.cto?.trim().toLowerCase();
    if (medium) rows = rows.filter((c) => c.medium_type === medium);
    if (ctoText) rows = rows.filter((c) => (c.cto ?? "").toLowerCase().includes(ctoText));
    return rows;
  }, [connections, filters.logins.medium_type, filters.logins.cto]);

  useEffect(() => {
    setPage(0);
  }, [debouncedQ, kindQ, pageSize, sortKey, sortDir, filters.logins.medium_type, filters.logins.cto]);

  const sortedConnections = useMemo(
    () => sortConnections(connectionsFiltered, sortKey, sortDir),
    [connectionsFiltered, sortKey, sortDir],
  );

  const totalPages = Math.max(1, Math.ceil(sortedConnections.length / pageSize));
  const safePage = Math.min(page, totalPages - 1);
  const pageRows = useMemo(
    () => sortedConnections.slice(safePage * pageSize, safePage * pageSize + pageSize),
    [sortedConnections, safePage, pageSize],
  );
  const rangeFrom = sortedConnections.length === 0 ? 0 : safePage * pageSize + 1;
  const rangeTo = Math.min(sortedConnections.length, (safePage + 1) * pageSize);

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else {
      setSortKey(key);
      setSortDir("asc");
    }
  }

  function sortMark(key: SortKey) {
    if (sortKey !== key) return null;
    return <span className="conn-table__sort-mark">{sortDir === "asc" ? "▲" : "▼"}</span>;
  }

  const saveMut = useMutation({
    mutationFn: async (policy?: DuplicatePolicy) => {
      const payload = formToPayload(form);
      if (!payload.client_name || !payload.login) throw new Error("Nome e login são obrigatórios.");
      if (editId) {
        return apiFetch(`/api/v1/commercial/connections/${editId}`, { method: "PATCH", json: payload });
      }
      return apiFetch<{ id?: string; skipped?: boolean }>("/api/v1/commercial/connections", {
        method: "POST",
        json: { ...payload, duplicate_policy: policy ?? "reject" },
      });
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: queryKeys.clientConnections });
      qc.invalidateQueries({ queryKey: queryKeys.mapConnectionPoints });
      setFormOpen(false);
      setEditId(null);
      setForm(EMPTY_FORM);
      setDupModal(null);
      const skipped = data && typeof data === "object" && "skipped" in data && data.skipped;
      pushToast({
        text: skipped ? "Conexão ignorada (duplicado)." : editId ? "Conexão actualizada." : "Conexão criada.",
        tone: skipped ? "info" : "ok",
      });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError && e.code === "DUPLICATE" && e.body && typeof e.body === "object") {
        const details = (e.body as { details?: { login_conflict?: ConnConflict; ip_conflict?: ConnConflict } }).details;
        if (details) {
          setDupModal({
            login_conflict: details.login_conflict ?? null,
            ip_conflict: details.ip_conflict ?? null,
          });
          return;
        }
      }
      toastErr(pushToast, e, "Falha ao guardar.");
    },
  });

  const checkDupMut = useMutation({
    mutationFn: () => {
      const payload = formToPayload(form);
      return apiFetch<{
        has_duplicate: boolean;
        login_conflict?: ConnConflict;
        ip_conflict?: ConnConflict;
      }>("/api/v1/commercial/connections/check-duplicates", {
        method: "POST",
        json: {
          login: payload.login,
          ip_address: payload.ip_address,
          exclude_id: editId,
        },
      });
    },
    onSuccess: (data) => {
      if (!data.has_duplicate) {
        saveMut.mutate(undefined);
        return;
      }
      setDupModal({ login_conflict: data.login_conflict ?? null, ip_conflict: data.ip_conflict ?? null });
    },
    onError: (e) => toastErr(pushToast, e),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/connections/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.clientConnections });
      qc.invalidateQueries({ queryKey: queryKeys.mapConnectionPoints });
      setDeleteId(null);
      toastOk(pushToast, "Conexão removida.");
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao remover."),
  });

  async function runCsvImport(file: File, policy: DuplicatePolicy) {
    setImporting(true);
    setImportFileName(file.name);
    setImportProgress(null);
    const loadingId = toastLoading(pushToast, `A importar «${file.name}»…`);
    try {
      const { rows, errors: parseErrors } = await parseConnectionsCsvFile(file);
      const total = rows.length;
      let imported = 0;
      let skipped = 0;
      const failed: ImportFailRow[] = parseErrors.map((e) => ({
        line: e.line,
        login: e.login,
        error: e.error,
      }));

      if (total === 0 && failed.length > 0) {
        throw new Error(failed[0]?.error ?? "Nenhuma linha válida no CSV");
      }

      setImportProgress({ total, processed: 0, imported: 0, skipped: 0 });

      for (let offset = 0; offset < rows.length; offset += CONN_IMPORT_BATCH_SIZE) {
        const batch = rows.slice(offset, offset + CONN_IMPORT_BATCH_SIZE);
        const res = await apiFetch<{
          imported?: number;
          skipped?: number;
          failed?: Array<{ index?: number; line?: number; login?: string; error: string; details?: unknown }>;
        }>("/api/v1/commercial/connections/bulk", {
          method: "POST",
          json: {
            connections: batch.map((r) => r.payload),
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
            login: f.login ?? src?.payload.login,
            error: f.error,
            details: f.details,
          });
        }

        setImportProgress({
          total,
          processed: Math.min(offset + batch.length, total),
          imported,
          skipped,
        });
      }

      await qc.invalidateQueries({ queryKey: queryKeys.clientConnections });
      await qc.invalidateQueries({ queryKey: queryKeys.mapConnectionPoints });

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
        toastOk(pushToast, `Importação concluída: ${imported} conexão(ões)${skipped ? ` · ${skipped} ignorada(s)` : ""}.`);
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

  const lookupMut = useMutation({
    mutationFn: () =>
      apiFetch<{
        login: string;
        results: Array<{
          integration_id: string;
          integration_name: string;
          ok: boolean;
          message?: string;
          clients: IntegrationLookupClient[];
        }>;
      }>("/api/v1/commercial/connections/integration-lookup", {
        method: "POST",
        json: {
          login: lookupLogin.trim(),
          integration_ids: lookupAll ? [] : lookupIds,
        },
      }),
    onError: (e) => toastErr(pushToast, e, "Falha na consulta."),
  });

  function downloadTemplate() {
    const sample = [
      "Cliente Exemplo",
      "Rua A, 100",
      "Centro",
      "cliente@provedor",
      "senha123",
      "177.10.1.50",
      "pppoe",
      "fibra",
      "500 megas",
      "AA:BB:CC:DD:EE:FF",
      "-22,5",
      "2,1",
      "OLT-01",
      "CTO-12",
      "8",
      "-23,55052",
      "-46,63331",
    ];
    downloadBlob("modelo_conexoes_clientes.csv", buildExcelCsvBlob([CSV_HEADER, sample]));
  }

  function exportConnectionsCsv() {
    const rows = sortedConnections.map(connToCsvRow);
    const stamp = new Date().toISOString().slice(0, 10);
    downloadBlob(`conexoes_clientes_${stamp}.csv`, buildExcelCsvBlob([CSV_HEADER, ...rows]));
    toastOk(pushToast, `Exportados ${rows.length} login(s).`);
  }

  function openCreate() {
    setEditId(null);
    setForm(EMPTY_FORM);
    setFormOpen(true);
  }

  function openEdit(c: ClientConnection) {
    setEditId(c.id);
    setForm(connToForm(c));
    setFormOpen(true);
  }

  function openLookup(login: string) {
    setLookupLogin(login);
    setLookupAll(true);
    setLookupIds([]);
    setLookupOpen(true);
    lookupMut.reset();
  }

  function applyLookupClient(client: IntegrationLookupClient) {
    const svc = client.services?.[0];
    setForm((f) => ({
      ...f,
      client_name: client.name?.trim() || f.client_name,
      address: client.address?.trim() || f.address,
      login: svc?.login?.trim() || lookupLogin.trim() || f.login,
      ip_address: svc?.ipv4?.trim() || client.ipv4?.trim() || f.ip_address,
      sales_plan: svc?.plano_venda?.trim() || f.sales_plan,
    }));
    setLookupOpen(false);
    if (!formOpen) {
      setEditId(null);
      setFormOpen(true);
    }
    toastOk(pushToast, "Dados preenchidos a partir da integração.");
  }

  function handleSaveClick() {
    if (editId) {
      saveMut.mutate(undefined);
      return;
    }
    checkDupMut.mutate();
  }

  function confirmDupSave(policy: DuplicatePolicy) {
    saveMut.mutate(policy);
  }

  if (list.isPending && !list.data) return <p>Carregando conexões…</p>;
  if (list.isError && !list.data) return <div className="msg msg--err">{errorMessageFromUnknown(list.error)}</div>;

  const integrationOptions = integrationsQ.data?.integrations ?? [];

  const hiddenCols = new Set(prefs.hiddenColumns);
  const showSecondary = prefs.showSecondaryInfo !== false;

  return (
    <>
      <div className="conn-toolbar">
        <PageCountPill label="Logins" count={sortedConnections.length} />
        <button
          type="button"
          className="btn"
          title="Buscar login nas integrações"
          onClick={() => openLookup(filters.q.trim())}
        >
          <Search size={16} strokeWidth={2} style={{ marginRight: 6, verticalAlign: -3 }} />
          Integrações
        </button>
        {canMutate ? (
          <>
            <button type="button" className="btn btn--primary" onClick={openCreate}>
              Nova conexão
            </button>
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
                  label: "Exportar logins cadastrados",
                  disabled: sortedConnections.length === 0,
                  onClick: exportConnectionsCsv,
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
          <button
            type="button"
            className="btn"
            disabled={sortedConnections.length === 0}
            onClick={exportConnectionsCsv}
          >
            Exportar CSV
          </button>
        )}
      </div>

      <div className="table-wrap">
        <table className="conn-table" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              <th className="conn-table__num conn-table__sortable" onClick={() => toggleSort("display_number")}>
                #{sortMark("display_number")}
              </th>
              {!hiddenCols.has("client_name") ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("client_name")}>
                  Cliente{sortMark("client_name")}
                </th>
              ) : null}
              {!hiddenCols.has("login") ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("login")}>
                  Login{sortMark("login")}
                </th>
              ) : null}
              {!hiddenCols.has("connection_kind") ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("connection_kind")}>
                  Tipo{sortMark("connection_kind")}
                </th>
              ) : null}
              {!hiddenCols.has("medium_type") ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("medium_type")}>
                  Meio{sortMark("medium_type")}
                </th>
              ) : null}
              {!hiddenCols.has("sales_plan") && showSecondary ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("sales_plan")}>
                  Plano{sortMark("sales_plan")}
                </th>
              ) : null}
              {!hiddenCols.has("ip_address") ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("ip_address")}>
                  IP{sortMark("ip_address")}
                </th>
              ) : null}
              {!hiddenCols.has("coords") ? (
                <th className="mono conn-table__sortable" onClick={() => toggleSort("coords")}>
                  Coord.{sortMark("coords")}
                </th>
              ) : null}
              {!hiddenCols.has("cto_port") ? (
                <th className="conn-table__sortable" onClick={() => toggleSort("cto_port")}>
                  CTO / Porta{sortMark("cto_port")}
                </th>
              ) : null}
              <th style={{ width: 100 }} />
            </tr>
          </thead>
          <tbody>
            {pageRows.map((c) => (
              <tr key={c.id}>
                <td className="conn-table__num mono">{c.display_number}</td>
                {!hiddenCols.has("client_name") ? (
                  <td className="conn-table__client" title={c.client_name}>
                    {c.client_name}
                  </td>
                ) : null}
                {!hiddenCols.has("login") ? <td className="mono">{c.login}</td> : null}
                {!hiddenCols.has("connection_kind") ? (
                  <td>{c.connection_kind === "dhcp" ? "DHCP" : "PPPoE"}</td>
                ) : null}
                {!hiddenCols.has("medium_type") ? <td>{c.medium_type ?? "—"}</td> : null}
                {!hiddenCols.has("sales_plan") && showSecondary ? <td>{c.sales_plan ?? "—"}</td> : null}
                {!hiddenCols.has("ip_address") ? <td className="mono">{c.ip_address ?? "—"}</td> : null}
                {!hiddenCols.has("coords") ? (
                  <td className="mono">
                    {c.latitude != null && c.longitude != null ? `${fmtCoord(c.latitude)}, ${fmtCoord(c.longitude)}` : "—"}
                  </td>
                ) : null}
                {!hiddenCols.has("cto_port") ? <td>{[c.cto, c.port].filter(Boolean).join(" / ") || "—"}</td> : null}
                <td>
                  <div className="conn-row-actions">
                    <button
                      type="button"
                      className="btn btn--icon"
                      title="Buscar nas integrações"
                      aria-label="Buscar nas integrações"
                      onClick={() => openLookup(c.login)}
                    >
                      <Plug size={15} strokeWidth={2} />
                    </button>
                    {canMutate ? (
                      <>
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Editar"
                          aria-label="Editar"
                          onClick={() => openEdit(c)}
                        >
                          <Pencil size={15} strokeWidth={2} />
                        </button>
                        <button
                          type="button"
                          className="btn btn--icon btn--danger"
                          title="Excluir"
                          aria-label="Excluir"
                          onClick={() => setDeleteId(c.id)}
                        >
                          <Trash2 size={15} strokeWidth={2} />
                        </button>
                      </>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <ConnectionsPager
          safePage={safePage}
          totalPages={totalPages}
          total={sortedConnections.length}
          rangeFrom={rangeFrom}
          rangeTo={rangeTo}
          onPrev={() => setPage((p) => Math.max(0, p - 1))}
          onNext={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
        />
      </div>

      {formOpen && canMutate ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !saveMut.isPending && setFormOpen(false)}>
          <div
            className="modal conn-form-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="conn-form-title"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="conn-form-modal__head">
              <h2 id="conn-form-title">{editId ? "Editar conexão" : "Nova conexão"}</h2>
              <p>
                {editId
                  ? "Actualize os dados do cliente. O login PPPoE/DHCP não pode ser alterado."
                  : "Cadastre um cliente com login, plano e coordenadas para aparecer no mapa de conexões."}
              </p>
            </div>
            <div className="conn-form-modal__body">
              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Cliente</h3>
                <div className="conn-form-modal__grid">
                  <label className="conn-form-modal__field field--full">
                    Nome do cliente *
                    <input className="input" value={form.client_name} onChange={(e) => setForm({ ...form, client_name: e.target.value })} placeholder="Nome completo ou razão social" />
                  </label>
                  <label className="conn-form-modal__field">
                    Endereço
                    <input className="input" value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} placeholder="Rua, número, complemento" />
                  </label>
                  <label className="conn-form-modal__field">
                    Bairro
                    <input className="input" value={form.neighborhood} onChange={(e) => setForm({ ...form, neighborhood: e.target.value })} />
                  </label>
                </div>
              </section>

              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Ligação</h3>
                <div className="conn-form-modal__grid">
                  <label className="conn-form-modal__field">
                    Login *
                    <span>Identificador único PPPoE ou DHCP</span>
                    <div className="row" style={{ gap: 6 }}>
                      <input
                        className="input mono"
                        style={{ flex: 1 }}
                        value={form.login}
                        onChange={(e) => setForm({ ...form, login: e.target.value })}
                        disabled={!!editId}
                        placeholder="cliente@provedor"
                      />
                      {!editId ? (
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Buscar login nas integrações"
                          aria-label="Buscar login nas integrações"
                          onClick={() => openLookup(form.login)}
                        >
                          <Plug size={16} strokeWidth={2} />
                        </button>
                      ) : null}
                    </div>
                  </label>
                  <label className="conn-form-modal__field">
                    Senha
                    <input className="input mono" type="password" autoComplete="off" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
                  </label>
                  <label className="conn-form-modal__field">
                    Tipo de conexão
                    <select className="select" value={form.connection_kind} onChange={(e) => setForm({ ...form, connection_kind: e.target.value as "pppoe" | "dhcp" })}>
                      <option value="pppoe">PPPoE</option>
                      <option value="dhcp">DHCP</option>
                    </select>
                  </label>
                  <label className="conn-form-modal__field">
                    IP (público ou NAT)
                    <input className="input mono" value={form.ip_address} onChange={(e) => setForm({ ...form, ip_address: e.target.value })} placeholder="177.x.x.x" />
                  </label>
                  <label className="conn-form-modal__field">
                    Meio de acesso
                    <select className="select" value={form.medium_type} onChange={(e) => setForm({ ...form, medium_type: e.target.value as typeof form.medium_type })}>
                      <option value="">—</option>
                      <option value="fibra">Fibra</option>
                      <option value="radio">Rádio</option>
                      <option value="cabo_utp">Cabo UTP</option>
                    </select>
                  </label>
                  <label className="conn-form-modal__field">
                    Plano de venda
                    <input className="input" value={form.sales_plan} onChange={(e) => setForm({ ...form, sales_plan: e.target.value })} placeholder="ex. 500 Mbps" />
                  </label>
                </div>
              </section>

              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Rede / óptica</h3>
                <div className="conn-form-modal__grid">
                  <label className="conn-form-modal__field">
                    MAC / SN ONU
                    <input className="input mono" value={form.onu_mac_sn} onChange={(e) => setForm({ ...form, onu_mac_sn: e.target.value })} placeholder="AA:BB:CC:DD:EE:FF" />
                  </label>
                  <label className="conn-form-modal__field">
                    Transmissor (OLT)
                    <input className="input" value={form.transmitter} onChange={(e) => setForm({ ...form, transmitter: e.target.value })} />
                  </label>
                  <label className="conn-form-modal__field">
                    RX (dBm)
                    <input className="input mono" value={form.rx_dbm} onChange={(e) => setForm({ ...form, rx_dbm: e.target.value })} placeholder="-22,5" />
                  </label>
                  <label className="conn-form-modal__field">
                    TX (dBm)
                    <input className="input mono" value={form.tx_dbm} onChange={(e) => setForm({ ...form, tx_dbm: e.target.value })} placeholder="2,1" />
                  </label>
                  <label className="conn-form-modal__field">
                    CTO
                    <input className="input" value={form.cto} onChange={(e) => setForm({ ...form, cto: e.target.value })} />
                  </label>
                  <label className="conn-form-modal__field">
                    Porta
                    <input className="input" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} />
                  </label>
                </div>
              </section>

              <section className="conn-form-modal__section">
                <h3 className="conn-form-modal__section-title">Localização (mapa)</h3>
                <div className="conn-form-modal__grid">
                  <label className="conn-form-modal__field">
                    Latitude
                    <span>Decimal com ponto ou vírgula</span>
                    <input className="input mono" value={form.latitude} onChange={(e) => setForm({ ...form, latitude: e.target.value })} placeholder="-23,55052" />
                  </label>
                  <label className="conn-form-modal__field">
                    Longitude
                    <input className="input mono" value={form.longitude} onChange={(e) => setForm({ ...form, longitude: e.target.value })} placeholder="-46,63331" />
                  </label>
                </div>
              </section>
            </div>
            <div className="conn-form-modal__foot">
              <button type="button" className="btn" disabled={saveMut.isPending || checkDupMut.isPending} onClick={() => setFormOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={saveMut.isPending || checkDupMut.isPending}
                onClick={handleSaveClick}
              >
                {saveMut.isPending || checkDupMut.isPending ? "Guardando…" : editId ? "Guardar alterações" : "Criar conexão"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {dupModal ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !saveMut.isPending && setDupModal(null)}>
          <div className="modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 480 }}>
            <h3 style={{ marginTop: 0 }}>Conexão duplicada</h3>
            <p style={{ fontSize: 13, color: "var(--muted)" }}>
              Já existe registo com {conflictSummary(dupModal.login_conflict, dupModal.ip_conflict)}. O que deseja fazer?
            </p>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16 }}>
              <button type="button" className="btn" disabled={saveMut.isPending} onClick={() => setDupModal(null)}>
                Cancelar
              </button>
              <button type="button" className="btn" disabled={saveMut.isPending} onClick={() => confirmDupSave("ignore")}>
                Ignorar
              </button>
              <button type="button" className="btn btn--primary" disabled={saveMut.isPending} onClick={() => confirmDupSave("replace")}>
                Substituir existente
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {importOpen && canMutate ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !importing && setImportOpen(false)}>
          <div className="modal conn-import-modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 440 }}>
            <h3 style={{ marginTop: 0 }}>Importar CSV</h3>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Use o modelo (separador <strong>;</strong>). Importação em lotes de {CONN_IMPORT_BATCH_SIZE} logins.
            </p>
            <div className="field" style={{ marginTop: 12 }}>
              <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer" }}>
                <input type="radio" name="import-dup" checked={importPolicy === "replace"} onChange={() => setImportPolicy("replace")} disabled={importing} />
                Substituir registo existente
              </label>
              <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer", marginTop: 6 }}>
                <input type="radio" name="import-dup" checked={importPolicy === "ignore"} onChange={() => setImportPolicy("ignore")} disabled={importing} />
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
                          {importProgress.processed} / {importProgress.total} logins
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
                    <th>Login</th>
                    <th>Motivo</th>
                  </tr>
                </thead>
                <tbody>
                  {importReport.failed.map((row, idx) => (
                    <tr key={`${row.line ?? row.index ?? idx}-${row.login ?? idx}`}>
                      <td className="mono">{row.line ?? (row.index != null ? row.index + 1 : "—")}</td>
                      <td className="mono">{row.login ?? "—"}</td>
                      <td style={{ color: "var(--danger, #c44)" }}>
                        {row.error}
                        {row.details && typeof row.details === "object" ? (
                          <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 2 }}>
                            {[
                              (row.details as { login_conflict?: ConnConflict }).login_conflict
                                ? `login → #${(row.details as { login_conflict: ConnConflict }).login_conflict.display_number}`
                                : null,
                              (row.details as { ip_conflict?: ConnConflict }).ip_conflict
                                ? `IP → #${(row.details as { ip_conflict: ConnConflict }).ip_conflict.display_number}`
                                : null,
                            ]
                              .filter(Boolean)
                              .join(" · ")}
                          </div>
                        ) : null}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  const text = importReport.failed.map(formatImportFailMessage).join("\n");
                  void navigator.clipboard.writeText(text).then(() => toastOk(pushToast, "Erros copiados."));
                }}
              >
                Copiar erros
              </button>
              <button type="button" className="btn btn--primary" onClick={() => setImportErrorsOpen(false)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {lookupOpen ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !lookupMut.isPending && setLookupOpen(false)}>
          <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 720 }}>
            <h3 style={{ marginTop: 0 }}>Buscar login nas integrações</h3>
            <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
              <input
                className="input mono"
                style={{ flex: "1 1 200px" }}
                placeholder="Login PPPoE / RADIUS"
                value={lookupLogin}
                onChange={(e) => setLookupLogin(e.target.value)}
              />
              <button
                type="button"
                className="btn btn--primary"
                disabled={!lookupLogin.trim() || lookupMut.isPending}
                onClick={() => lookupMut.mutate()}
              >
                {lookupMut.isPending ? "A consultar…" : "Consultar"}
              </button>
            </div>
            <div className="field" style={{ marginBottom: 12 }}>
              <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer", fontSize: 12 }}>
                <input type="checkbox" checked={lookupAll} onChange={(e) => setLookupAll(e.target.checked)} />
                Todas as integrações
              </label>
              {!lookupAll ? (
                <div style={{ marginTop: 8, maxHeight: 120, overflow: "auto", border: "1px solid var(--border)", borderRadius: 6, padding: 8 }}>
                  {integrationOptions.map((ig) => (
                    <label key={ig.id} className="row" style={{ gap: 8, fontSize: 12, marginBottom: 4, cursor: "pointer" }}>
                      <input
                        type="checkbox"
                        checked={lookupIds.includes(ig.id)}
                        onChange={(e) => {
                          setLookupIds((prev) =>
                            e.target.checked ? [...prev, ig.id] : prev.filter((id) => id !== ig.id),
                          );
                        }}
                      />
                      {ig.name}
                    </label>
                  ))}
                </div>
              ) : null}
            </div>
            {lookupMut.data?.results?.map((r) => (
              <div key={r.integration_id} style={{ marginBottom: 12, padding: 10, border: "1px solid var(--border)", borderRadius: 8 }}>
                <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 4 }}>{r.integration_name}</div>
                {!r.ok ? (
                  <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>{r.message || "Sem resultado"}</p>
                ) : r.clients.length === 0 ? (
                  <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>Nenhum cliente encontrado.</p>
                ) : (
                  r.clients.map((client, idx) => (
                    <div key={idx} className="row" style={{ justifyContent: "space-between", alignItems: "center", gap: 8, marginTop: 6 }}>
                      <div style={{ fontSize: 12, minWidth: 0 }}>
                        <strong>{client.name || "—"}</strong>
                        {client.ipv4 ? <span className="mono" style={{ marginLeft: 8 }}>{client.ipv4}</span> : null}
                        {client.services?.[0]?.plano_venda ? (
                          <span style={{ marginLeft: 8, color: "var(--muted)" }}>{client.services[0].plano_venda}</span>
                        ) : null}
                      </div>
                      {canMutate ? (
                        <button type="button" className="btn" style={{ fontSize: 11 }} onClick={() => applyLookupClient(client)}>
                          Usar dados
                        </button>
                      ) : null}
                    </div>
                  ))
                )}
              </div>
            ))}
            <div className="row" style={{ justifyContent: "flex-end", marginTop: 8 }}>
              <button type="button" className="btn" onClick={() => setLookupOpen(false)}>
                Fechar
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <ConfirmModal
        open={!!deleteId}
        title="Excluir conexão"
        message="Remover este login da base de clientes?"
        confirmLabel="Excluir"
        danger
        onCancel={() => setDeleteId(null)}
        onConfirm={() => deleteId && deleteMut.mutate(deleteId)}
      />
    </>
  );
}
