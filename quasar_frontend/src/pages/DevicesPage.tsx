import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { ActionMenu } from "../components/ActionMenu";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch, downloadBlob } from "../lib/api";
import { apiUrl, getStoredApiKey, isAdminUser } from "../lib/auth";
import { PAGE_TOAST_AUTO_MS } from "../lib/pageToast";
import { DeviceReportModal, type DeviceReportTarget } from "../components/DeviceReportModal";
import { DeviceEditBackupTab } from "../components/device/DeviceEditBackupTab";
import { DeviceEditHistoricoTab } from "../components/device/DeviceEditHistoricoTab";

type Device = {
  id: string;
  pop_id?: string | null;
  locality_id?: string | null;
  category: string;
  description: string;
  ip?: string | null;
  network_status: string;
  access_mode?: string | null;
  telemetry_mode?: string | null;
  ping_enabled: boolean;
  telemetry_enabled: boolean;
  operational_mode: string;
  latitude?: number | null;
  longitude?: number | null;
  brand?: string | null;
  model?: string | null;
  mac?: string | null;
  serial_number?: string | null;
  software_version?: string | null;
  hardware_version?: string | null;
  acquired_at?: string | null;
  snmp_community?: string | null;
  mib_folder_path?: string | null;
  telemetry_oid_strategy?: "default" | "manual" | null;
  telemetry_oid_overrides?: {
    cpu_oid?: string;
    cpu_available_oid?: string;
    memory_used_oid?: string;
    memory_size_oid?: string;
    temp_oid?: string;
    uptime_oid?: string;
  } | null;
  max_pons?: number | null;
};

const CATEGORIES = ["Concentrador", "Energia", "Mikrotik", "OLT", "Rádio", "Servidor", "Máquina Virtual", "Outros"] as const;
const OPS = ["Ativo", "Inativo", "Manutenção", "Reserva"] as const;
const NET = ["Normal", "Bridge"] as const;

const ACCESS_MODES = ["Web", "Telnet", "SSH", "Winbox"] as const;
const TELEMETRY_MODES = [
  { value: "SNMP", label: "SNMP (padrão)" },
  { value: "telnet", label: "Telnet" },
  { value: "ssh", label: "SSH" },
] as const;

const DEVICE_BRANDS = ["Mikrotik", "VSOL", "ZTE", "Huawei", "Intelbras", "Datacom", "Nokia", "TP-Link"] as const;

function normalizeBrand(raw: string | null | undefined): string {
  if (!raw?.trim()) return "";
  const t = raw.trim().toLowerCase();
  const found = DEVICE_BRANDS.find((b) => b.toLowerCase() === t);
  return found ?? "";
}

function DeviceBoolIcon({ value, label }: { value: boolean; label: string }) {
  const Icon = value ? Check : X;
  return (
    <span
      className={`devices-overview-table__bool-icon ${value ? "devices-overview-table__bool-icon--yes" : "devices-overview-table__bool-icon--no"}`}
      title={label}
      aria-label={label}
    >
      <Icon size={18} strokeWidth={2} aria-hidden />
    </span>
  );
}

function networkIsBridge(ns: string | undefined | null): boolean {
  return String(ns ?? "").trim() === "Bridge";
}

function normalizeAccessMode(raw: string | null | undefined): string {
  if (!raw?.trim()) return "";
  const t = raw.trim().toLowerCase();
  const found = ACCESS_MODES.find((m) => m.toLowerCase() === t);
  return found ?? "";
}

function parentDirectoryFromFile(fullPath: string): string {
  const t = String(fullPath ?? "").trim();
  if (!t) return "";
  const i = Math.max(t.lastIndexOf("\\"), t.lastIndexOf("/"));
  if (i <= 0) return t;
  return t.slice(0, i);
}

function dispCadastro(s: string | null | undefined): string {
  const t = String(s ?? "").trim();
  return t || "—";
}

type DeviceSortKey =
  | "category"
  | "brand"
  | "description"
  | "ip"
  | "mac"
  | "serial_number"
  | "software_version"
  | "hardware_version"
  | "network_status"
  | "ping"
  | "telemetry"
  | "operational_mode";

function normalizeTelemetryMode(raw: string | null | undefined): "SNMP" | "telnet" | "ssh" {
  if (!raw?.trim()) return "SNMP";
  const t = raw.trim();
  if (/^snmp$/i.test(t)) return "SNMP";
  if (t.toLowerCase() === "telnet") return "telnet";
  if (t.toLowerCase() === "ssh") return "ssh";
  return "SNMP";
}

/** Texto em minúsculas sem marcas diacríticas (á → a) para busca tolerante a acentos. */
function foldAccents(s: string): string {
  return s
    .normalize("NFD")
    .replace(/\p{M}/gu, "")
    .toLowerCase();
}

function FilterIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
    </svg>
  );
}

function PanelSwitch({
  id,
  label,
  checked,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="toggle" htmlFor={id}>
      <span className="toggle__track">
        <input
          id={id}
          type="checkbox"
          role="switch"
          className="toggle__input"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="toggle__thumb" aria-hidden />
      </span>
      <span className="toggle__label">{label}</span>
    </label>
  );
}

type TriFilterValue = "any" | "on" | "off";

function BulkFieldRow({
  label,
  checked,
  onCheck,
  children,
}: {
  label: string;
  checked: boolean;
  onCheck: (v: boolean) => void;
  children: ReactNode;
}) {
  return (
    <div className="field">
      <label className="row" style={{ gap: 8, alignItems: "center", marginBottom: 6 }}>
        <input type="checkbox" checked={checked} onChange={(e) => onCheck(e.target.checked)} />
        <span>{label}</span>
      </label>
      {children}
    </div>
  );
}

function TriFilter({
  label,
  value,
  onChange,
}: {
  label: string;
  value: TriFilterValue;
  onChange: (v: TriFilterValue) => void;
}) {
  const pill = (v: TriFilterValue, text: string) => (
    <button key={v} type="button" className={`filter-pill ${value === v ? "filter-pill--active" : ""}`} onClick={() => onChange(v)}>
      {text}
    </button>
  );
  return (
    <div className="filter-pill-field">
      <span className="filter-pill-field__label">{label}</span>
      <div className="filter-pill-group" role="group" aria-label={label}>
        {pill("any", "Qualquer")}
        {pill("on", "Ligado")}
        {pill("off", "Desligado")}
      </div>
    </div>
  );
}

type BulkEditForm = {
  updatePop: boolean;
  popId: string;
  updateLocality: boolean;
  localityId: string;
  updateCategory: boolean;
  category: string;
  updateNetwork: boolean;
  network_status: string;
  updateOperational: boolean;
  operational_mode: string;
  updatePing: boolean;
  ping_enabled: boolean;
  updateTelemetry: boolean;
  telemetry_enabled: boolean;
  updateBrand: boolean;
  brand: string;
  updateModel: boolean;
  model: string;
  updateAccess: boolean;
  access_mode: string;
  updateTelemetryMode: boolean;
  telemetry_mode: string;
  updateSnmp: boolean;
  snmp_community: string;
};

function emptyBulkForm(): BulkEditForm {
  return {
    updatePop: false,
    popId: "",
    updateLocality: false,
    localityId: "",
    updateCategory: false,
    category: "",
    updateNetwork: false,
    network_status: "Normal",
    updateOperational: false,
    operational_mode: "Ativo",
    updatePing: false,
    ping_enabled: true,
    updateTelemetry: false,
    telemetry_enabled: false,
    updateBrand: false,
    brand: "",
    updateModel: false,
    model: "",
    updateAccess: false,
    access_mode: "",
    updateTelemetryMode: false,
    telemetry_mode: "SNMP",
    updateSnmp: false,
    snmp_community: "",
  };
}

function buildBulkDevicePatch(_d: Device, b: BulkEditForm): Record<string, unknown> | null {
  const body: Record<string, unknown> = {};
  if (b.updatePop) body.pop_id = b.popId.trim() ? b.popId : null;
  if (b.updateLocality) body.locality_id = b.localityId.trim() ? b.localityId : null;
  if (b.updateCategory && b.category.trim()) body.category = b.category.trim();
  if (b.updateNetwork && (b.network_status === "Normal" || b.network_status === "Bridge")) body.network_status = b.network_status;
  if (b.updateOperational && b.operational_mode.trim()) body.operational_mode = b.operational_mode.trim();
  if (b.updateBrand) body.brand = b.brand.trim() ? b.brand.trim() : null;
  if (b.updateModel) body.model = b.model.trim() ? b.model.trim() : null;
  if (b.updateAccess) body.access_mode = b.access_mode.trim() ? b.access_mode.trim() : null;
  if (b.updateTelemetryMode) body.telemetry_mode = b.telemetry_mode.trim() ? b.telemetry_mode.trim() : null;
  if (b.updateSnmp) body.snmp_community = b.snmp_community.trim() ? b.snmp_community.trim() : null;

  let ping: boolean | undefined;
  let tel: boolean | undefined;
  if (b.updatePing) ping = b.ping_enabled;
  if (b.updateTelemetry) tel = b.telemetry_enabled;
  if (tel === true) ping = true;
  if (tel !== undefined) body.telemetry_enabled = tel;
  if (ping !== undefined) body.ping_enabled = ping;
  if (body.telemetry_enabled === true) body.ping_enabled = true;

  if (Object.keys(body).length === 0) return null;
  return body;
}

function emptyForm(): Partial<Device> {
  return {
    category: "OLT",
    description: "",
    ip: "",
    network_status: "Normal",
    ping_enabled: true,
    telemetry_enabled: false,
    operational_mode: "Ativo",
    locality_id: null,
    access_mode: null,
    telemetry_mode: null,
    telemetry_oid_strategy: "default",
    telemetry_oid_overrides: null,
    acquired_at: null,
  };
}

export function DevicesPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const focusEditId = searchParams.get("focus");
  const focusReportId = searchParams.get("report");
  const pops = useQuery({ queryKey: ["pops"], queryFn: () => apiFetch<{ pops: { id: string; description: string }[] }>("/api/v1/pops") });
  const locs = useQuery({
    queryKey: ["commercial-loc"],
    queryFn: () => apiFetch<{ localities: { id: string; name: string }[] }>("/api/v1/commercial/localities"),
  });
  const devices = useQuery({ queryKey: ["devices"], queryFn: () => apiFetch<{ devices: Device[] }>("/api/v1/devices") });

  const [filterText, setFilterText] = useState("");
  const [filterPop, setFilterPop] = useState("");
  const [filterPing, setFilterPing] = useState<TriFilterValue>("any");
  const [filterTelemetry, setFilterTelemetry] = useState<TriFilterValue>("any");
  const [filterNetwork, setFilterNetwork] = useState("");
  const [filterOperational, setFilterOperational] = useState("");
  const [filterCategory, setFilterCategory] = useState("");
  const [filterBrand, setFilterBrand] = useState("");
  const [reloadNote, setReloadNote] = useState<string | null>(null);
  useEffect(() => {
    if (!reloadNote) return;
    const t = window.setTimeout(() => setReloadNote(null), PAGE_TOAST_AUTO_MS);
    return () => window.clearTimeout(t);
  }, [reloadNote]);

  const [actionToast, setActionToast] = useState<{ ok: boolean; text: string } | null>(null);
  useEffect(() => {
    if (!actionToast) return;
    const t = window.setTimeout(() => setActionToast(null), PAGE_TOAST_AUTO_MS);
    return () => window.clearTimeout(t);
  }, [actionToast]);
  const [bulkModalOpen, setBulkModalOpen] = useState(false);
  const [bulkForm, setBulkForm] = useState<BulkEditForm>(() => emptyBulkForm());
  const [bulkRunning, setBulkRunning] = useState(false);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [sortKey, setSortKey] = useState<DeviceSortKey>("description");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const csvImportInputRef = useRef<HTMLInputElement>(null);
  const mibFolderPickerRef = useRef<HTMLInputElement>(null);
  const [mibBrowseNote, setMibBrowseNote] = useState<string | null>(null);

  const [modal, setModal] = useState<"create" | "edit" | null>(null);
  const [editTab, setEditTab] = useState<"cadastro" | "historico" | "backup">("cadastro");
  const [reportModalDevice, setReportModalDevice] = useState<DeviceReportTarget | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<Partial<Device>>(emptyForm());

  useEffect(() => {
    if (modal === "create") setForm(emptyForm());
  }, [modal]);

  useEffect(() => {
    if (!modal) {
      setMibBrowseNote(null);
      setEditTab("cadastro");
    }
  }, [modal]);

  function openMibFolderPicker() {
    setMibBrowseNote(null);
    mibFolderPickerRef.current?.click();
  }

  function onMibFolderPicked(ev: React.ChangeEvent<HTMLInputElement>) {
    const list = ev.target.files;
    if (!list?.length) {
      ev.target.value = "";
      return;
    }
    const f = list[0] as File & { path?: string; webkitRelativePath?: string };
    const abs = typeof f.path === "string" && f.path.trim().length > 0 ? parentDirectoryFromFile(f.path) : "";
    if (abs) {
      setForm((prev) => ({ ...prev, mib_folder_path: abs }));
      setMibBrowseNote("Caminho preenchido a partir da pasta seleccionada no seu computador.");
      ev.target.value = "";
      return;
    }
    const wrp = (f.webkitRelativePath ?? "").trim();
    const root = wrp.split(/[/\\]/)[0]?.trim();
    if (root) {
      setForm((prev) => ({ ...prev, mib_folder_path: `data/mibs/${root}` }));
      setMibBrowseNote(
        "Este navegador não expõe o caminho absoluto da pasta — foi aplicada uma sugestão para o servidor: data/mibs/<nome>/ . Copie os ficheiros para essa pasta no backend ou edite manualmente (ex.: C:\\…)",
      );
    }
    ev.target.value = "";
  }

  const openEdit = useCallback((d: Device) => {
    setEditingId(d.id);
    const acq = d.acquired_at?.trim();
    const acqShort = acq && acq.length >= 10 ? acq.slice(0, 10) : acq ?? "";
    setForm({
      ...d,
      ip: d.ip ?? "",
      locality_id: d.locality_id ?? null,
      acquired_at: acqShort || null,
      access_mode: normalizeAccessMode(d.access_mode) || null,
      telemetry_mode: normalizeTelemetryMode(d.telemetry_mode),
      telemetry_oid_strategy: d.telemetry_oid_strategy ?? "default",
      telemetry_oid_overrides: d.telemetry_oid_overrides ?? null,
      brand: normalizeBrand(d.brand) || null,
      ...(networkIsBridge(d.network_status)
        ? { ping_enabled: false, telemetry_enabled: false }
        : {}),
    });
    setEditTab("cadastro");
    setModal("edit");
  }, []);

  useEffect(() => {
    const id = focusEditId?.trim();
    if (!id || !devices.data?.devices?.length) return;
    if (!canMutate) {
      setSearchParams(
        (prev) => {
          const n = new URLSearchParams(prev);
          n.delete("focus");
          return n;
        },
        { replace: true },
      );
      return;
    }
    const clearFocus = () =>
      setSearchParams(
        (prev) => {
          const n = new URLSearchParams(prev);
          n.delete("focus");
          return n;
        },
        { replace: true },
      );
    const d = devices.data.devices.find((x) => x.id === id);
    if (!d) {
      clearFocus();
      return;
    }
    openEdit(d);
    clearFocus();
  }, [focusEditId, devices.data?.devices, openEdit, setSearchParams, canMutate]);

  useEffect(() => {
    const id = focusReportId?.trim();
    if (!id || !devices.data?.devices?.length) return;
    const clearReport = () =>
      setSearchParams(
        (prev) => {
          const n = new URLSearchParams(prev);
          n.delete("report");
          return n;
        },
        { replace: true },
      );
    const d = devices.data.devices.find((x) => x.id === id);
    if (!d) {
      clearReport();
      return;
    }
    setReportModalDevice({
      id: d.id,
      description: d.description,
      ip: d.ip,
      category: d.category,
      brand: d.brand,
    });
    clearReport();
  }, [focusReportId, devices.data?.devices, setSearchParams]);

  const oltModelsCatalog = useQuery({
    queryKey: ["olt-models-catalog"],
    queryFn: () => apiFetch<{ catalog: Record<string, string[]> }>("/api/v1/settings/olt-vendors/catalog"),
    staleTime: 120_000,
  });

  const oltModelOptions = useMemo(() => {
    const b = normalizeBrand(form.brand);
    if (!b) return [];
    const cat = oltModelsCatalog.data?.catalog ?? {};
    let list = cat[b] ?? [];
    if (list.length === 0) {
      const key = Object.keys(cat).find((k) => k.toLowerCase() === b.toLowerCase());
      if (key) list = cat[key] ?? [];
    }
    const cur = (form.model ?? "").trim();
    if (cur && !list.includes(cur)) return [cur, ...list];
    return list;
  }, [form.brand, form.model, oltModelsCatalog.data]);

  const categoryFilterOptions = useMemo(() => {
    const s = new Set<string>();
    for (const d of devices.data?.devices ?? []) {
      const c = String(d.category ?? "").trim();
      if (c) s.add(c);
    }
    return [...s].sort((a, b) => a.localeCompare(b, "pt"));
  }, [devices.data?.devices]);

  const brandFilterOptions = useMemo(() => {
    const s = new Set<string>();
    for (const d of devices.data?.devices ?? []) {
      const b = String(d.brand ?? "").trim();
      if (b) s.add(b);
    }
    return [...s].sort((a, b) => a.localeCompare(b, "pt"));
  }, [devices.data?.devices]);

  const filteredDevices = useMemo(() => {
    const all = devices.data?.devices ?? [];
    const t = foldAccents(filterText.trim());
    const brandSel = filterBrand.trim();
    const catSel = filterCategory.trim();
    return all.filter((d) => {
      if (filterPop && (d.pop_id ?? "") !== filterPop) return false;
      if (filterPing === "on" && !d.ping_enabled) return false;
      if (filterPing === "off" && d.ping_enabled) return false;
      if (filterTelemetry === "on" && !d.telemetry_enabled) return false;
      if (filterTelemetry === "off" && d.telemetry_enabled) return false;
      if (filterNetwork && d.network_status !== filterNetwork) return false;
      if (filterOperational && d.operational_mode !== filterOperational) return false;
      if (catSel && d.category !== catSel) return false;
      if (brandSel && String(d.brand ?? "").trim() !== brandSel) return false;
      if (!t) return true;
      const hay = foldAccents(
        `${d.description} ${d.category} ${d.brand ?? ""} ${d.ip ?? ""} ${d.id} ${d.mac ?? ""} ${d.serial_number ?? ""} ${d.software_version ?? ""} ${d.hardware_version ?? ""}`,
      );
      return hay.includes(t);
    });
  }, [
    devices.data?.devices,
    filterText,
    filterPop,
    filterPing,
    filterTelemetry,
    filterNetwork,
    filterOperational,
    filterCategory,
    filterBrand,
  ]);

  const sortedDevices = useMemo(() => {
    const out = [...filteredDevices];
    const txt = (v: unknown) => String(v ?? "").trim().toLowerCase();
    const boolNum = (v: boolean) => (v ? 1 : 0);
    out.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "category":
          cmp = txt(a.category).localeCompare(txt(b.category), "pt");
          break;
        case "brand":
          cmp = txt(a.brand).localeCompare(txt(b.brand), "pt");
          break;
        case "description":
          cmp = txt(a.description).localeCompare(txt(b.description), "pt");
          break;
        case "ip":
          cmp = txt(a.ip).localeCompare(txt(b.ip), "pt");
          break;
        case "mac":
          cmp = txt(a.mac).localeCompare(txt(b.mac), "pt");
          break;
        case "serial_number":
          cmp = txt(a.serial_number).localeCompare(txt(b.serial_number), "pt");
          break;
        case "software_version":
          cmp = txt(a.software_version).localeCompare(txt(b.software_version), "pt");
          break;
        case "hardware_version":
          cmp = txt(a.hardware_version).localeCompare(txt(b.hardware_version), "pt");
          break;
        case "network_status":
          cmp = txt(a.network_status).localeCompare(txt(b.network_status), "pt");
          break;
        case "ping":
          cmp = boolNum(a.ping_enabled) - boolNum(b.ping_enabled);
          break;
        case "telemetry":
          cmp = boolNum(a.telemetry_enabled) - boolNum(b.telemetry_enabled);
          break;
        case "operational_mode":
          cmp = txt(a.operational_mode).localeCompare(txt(b.operational_mode), "pt");
          break;
      }
      if (cmp === 0) cmp = txt(a.description).localeCompare(txt(b.description), "pt");
      return sortDir === "asc" ? cmp : -cmp;
    });
    return out;
  }, [filteredDevices, sortDir, sortKey]);

  const sortArrow = (k: DeviceSortKey) => (sortKey !== k ? "↕" : sortDir === "asc" ? "↑" : "↓");
  const toggleSort = (k: DeviceSortKey) => {
    setSortKey((cur) => {
      if (cur === k) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
        return cur;
      }
      setSortDir("asc");
      return k;
    });
  };

  function clearFilters() {
    setFilterText("");
    setFilterPop("");
    setFilterPing("any");
    setFilterTelemetry("any");
    setFilterNetwork("");
    setFilterOperational("");
    setFilterCategory("");
    setFilterBrand("");
  }

  async function runBulkApply() {
    const targets = filteredDevices;
    if (targets.length === 0) {
      setActionToast({ ok: false, text: "Nenhum equipamento visível com os filtros actuais." });
      return;
    }
    let anyField = false;
    const b = bulkForm;
    if (
      b.updatePop ||
      b.updateLocality ||
      b.updateCategory ||
      b.updateNetwork ||
      b.updateOperational ||
      b.updatePing ||
      b.updateTelemetry ||
      b.updateBrand ||
      b.updateModel ||
      b.updateAccess ||
      b.updateTelemetryMode ||
      b.updateSnmp
    ) {
      anyField = true;
    }
    if (!anyField) {
      setActionToast({ ok: false, text: "Marque pelo menos um campo a alterar." });
      return;
    }
    setBulkRunning(true);
    const errors: string[] = [];
    let ok = 0;
    try {
      for (const d of targets) {
        const body = buildBulkDevicePatch(d, b);
        if (!body || Object.keys(body).length === 0) continue;
        try {
          await apiFetch(`/api/v1/devices/${d.id}`, { method: "PATCH", json: body });
          ok++;
        } catch (e) {
          errors.push(`${d.description}: ${(e as Error).message}`);
        }
      }
      await qc.invalidateQueries({ queryKey: ["devices"] });
      if (ok === 0 && errors.length === 0) {
        setActionToast({
          ok: false,
          text: "Nenhuma requisição enviada: confirme valores (ex.: categoria escolhida) ou que os equipamentos visíveis aceitem o PATCH.",
        });
        return;
      }
      if (errors.length) {
        setActionToast({
          ok: false,
          text: `Actualizados ${ok}; ${errors.length} erro(s). ${errors.slice(0, 3).join(" · ")}`,
        });
      } else {
        setActionToast({ ok: true, text: `Alteração em massa: ${ok} equipamento(s) actualizado(s).` });
        setBulkModalOpen(false);
        setBulkForm(emptyBulkForm());
      }
    } finally {
      setBulkRunning(false);
    }
  }

  const save = useMutation({
    mutationFn: async () => {
      const ns = form.network_status || "Normal";
      const bridge = networkIsBridge(ns);
      const isOlt = (form.category ?? "").trim() === "OLT";

      if (form.telemetry_enabled && !form.ping_enabled && !bridge) {
        throw new Error("Telemetria exige ping ativo no equipamento.");
      }
      if (isOlt && !(form.model ?? "").trim()) {
        throw new Error("Selecione o modelo da OLT (Definições → Perfis OLT para cadastrar modelos).");
      }

      const telOn = !!form.telemetry_enabled && !bridge;
      const pingOn = !!form.ping_enabled && !bridge;
      const body = {
        pop_id: form.pop_id || null,
        locality_id: isOlt && form.locality_id && String(form.locality_id).trim() !== "" ? form.locality_id : null,
        category: form.category!,
        description: form.description!,
        ip: form.ip && String(form.ip).trim() !== "" ? String(form.ip).trim() : null,
        network_status: ns,
        access_mode: form.access_mode && String(form.access_mode).trim() !== "" ? String(form.access_mode).trim() : null,
        telemetry_mode: telOn ? normalizeTelemetryMode(form.telemetry_mode ?? "SNMP") : null,
        ping_enabled: pingOn,
        telemetry_enabled: telOn,
        operational_mode: form.operational_mode || "Ativo",
        latitude: form.latitude ?? null,
        longitude: form.longitude ?? null,
        brand: normalizeBrand(form.brand).trim() === "" ? null : normalizeBrand(form.brand),
        model: form.model || null,
        mac: form.mac || null,
        serial_number: form.serial_number || null,
        software_version: form.software_version || null,
        hardware_version: form.hardware_version || null,
        acquired_at: form.acquired_at && String(form.acquired_at).trim() !== "" ? String(form.acquired_at).trim() : null,
        snmp_community: telOn ? (form.snmp_community && String(form.snmp_community).trim() !== "" ? String(form.snmp_community).trim() : null) : null,
        mib_folder_path: form.mib_folder_path && String(form.mib_folder_path).trim() !== "" ? String(form.mib_folder_path).trim() : null,
        telemetry_oid_strategy:
          telOn && String(form.category ?? "").trim() === "Outros"
            ? ((form.telemetry_oid_strategy as "default" | "manual" | null) ?? "default")
            : "default",
        telemetry_oid_overrides:
          telOn &&
          String(form.category ?? "").trim() === "Outros" &&
          ((form.telemetry_oid_strategy as "default" | "manual" | null) ?? "default") === "manual"
            ? {
                cpu_oid: String(form.telemetry_oid_overrides?.cpu_oid ?? "").trim() || undefined,
                cpu_available_oid: String(form.telemetry_oid_overrides?.cpu_available_oid ?? "").trim() || undefined,
                memory_used_oid: String(form.telemetry_oid_overrides?.memory_used_oid ?? "").trim() || undefined,
                memory_size_oid: String(form.telemetry_oid_overrides?.memory_size_oid ?? "").trim() || undefined,
                temp_oid: String(form.telemetry_oid_overrides?.temp_oid ?? "").trim() || undefined,
                uptime_oid: String(form.telemetry_oid_overrides?.uptime_oid ?? "").trim() || undefined,
              }
            : {},
        max_pons: isOlt ? (form.max_pons != null && Number(form.max_pons) > 0 ? Number(form.max_pons) : null) : null,
      };
      if (modal === "create") {
        await apiFetch("/api/v1/devices", { method: "POST", json: body });
      } else if (editingId) {
        await apiFetch(`/api/v1/devices/${editingId}`, { method: "PATCH", json: body });
      }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["devices"] });
      setActionToast({ ok: true, text: modal === "create" ? "Equipamento adicionado com sucesso." : "Equipamento atualizado com sucesso." });
      setModal(null);
      setEditingId(null);
    },
    onError: (e: Error) => setActionToast({ ok: false, text: e.message }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/devices/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["devices"] });
      setActionToast({ ok: true, text: "Equipamento excluído com sucesso." });
    },
    onError: (e: Error) => setActionToast({ ok: false, text: e.message }),
  });

  async function exportCsv() {
    const headers = new Headers();
    const k = getStoredApiKey();
    if (k) headers.set("X-API-Key", k);
    const res = await fetch(apiUrl("/api/v1/devices/export?format=csv"), { headers });
    if (!res.ok) throw new Error(await res.text());
    downloadBlob("devices_export.csv", await res.blob());
  }

  function downloadCsvTemplate() {
    const header = [
      "description",
      "category",
      "ip",
      "network_status",
      "ping_enabled",
      "telemetry_enabled",
      "operational_mode",
      "pop_id",
      "locality_id",
      "brand",
      "model",
      "access_mode",
      "telemetry_mode",
      "latitude",
      "longitude",
      "snmp_community",
      "mib_folder_path",
    ];
    const sample = ["OLT Exemplo", "OLT", "10.0.0.2", "Normal", "true", "true", "Ativo", "", "", "VSOL", "V1600D", "SNMP", "SNMP", "", "", "public", "C:\\mibs"];
    const csv = `${header.join(",")}\n${sample.join(",")}\n`;
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    downloadBlob("modelo_importacao_equipamentos.csv", blob);
  }

  const importCsv = useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append("file", file);
      const headers = new Headers();
      const k = getStoredApiKey();
      if (k) headers.set("X-API-Key", k);
      const res = await fetch(apiUrl("/api/v1/devices/import/csv"), { method: "POST", headers, body: form });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data?.error?.message || `Falha ao importar CSV (${res.status})`);
      return data as { imported: number; failed?: { line: number; error: string }[] };
    },
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: ["devices"] });
      const failedCount = data.failed?.length ?? 0;
      if (failedCount > 0) {
        const first = data.failed?.slice(0, 3).map((x) => `L${x.line}: ${x.error}`).join(" | ");
        setReloadNote(`Importação parcial: ${data.imported} item(ns) importado(s), ${failedCount} falha(s). ${first ?? ""}`);
      } else {
        setReloadNote(`Importação concluída: ${data.imported} item(ns) importado(s).`);
      }
    },
    onError: (e: Error) => setReloadNote(`Erro: ${e.message}`),
  });

  const pingRun = useMutation({
    mutationFn: (id: string) =>
      apiFetch<{
        ok?: boolean;
        latency_ms?: number | null;
        host?: string;
        method?: string;
      }>(`/api/v1/ping/devices/${id}/run?port=443&icmp_only=true`, { method: "POST" }),
    onSuccess: (data) => {
      const ms = data.latency_ms != null ? `${data.latency_ms} ms` : "—";
      const line = `${data.host ?? "?"}: ${data.ok ? "OK" : "FALHOU"} (${data.method ?? "?"}, ${ms})`;
      setActionToast({ ok: !!data.ok, text: `Ping: ${line}` });
    },
    onError: (e: Error) => setActionToast({ ok: false, text: e.message }),
  });

  const telCollect = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/telemetry/devices/${id}/collect`, { method: "POST", json: {} }),
    onSuccess: () => setActionToast({ ok: true, text: "Telemetria solicitada." }),
    onError: (e: Error) => setActionToast({ ok: false, text: e.message }),
  });

  const reloadAllFromDb = useMutation({
    mutationFn: () =>
      apiFetch<{ reloaded?: boolean; device_count?: number }>("/api/v1/monitoring/reload-devices", { method: "POST", json: {} }),
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: ["devices"] });
      await qc.invalidateQueries({ queryKey: ["pops"] });
      await qc.invalidateQueries({ queryKey: ["commercial-loc"] });
      await qc.invalidateQueries({ queryKey: ["map-points"] });
      const n = data?.device_count;
      setReloadNote(typeof n === "number" ? `Lista sincronizada com a base de dados. ${n} equipamento(s).` : "Lista atualizada.");
    },
    onError: (e: Error) => setReloadNote(`Erro: ${e.message}`),
  });

  if (devices.isLoading || pops.isLoading || locs.isLoading) return <p>Carregando…</p>;
  if (devices.isError) return <div className="msg msg--err">{(devices.error as Error).message}</div>;
  if (pops.isError) return <div className="msg msg--err">{(pops.error as Error).message}</div>;

  const formIsBridge = networkIsBridge(form.network_status);
  const formIsOlt = (form.category ?? "").trim() === "OLT";
  const formIsOutros = (form.category ?? "").trim() === "Outros";
  const formTelemetryOIDStrategy = ((form.telemetry_oid_strategy as "default" | "manual" | null) ?? "default");

  return (
    <>
      <div className="devices-page-screen">
      <h1 style={{ marginTop: 0 }}>Equipamentos</h1>
      <div className="devices-toolbar">
        <div className="devices-toolbar__actions">
          {canMutate ? (
            <button type="button" className="btn btn--primary" onClick={() => setModal("create")}>
              Adicionar
            </button>
          ) : null}
          <ActionMenu
            align="start"
            title="Mais opções"
            items={[
              { id: "csv-template", label: "Baixar modelo CSV", onClick: downloadCsvTemplate },
              ...(canMutate
                ? [
                    {
                      id: "csv-import",
                      label: importCsv.isPending ? "Importando CSV…" : "Importar equipamentos por CSV",
                      disabled: importCsv.isPending,
                      onClick: () => csvImportInputRef.current?.click(),
                    },
                    {
                      id: "bulk",
                      label: "Alteração em massa…",
                      onClick: () => {
                        setBulkForm(emptyBulkForm());
                        setBulkModalOpen(true);
                      },
                    },
                  ]
                : []),
              {
                id: "csv-export",
                label: "Exportar CSV",
                onClick: () => {
                  void exportCsv().catch((e) => alert(String(e)));
                },
              },
            ]}
          />
          {canMutate ? (
            <button
              type="button"
              className="btn btn--primary"
              disabled={reloadAllFromDb.isPending}
              onClick={() => {
                setReloadNote(null);
                reloadAllFromDb.mutate();
              }}
            >
              Recarregar
            </button>
          ) : null}
          <button
            type="button"
            className={`btn btn--icon btn--icon-menu${filtersOpen ? " btn--filter-active" : ""}`}
            title={filtersOpen ? "Ocultar filtros" : "Mostrar filtros"}
            aria-label={filtersOpen ? "Ocultar painel de filtros" : "Mostrar painel de filtros"}
            aria-expanded={filtersOpen}
            onClick={() => setFiltersOpen((o) => !o)}
          >
            <FilterIcon />
          </button>
          <input
            className="input devices-toolbar__search"
            aria-label="Busca geral em equipamentos"
            placeholder="Busca geral (texto, IP, MAC, série…)"
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
          />
        </div>
        <PageCountPill label="Equipamentos" count={filteredDevices.length} />
      </div>
      {reloadNote && (
        <div
          className={`page-toast ${reloadNote.startsWith("Erro:") ? "page-toast--err" : "page-toast--ok"}`}
          role="status"
        >
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setReloadNote(null)}>
            ×
          </button>
          {reloadNote}
        </div>
      )}
      <input
        ref={csvImportInputRef}
        type="file"
        accept=".csv,text/csv"
        style={{ display: "none" }}
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) importCsv.mutate(f);
          e.target.value = "";
        }}
      />

      {filtersOpen && (
      <div className="card" style={{ marginBottom: 12, padding: "12px 14px" }}>
        <div
          className="row"
          style={{
            gap: 12,
            alignItems: "flex-end",
            flexWrap: "wrap",
          }}
        >
          <label className="filter-pill-field" style={{ minWidth: 170 }}>
            <span className="filter-pill-field__label">POP</span>
            <select className="select" style={{ width: "100%", minWidth: 170 }} title="Filtrar por POP" value={filterPop} onChange={(e) => setFilterPop(e.target.value)}>
              <option value="">Todos os POPs</option>
              {(pops.data?.pops ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.description}
                </option>
              ))}
            </select>
          </label>
          <label className="filter-pill-field" style={{ minWidth: 170 }}>
            <span className="filter-pill-field__label">Categoria</span>
            <select className="select" style={{ width: "100%", minWidth: 170 }} title="Filtrar por categoria" value={filterCategory} onChange={(e) => setFilterCategory(e.target.value)}>
              <option value="">Todas as categorias</option>
              {categoryFilterOptions.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
          <label className="filter-pill-field" style={{ minWidth: 170 }}>
            <span className="filter-pill-field__label">Marca</span>
            <select className="select" style={{ width: "100%", minWidth: 170 }} title="Filtrar por marca" value={filterBrand} onChange={(e) => setFilterBrand(e.target.value)}>
              <option value="">Todas as marcas</option>
              {brandFilterOptions.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </label>
          <label className="filter-pill-field" style={{ minWidth: 160 }}>
            <span className="filter-pill-field__label">Rede</span>
            <select className="select" style={{ width: "100%" }} title="Filtrar por tipo de rede" value={filterNetwork} onChange={(e) => setFilterNetwork(e.target.value)}>
              <option value="">Todas (Normal e Bridge)</option>
              {NET.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <label className="filter-pill-field" style={{ minWidth: 180 }}>
            <span className="filter-pill-field__label">Modo de operação</span>
            <select className="select" style={{ width: "100%" }} title="Filtrar por modo de operação" value={filterOperational} onChange={(e) => setFilterOperational(e.target.value)}>
              <option value="">Todos os modos</option>
              {OPS.map((o) => (
                <option key={o} value={o}>
                  {o}
                </option>
              ))}
            </select>
          </label>
          <TriFilter label="Ping" value={filterPing} onChange={setFilterPing} />
          <TriFilter label="Telemetria" value={filterTelemetry} onChange={setFilterTelemetry} />
        </div>
        <div className="row" style={{ marginTop: 10, justifyContent: "flex-end" }}>
          <button type="button" className="btn" onClick={() => clearFilters()}>
            Limpar filtros
          </button>
        </div>
      </div>
      )}

      <div className="table-wrap devices-overview-table" style={{ overflowX: "auto", overflowY: "visible" }}>
        <table>
          <thead>
            <tr>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("category")}>
                Categoria {sortArrow("category")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("brand")}>
                Marca {sortArrow("brand")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("description")}>
                Descrição {sortArrow("description")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("ip")}>
                IP {sortArrow("ip")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("mac")}>
                MAC {sortArrow("mac")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("serial_number")}>
                N.º série {sortArrow("serial_number")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("software_version")}>
                Firmware {sortArrow("software_version")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("hardware_version")}>
                Hardware {sortArrow("hardware_version")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("network_status")}>
                Rede {sortArrow("network_status")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("ping")}>
                Ping {sortArrow("ping")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("telemetry")}>
                Tel. {sortArrow("telemetry")}
              </th>
              <th style={{ cursor: "pointer" }} onClick={() => toggleSort("operational_mode")}>
                Modo op. {sortArrow("operational_mode")}
              </th>
              <th style={{ width: 52 }}>{canMutate ? "Ações" : ""}</th>
            </tr>
          </thead>
          <tbody>
            {sortedDevices.map((d) => {
              const br = networkIsBridge(d.network_status);
              return (
              <tr key={d.id}>
                <td>{d.category}</td>
                <td>{d.brand ?? "—"}</td>
                <td className="devices-overview-table__desc" title={d.description}>
                  {d.description}
                </td>
                <td className="devices-overview-table__mono">{d.ip ?? "—"}</td>
                <td className="devices-overview-table__mono devices-overview-table__truncate" title={dispCadastro(d.mac)}>
                  {dispCadastro(d.mac)}
                </td>
                <td className="devices-overview-table__mono devices-overview-table__truncate" title={dispCadastro(d.serial_number)}>
                  {dispCadastro(d.serial_number)}
                </td>
                <td className="devices-overview-table__truncate" title={dispCadastro(d.software_version)}>
                  {dispCadastro(d.software_version)}
                </td>
                <td className="devices-overview-table__truncate" title={dispCadastro(d.hardware_version)}>
                  {dispCadastro(d.hardware_version)}
                </td>
                <td className="devices-overview-table__mono">{d.network_status}</td>
                <td className="devices-overview-table__bool">
                  <DeviceBoolIcon value={d.ping_enabled} label={d.ping_enabled ? "Ping ativo" : "Ping inativo"} />
                </td>
                <td className="devices-overview-table__bool">
                  <DeviceBoolIcon
                    value={d.telemetry_enabled}
                    label={d.telemetry_enabled ? "Telemetria ativa" : "Telemetria inativa"}
                  />
                </td>
                <td>{d.operational_mode}</td>
                <td>
                  {canMutate ? (
                    <ActionMenu
                      align="end"
                      items={[
                        { id: "edit", label: "Editar", onClick: () => openEdit(d) },
                        { id: "ping", label: "Ping", disabled: br || pingRun.isPending, onClick: () => pingRun.mutate(d.id) },
                        { id: "snmp", label: "SNMP", disabled: br || telCollect.isPending, onClick: () => telCollect.mutate(d.id) },
                        {
                          id: "report",
                          label: "Relatório",
                          disabled: br,
                          onClick: () =>
                            setReportModalDevice({
                              id: d.id,
                              description: d.description,
                              ip: d.ip,
                              category: d.category,
                              brand: d.brand,
                            }),
                        },
                        { id: "del", label: "Excluir", danger: true, onClick: () => confirm("Excluir?") && remove.mutate(d.id) },
                      ]}
                    />
                  ) : (
                    <button
                      type="button"
                      className="btn"
                      style={{ fontSize: 11, padding: "4px 8px" }}
                      disabled={br}
                      onClick={() =>
                        setReportModalDevice({
                          id: d.id,
                          description: d.description,
                          ip: d.ip,
                          category: d.category,
                          brand: d.brand,
                        })
                      }
                    >
                      Relatório
                    </button>
                  )}
                </td>
              </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {actionToast && (
        <div className={`page-toast ${actionToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status">
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setActionToast(null)}>
            ×
          </button>
          {actionToast.text}
        </div>
      )}
      {modal && canMutate && (
        <div className="modal-backdrop" onClick={(e) => e.target === e.currentTarget && setModal(null)}>
          <div className="modal modal--wide" onClick={(e) => e.stopPropagation()}>
            <h3>{modal === "create" ? "Novo equipamento" : "Editar equipamento"}</h3>
            {modal === "edit" && editingId && (
              <div className="tabs" style={{ marginBottom: 12, flexWrap: "wrap" }}>
                <button type="button" className={editTab === "cadastro" ? "active" : ""} onClick={() => setEditTab("cadastro")}>
                  Cadastro
                </button>
                <button type="button" className={editTab === "historico" ? "active" : ""} onClick={() => setEditTab("historico")}>
                  Histórico
                </button>
                <button type="button" className={editTab === "backup" ? "active" : ""} onClick={() => setEditTab("backup")}>
                  Backup
                </button>
              </div>
            )}
            {modal === "edit" && editTab === "historico" && editingId && <DeviceEditHistoricoTab deviceId={editingId} />}

            {modal === "edit" && editTab === "backup" && editingId && (
              <DeviceEditBackupTab
                deviceId={editingId}
                deviceLabel={form.description ?? "equipamento"}
                canMutate={canMutate}
              />
            )}

            {(modal === "create" || editTab === "cadastro") && (
            <>
            <p style={{ color: "var(--muted)", fontSize: 12 }}>
              Com telemetria ativa, o ping precisa estar ligado. Em modo <strong>Bridge</strong>, o IP é opcional e ping/telemetria ficam desativados — o servidor ajusta ao salvar, se necessário.
            </p>
            {save.isError && <div className="msg msg--err">{(save.error as Error).message}</div>}

            <div className="device-form-grid">
              <div className="field">
                <label>Categoria</label>
                <select
                  className="select"
                  style={{ width: "100%" }}
                  value={form.category}
                  onChange={(e) => {
                    const category = e.target.value;
                    setForm((f) => ({
                      ...f,
                      category,
                      locality_id: category === "OLT" ? f.locality_id : null,
                      max_pons: category === "OLT" ? f.max_pons ?? null : null,
                      telemetry_oid_strategy: category === "Outros" ? (f.telemetry_oid_strategy ?? "default") : "default",
                      telemetry_oid_overrides: category === "Outros" ? f.telemetry_oid_overrides ?? {} : {},
                    }));
                  }}
                >
                  {CATEGORIES.map((c) => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label>Status rede</label>
                <select
                  className="select"
                  style={{ width: "100%" }}
                  value={form.network_status}
                  onChange={(e) => {
                    const network_status = e.target.value;
                    if (network_status === "Bridge") {
                      setForm((f) => ({
                        ...f,
                        network_status: "Bridge",
                        ping_enabled: false,
                        telemetry_enabled: false,
                      }));
                      return;
                    }
                    setForm((f) => ({ ...f, network_status }));
                  }}
                >
                  {NET.map((n) => (
                    <option key={n} value={n}>{n === "Normal" ? "Normal (IP obrigatório)" : n}</option>
                  ))}
                </select>
              </div>

              <div className="field field--full">
                <label>Descrição *</label>
                <input className="input" style={{ width: "100%" }} value={form.description ?? ""} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </div>
              <div className="field field--full">
                <label>{formIsBridge ? "IP (opcional em Bridge)" : "IP"}</label>
                <input className="input mono" style={{ width: "100%" }} value={(form.ip as string) ?? ""} onChange={(e) => setForm({ ...form, ip: e.target.value })} />
              </div>

              {!formIsOlt ? (
                <div className="field field--full">
                  <label>POP</label>
                  <select className="select" style={{ width: "100%" }} value={form.pop_id ?? ""} onChange={(e) => setForm({ ...form, pop_id: e.target.value || null })}>
                    <option value="">—</option>
                    {(pops.data?.pops ?? []).map((p) => (
                      <option key={p.id} value={p.id}>{p.description}</option>
                    ))}
                  </select>
                </div>
              ) : (
                <>
                  <div className="field">
                    <label>POP</label>
                    <select className="select" style={{ width: "100%" }} value={form.pop_id ?? ""} onChange={(e) => setForm({ ...form, pop_id: e.target.value || null })}>
                      <option value="">—</option>
                      {(pops.data?.pops ?? []).map((p) => (
                        <option key={p.id} value={p.id}>{p.description}</option>
                      ))}
                    </select>
                  </div>
                  <div className="field">
                    <label>Localidade (base comercial, só OLT)</label>
                    <select
                      className="select"
                      style={{ width: "100%" }}
                      value={form.locality_id ?? ""}
                      onChange={(e) => setForm({ ...form, locality_id: e.target.value || null })}
                    >
                      <option value="">—</option>
                      {(locs.data?.localities ?? []).map((l) => (
                        <option key={l.id} value={l.id}>
                          {l.name}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="field">
                    <label>Quantidade máxima de PONs</label>
                    <input
                      type="number"
                      min={1}
                      step={1}
                      className="input"
                      style={{ width: "100%" }}
                      placeholder="Ex.: 4"
                      value={form.max_pons ?? ""}
                      onChange={(e) => {
                        const raw = e.target.value.trim();
                        if (raw === "") {
                          setForm({ ...form, max_pons: null });
                          return;
                        }
                        const n = Number(raw);
                        setForm({ ...form, max_pons: Number.isFinite(n) ? Math.max(1, Math.trunc(n)) : null });
                      }}
                    />
                  </div>
                </>
              )}

              <div className="field field--full">
                <label>Modo de acesso</label>
                <select
                  className="select"
                  style={{ width: "100%" }}
                  value={normalizeAccessMode(form.access_mode)}
                  onChange={(e) => setForm({ ...form, access_mode: e.target.value.trim() === "" ? null : e.target.value })}
                >
                  <option value="">—</option>
                  {ACCESS_MODES.map((m) => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </select>
              </div>

              <div className="field">
                <label>Modo operacional</label>
                <select className="select" style={{ width: "100%" }} value={form.operational_mode} onChange={(e) => setForm({ ...form, operational_mode: e.target.value })}>
                  {OPS.map((o) => (
                    <option key={o} value={o}>{o}</option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label>Data aquisição (YYYY-MM-DD)</label>
                <input
                  className="input mono"
                  style={{ width: "100%" }}
                  placeholder="opcional"
                  value={form.acquired_at ?? ""}
                  onChange={(e) => setForm({ ...form, acquired_at: e.target.value.trim() || null })}
                />
              </div>

              <div className="toggle-row">
                <PanelSwitch
                  id="device-ping"
                  label="Monitorar com ping"
                  checked={formIsBridge ? false : !!form.ping_enabled}
                  disabled={formIsBridge}
                  onChange={(pingOn) =>
                    setForm((f) => ({
                      ...f,
                      ping_enabled: pingOn,
                      ...(pingOn ? {} : { telemetry_enabled: false }),
                    }))
                  }
                />
                <PanelSwitch
                  id="device-tel"
                  label="Telemetria"
                  checked={formIsBridge ? false : !!form.telemetry_enabled}
                  disabled={formIsBridge}
                  onChange={(telOn) =>
                    setForm((f) => ({
                      ...f,
                      telemetry_enabled: telOn,
                      ping_enabled: telOn ? true : f.ping_enabled,
                      telemetry_mode: telOn ? normalizeTelemetryMode(f.telemetry_mode ?? "SNMP") : f.telemetry_mode,
                    }))
                  }
                />
              </div>
              {formIsBridge && (
                <p style={{ gridColumn: "1 / -1", fontSize: 12, color: "var(--muted)", margin: "-0.25rem 0 0.5rem" }}>
                  Estado Bridge: ping e telemetria não estão disponíveis.
                </p>
              )}

              {!formIsBridge && form.telemetry_enabled ? (
                <>
                  <div className="field">
                    <label>Modo de telemetria</label>
                    <select
                      className="select"
                      style={{ width: "100%" }}
                      value={normalizeTelemetryMode(form.telemetry_mode ?? "SNMP")}
                      onChange={(e) =>
                        setForm({
                          ...form,
                          telemetry_mode: e.target.value as "SNMP" | "telnet" | "ssh",
                        })
                      }
                    >
                      {TELEMETRY_MODES.map((m) => (
                        <option key={m.value} value={m.value}>{m.label}</option>
                      ))}
                    </select>
                  </div>
                  <div className="field">
                    <label>SNMP community (opcional)</label>
                    <input className="input mono" style={{ width: "100%" }} value={form.snmp_community ?? ""} onChange={(e) => setForm({ ...form, snmp_community: e.target.value })} />
                  </div>
                  {formIsOutros && normalizeTelemetryMode(form.telemetry_mode ?? "SNMP") === "SNMP" && (
                    <>
                      <div className="field field--full">
                        <label>OIDs de telemetria para categoria Outros</label>
                        <select
                          className="select"
                          style={{ width: "100%" }}
                          value={formTelemetryOIDStrategy}
                          onChange={(e) =>
                            setForm((f) => ({
                              ...f,
                              telemetry_oid_strategy: e.target.value as "default" | "manual",
                              telemetry_oid_overrides: f.telemetry_oid_overrides ?? {},
                            }))
                          }
                        >
                          <option value="default">Usar OIDs padrão do sistema</option>
                          <option value="manual">Inserir OIDs manualmente neste equipamento</option>
                        </select>
                      </div>
                      {formTelemetryOIDStrategy === "manual" && (
                        <>
                          <div className="field">
                            <label>CPU (uso)</label>
                            <input
                              className="input mono"
                              style={{ width: "100%" }}
                              value={form.telemetry_oid_overrides?.cpu_oid ?? ""}
                              onChange={(e) =>
                                setForm((f) => ({
                                  ...f,
                                  telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), cpu_oid: e.target.value },
                                }))
                              }
                            />
                          </div>
                          <div className="field">
                            <label>CPU disponível (idle)</label>
                            <input
                              className="input mono"
                              style={{ width: "100%" }}
                              value={form.telemetry_oid_overrides?.cpu_available_oid ?? ""}
                              onChange={(e) =>
                                setForm((f) => ({
                                  ...f,
                                  telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), cpu_available_oid: e.target.value },
                                }))
                              }
                            />
                          </div>
                          <div className="field">
                            <label>Memória usada</label>
                            <input
                              className="input mono"
                              style={{ width: "100%" }}
                              value={form.telemetry_oid_overrides?.memory_used_oid ?? ""}
                              onChange={(e) =>
                                setForm((f) => ({
                                  ...f,
                                  telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), memory_used_oid: e.target.value },
                                }))
                              }
                            />
                          </div>
                          <div className="field">
                            <label>Memória total</label>
                            <input
                              className="input mono"
                              style={{ width: "100%" }}
                              value={form.telemetry_oid_overrides?.memory_size_oid ?? ""}
                              onChange={(e) =>
                                setForm((f) => ({
                                  ...f,
                                  telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), memory_size_oid: e.target.value },
                                }))
                              }
                            />
                          </div>
                          <div className="field">
                            <label>Temperatura</label>
                            <input
                              className="input mono"
                              style={{ width: "100%" }}
                              value={form.telemetry_oid_overrides?.temp_oid ?? ""}
                              onChange={(e) =>
                                setForm((f) => ({
                                  ...f,
                                  telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), temp_oid: e.target.value },
                                }))
                              }
                            />
                          </div>
                          <div className="field">
                            <label>Uptime</label>
                            <input
                              className="input mono"
                              style={{ width: "100%" }}
                              value={form.telemetry_oid_overrides?.uptime_oid ?? ""}
                              onChange={(e) =>
                                setForm((f) => ({
                                  ...f,
                                  telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), uptime_oid: e.target.value },
                                }))
                              }
                            />
                          </div>
                        </>
                      )}
                    </>
                  )}
                  <div className="field field--full">
                    <label>Pasta MIBs (.txt/.csv) para discovery — caminho na máquina do servidor ou relativo ao backend (opcional)</label>
                    <div className="row" style={{ gap: 8, alignItems: "stretch", flexWrap: "wrap" }}>
                      <input
                        className="input mono"
                        style={{ flex: "1 1 200px", minWidth: 0 }}
                        title="Ex.: data/mibs/marc ou caminho absoluto no servidor onde corre o backend"
                        value={form.mib_folder_path ?? ""}
                        onChange={(e) => {
                          setMibBrowseNote(null);
                          setForm({ ...form, mib_folder_path: e.target.value });
                        }}
                      />
                      <input
                        ref={mibFolderPickerRef}
                        type="file"
                        multiple
                        style={{ display: "none" }}
                        {...({ webkitdirectory: "", directory: "" } as Record<string, string>)}
                        onChange={onMibFolderPicked}
                      />
                      <button type="button" className="btn" style={{ flexShrink: 0 }} onClick={() => openMibFolderPicker()}>
                        Procurar…
                      </button>
                    </div>
                    {mibBrowseNote && (
                      <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0", lineHeight: 1.35 }}>
                        {mibBrowseNote}
                      </p>
                    )}
                  </div>
                </>
              ) : null}

              <div className="field">
                <label>Latitude</label>
                <input className="input" style={{ width: "100%" }} value={form.latitude ?? ""} onChange={(e) => setForm({ ...form, latitude: e.target.value === "" ? null : Number(e.target.value) })} />
              </div>
              <div className="field">
                <label>Longitude</label>
                <input className="input" style={{ width: "100%" }} value={form.longitude ?? ""} onChange={(e) => setForm({ ...form, longitude: e.target.value === "" ? null : Number(e.target.value) })} />
              </div>
              <div className="field">
                <label>Marca</label>
                <select
                  className="select"
                  style={{ width: "100%" }}
                  value={normalizeBrand(form.brand)}
                  onChange={(e) => {
                    const brand = e.target.value.trim() === "" ? null : e.target.value;
                    setForm((f) => ({
                      ...f,
                      brand,
                      model: (f.category ?? "").trim() === "OLT" ? null : f.model,
                    }));
                  }}
                >
                  <option value="">—</option>
                  {DEVICE_BRANDS.map((b) => (
                    <option key={b} value={b}>{b}</option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label>Modelo</label>
                {formIsOlt ? (
                  <>
                    <select
                      className="select"
                      style={{ width: "100%" }}
                      value={form.model ?? ""}
                      disabled={!normalizeBrand(form.brand)}
                      onChange={(e) => setForm({ ...form, model: e.target.value.trim() === "" ? null : e.target.value })}
                    >
                      <option value="">
                        {oltModelOptions.length
                          ? "— escolher modelo —"
                          : "Cadastre modelos em Definições → Perfis OLT"}
                      </option>
                      {oltModelOptions.map((m) => (
                        <option key={m} value={m}>
                          {m}
                        </option>
                      ))}
                    </select>
                    {formIsOlt && normalizeBrand(form.brand) && oltModelOptions.length === 0 && !oltModelsCatalog.isLoading && (
                      <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }}>
                        Nenhum modelo para esta marca. Crie em <strong>Definições → Perfis OLT</strong>.
                      </p>
                    )}
                  </>
                ) : (
                  <input
                    className="input"
                    placeholder="Modelo"
                    style={{ width: "100%" }}
                    value={form.model ?? ""}
                    onChange={(e) => setForm({ ...form, model: e.target.value })}
                  />
                )}
              </div>
              <div className="field">
                <label>MAC</label>
                <input className="input mono" style={{ width: "100%" }} value={form.mac ?? ""} onChange={(e) => setForm({ ...form, mac: e.target.value })} />
              </div>
              <div className="field">
                <label>Número de série</label>
                <input className="input mono" style={{ width: "100%" }} value={form.serial_number ?? ""} onChange={(e) => setForm({ ...form, serial_number: e.target.value })} />
              </div>
              <div className="field">
                <label>Versão firmware / software</label>
                <input className="input" style={{ width: "100%" }} value={form.software_version ?? ""} onChange={(e) => setForm({ ...form, software_version: e.target.value })} />
              </div>
              <div className="field">
                <label>Versão hardware</label>
                <input className="input" style={{ width: "100%" }} value={form.hardware_version ?? ""} onChange={(e) => setForm({ ...form, hardware_version: e.target.value })} />
              </div>
            </div>

            <div className="row" style={{ marginTop: 12 }}>
              <button type="button" className="btn" onClick={() => setModal(null)}>Cancelar</button>
              <button type="button" className="btn btn--primary" disabled={save.isPending || !form.description?.trim()} onClick={() => save.mutate()}>
                Salvar
              </button>
            </div>
            </>
            )}

            {modal === "edit" && editTab !== "cadastro" && (
              <div className="row" style={{ marginTop: 12 }}>
                <button type="button" className="btn" onClick={() => setModal(null)}>Fechar</button>
              </div>
            )}
          </div>
        </div>
      )}
      </div>

      {bulkModalOpen && (
        <div
          className="modal-backdrop"
          role="presentation"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !bulkRunning) {
              setBulkModalOpen(false);
              setBulkForm(emptyBulkForm());
            }
          }}
        >
          <div
            className="modal modal--wide"
            role="dialog"
            aria-modal="true"
            aria-labelledby="bulk-devices-title"
            onMouseDown={(e) => e.stopPropagation()}
            style={{ maxHeight: "92vh", display: "flex", flexDirection: "column" }}
          >
            <h3 id="bulk-devices-title" style={{ marginTop: 0 }}>
              Alteração em massa
            </h3>
            <p style={{ color: "var(--muted)", fontSize: 13, marginTop: 0 }}>
              Aplica <strong>PATCH</strong> aos <strong>{filteredDevices.length}</strong> equipamento(s) que correspondem aos filtros actuais. Marque cada campo que pretende alterar e preencha o valor; os restantes mantêm-se na base de dados.
            </p>
            <div style={{ overflow: "auto", flex: 1, paddingRight: 4 }}>
              <div className="device-form-grid" style={{ marginTop: 8 }}>
                <BulkFieldRow
                  label="POP"
                  checked={bulkForm.updatePop}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updatePop: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updatePop}
                    value={bulkForm.popId}
                    onChange={(e) => setBulkForm((f) => ({ ...f, popId: e.target.value }))}
                  >
                    <option value="">— sem POP —</option>
                    {(pops.data?.pops ?? []).map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.description}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Localidade (OLT)"
                  checked={bulkForm.updateLocality}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateLocality: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateLocality}
                    value={bulkForm.localityId}
                    onChange={(e) => setBulkForm((f) => ({ ...f, localityId: e.target.value }))}
                  >
                    <option value="">— sem localidade —</option>
                    {(locs.data?.localities ?? []).map((l) => (
                      <option key={l.id} value={l.id}>
                        {l.name}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Categoria"
                  checked={bulkForm.updateCategory}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateCategory: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateCategory}
                    value={bulkForm.category}
                    onChange={(e) => setBulkForm((f) => ({ ...f, category: e.target.value }))}
                  >
                    <option value="">— escolher —</option>
                    {CATEGORIES.map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Rede (Normal / Bridge)"
                  checked={bulkForm.updateNetwork}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateNetwork: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateNetwork}
                    value={bulkForm.network_status}
                    onChange={(e) => setBulkForm((f) => ({ ...f, network_status: e.target.value }))}
                  >
                    {NET.map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Modo de operação"
                  checked={bulkForm.updateOperational}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateOperational: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateOperational}
                    value={bulkForm.operational_mode}
                    onChange={(e) => setBulkForm((f) => ({ ...f, operational_mode: e.target.value }))}
                  >
                    {OPS.map((o) => (
                      <option key={o} value={o}>
                        {o}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Ping ativo"
                  checked={bulkForm.updatePing}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updatePing: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updatePing}
                    value={bulkForm.ping_enabled ? "1" : "0"}
                    onChange={(e) => setBulkForm((f) => ({ ...f, ping_enabled: e.target.value === "1" }))}
                  >
                    <option value="1">Sim</option>
                    <option value="0">Não</option>
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Telemetria ativa"
                  checked={bulkForm.updateTelemetry}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateTelemetry: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateTelemetry}
                    value={bulkForm.telemetry_enabled ? "1" : "0"}
                    onChange={(e) => setBulkForm((f) => ({ ...f, telemetry_enabled: e.target.value === "1" }))}
                  >
                    <option value="1">Sim</option>
                    <option value="0">Não</option>
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Modo de telemetria"
                  checked={bulkForm.updateTelemetryMode}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateTelemetryMode: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateTelemetryMode}
                    value={bulkForm.telemetry_mode}
                    onChange={(e) => setBulkForm((f) => ({ ...f, telemetry_mode: e.target.value }))}
                  >
                    {TELEMETRY_MODES.map((m) => (
                      <option key={m.value} value={m.value}>
                        {m.label}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Modo de acesso"
                  checked={bulkForm.updateAccess}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateAccess: v }))}
                >
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateAccess}
                    value={bulkForm.access_mode}
                    onChange={(e) => setBulkForm((f) => ({ ...f, access_mode: e.target.value }))}
                  >
                    <option value="">— limpar —</option>
                    {ACCESS_MODES.map((m) => (
                      <option key={m} value={m}>
                        {m}
                      </option>
                    ))}
                  </select>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Marca"
                  checked={bulkForm.updateBrand}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateBrand: v }))}
                >
                  <input
                    className="input"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateBrand}
                    list="bulk-brand-suggestions"
                    value={bulkForm.brand}
                    onChange={(e) => setBulkForm((f) => ({ ...f, brand: e.target.value }))}
                  />
                  <datalist id="bulk-brand-suggestions">
                    {brandFilterOptions.map((b) => (
                      <option key={b} value={b} />
                    ))}
                  </datalist>
                </BulkFieldRow>
                <BulkFieldRow
                  label="Modelo"
                  checked={bulkForm.updateModel}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateModel: v }))}
                >
                  <input
                    className="input"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateModel}
                    value={bulkForm.model}
                    onChange={(e) => setBulkForm((f) => ({ ...f, model: e.target.value }))}
                  />
                </BulkFieldRow>
                <BulkFieldRow
                  label="Comunidade SNMP"
                  checked={bulkForm.updateSnmp}
                  onCheck={(v) => setBulkForm((f) => ({ ...f, updateSnmp: v }))}
                >
                  <input
                    className="input mono"
                    style={{ width: "100%" }}
                    disabled={!bulkForm.updateSnmp}
                    value={bulkForm.snmp_community}
                    onChange={(e) => setBulkForm((f) => ({ ...f, snmp_community: e.target.value }))}
                  />
                </BulkFieldRow>
              </div>
            </div>
            <div className="row" style={{ marginTop: 14, justifyContent: "flex-end", gap: 8, flexShrink: 0 }}>
              <button
                type="button"
                className="btn"
                disabled={bulkRunning}
                onClick={() => {
                  setBulkModalOpen(false);
                  setBulkForm(emptyBulkForm());
                }}
              >
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" disabled={bulkRunning || filteredDevices.length === 0} onClick={() => void runBulkApply()}>
                {bulkRunning ? "A aplicar…" : "Aplicar a todos os visíveis"}
              </button>
            </div>
          </div>
        </div>
      )}

      {reportModalDevice && (
        <DeviceReportModal device={reportModalDevice} onClose={() => setReportModalDevice(null)} />
      )}
    </>
  );
}
