import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Pencil, Plus, Trash2 } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { OltMetricsOidTable, type OltMetricFieldMeta } from "./OltMetricsOidTable";

/** Textarea à largura do modal; altura acompanha o conteúdo até um máximo. */
function OltCmdTextarea({
  value,
  onChange,
  placeholder,
  id,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  id?: string;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "0px";
    const next = Math.min(Math.max(el.scrollHeight, 72), 360);
    el.style.height = `${next}px`;
  }, [value]);
  return (
    <textarea
      ref={ref}
      id={id}
      className="input mono olt-cmd-textarea"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      rows={3}
      spellCheck={false}
    />
  );
}

type OltCollectionStep = {
  id?: string;
  method: string;
  enabled?: boolean;
  oid?: string;
  oid_field?: string;
  oids?: string[];
  store_as?: string;
  command?: string;
  pre_commands?: string[];
  parser?: string;
  params?: Record<string, unknown>;
};

type OltOnuReportCommands = {
  enabled?: boolean;
  monitor_online_only?: boolean;
  max_onus_per_cycle?: number;
  pre_commands?: string[];
  command?: string;
  commands?: string[];
  serial_search_command?: string;
  onu_authorize_command?: string;
  onu_deauthorize_command?: string;
  unauthorized_onu_query_command?: string;
  unauthorized_onu_pre_commands?: string[];
  authorize_vlan?: string;
  authorize_onu_type?: string;
  authorize_name?: string;
  authorize_vlan_snmp_oid?: string;
  authorize_vlan_catalog?: OltAuthorizeVlanEntry[];
};

type OltAuthorizeVlanEntry = {
  vid: number;
  name?: string;
  description?: string;
  pon?: number;
  ignored?: boolean;
};

const DEFAULT_ZTE_VLAN_SNMP_OID = "1.3.6.1.4.1.3902.1082.40.50.2.1.2";

const PROFILE_PLACEHOLDERS = ["{pon}", "{onu}", "{serial}", "{gpon_onu}", "{vlan}", "{onu_type}", "{name}"] as const;

const OLT_BRAND_LOGOS: Array<{ key: string; label: string; file: string }> = [
  { key: "zte", label: "ZTE", file: "zte.png" },
  { key: "datacom", label: "DATACOM", file: "datacom.png" },
  { key: "intelbras", label: "Intelbras", file: "intelbras.png" },
  { key: "cisco", label: "Cisco", file: "cisco.png" },
  { key: "juniper", label: "Juniper", file: "juniper.png" },
  { key: "huawei", label: "Huawei", file: "huawei.png" },
  { key: "vsol", label: "VSOL", file: "vsol.png" },
];

type OltEditSection =
  | "geral"
  | "snmp-onu"
  | "snmp-pon"
  | "telnet-onu"
  | "telnet-unauth"
  | "telnet-auth"
  | "telnet-pon"
  | "advanced"
  | "vars";

const OLT_EDIT_SECTIONS: Array<{ id: OltEditSection; label: string }> = [
  { id: "geral", label: "Informações gerais" },
  { id: "snmp-onu", label: "Métricas ONU (OIDs)" },
  { id: "snmp-pon", label: "Métricas PON (OIDs)" },
  { id: "telnet-onu", label: "Telnet — relatório ONU" },
  { id: "telnet-unauth", label: "Telnet — não autorizadas" },
  { id: "telnet-auth", label: "Telnet — autorizar / VLAN" },
  { id: "telnet-pon", label: "Telnet — PON/SFP" },
  { id: "advanced", label: "Opções avançadas" },
  { id: "vars", label: "Variáveis" },
];

function brandMatchesFilter(brand: string, filter: string): boolean {
  if (!filter) return true;
  return brand.toLowerCase().includes(filter.toLowerCase());
}

function profileDescription(brand: string, model: string): string {
  const b = brand.trim();
  const m = model.trim();
  if (!b) return m;
  if (m.toLowerCase().includes(b.toLowerCase())) return m;
  return `${b} ${m}`;
}

type OltPonTelnetCommands = {
  enabled?: boolean;
  max_pons_per_cycle?: number;
  pre_commands?: string[];
  commands?: string[];
};

type OltOnuMetricDef = {
  enabled?: boolean;
  oid?: string;
  value_divisor?: number;
  online_values?: number[];
  offline_values?: number[];
  status_mode?: "pon_onu_suffix" | "if_mib_index" | "pon_online_offline" | "rx_power_threshold";
  offline_rx_dbm?: number;
  ifdescr_oid?: string;
  ifname_oid?: string;
  ifoper_oid?: string;
  online_count_oid?: string;
  offline_count_oid?: string;
};

type OltMetricsForm = Record<string, OltOnuMetricDef>;

const OLT_METRIC_FIELDS: OltMetricFieldMeta[] = [
  {
    key: "serial",
    label: "Número de série",
    shortDesc: "Serial da ONU na OLT",
    hint: "OID da tabela SNMP. O sistema faz snmpwalk e lê .PON.ONU (ex.: …2.1.5.3.10 = PON 3, ONU 10).",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5",
    entity: "ONU",
    unit: "—",
    typeLabel: "String",
  },
  {
    key: "status",
    label: "Status (online / offline)",
    shortDesc: "Estado operacional da ONU",
    hint: "VSOL fase: …1.1.1.1.5 com sufixo .PON.ONU (ex. …5.1.5 = PON 1 ONU 5). Online=3 (working).",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5",
    entity: "ONU",
    unit: "—",
    typeLabel: "Status",
    hasStatusValues: true,
  },
  {
    key: "rx_power",
    label: "RX da ONU",
    shortDesc: "Potência óptica recebida",
    hint: "VSOL Pirapetinga: 1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7 (não use .3 temperatura nem .2 índice). Sufixo .PON.ONU — ex. …3.1.7.1.1 = PON 1 ONU 1.",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7",
    entity: "ONU",
    unit: "dBm",
    typeLabel: "Gauge",
  },
  {
    key: "tx_power",
    label: "TX da ONU",
    shortDesc: "Potência óptica transmitida",
    hint: "Potência transmitida pela ONU. OID da tabela SNMP; sufixo .PON.ONU.",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6",
    entity: "ONU",
    unit: "dBm",
    typeLabel: "Gauge",
  },
  {
    key: "temperature",
    label: "Temperatura",
    shortDesc: "Temperatura do módulo óptico",
    hint: "OID da tabela de temperatura da ONU.",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.3",
    entity: "ONU",
    unit: "°C",
    typeLabel: "Gauge",
  },
  {
    key: "model",
    label: "Modelo da ONU",
    shortDesc: "Modelo reportado pela ONU",
    hint: "OID da tabela de modelo (ex.: …2.1.6.3.10 = modelo da ONU 10 na PON 3).",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6",
    entity: "ONU",
    unit: "—",
    typeLabel: "String",
  },
  {
    key: "vlan",
    label: "VLAN da ONU",
    shortDesc: "VLAN padrão da porta",
    hint: "VLAN padrão da porta (gOnuCfgPortVlanDefVlan). Walk com sufixo .PON.ONU; na VSOL pode ser necessário walk em 1.3.6.1.4.1.37950.1.1.6.1.1.7.5.8.",
    placeholder: "1.3.6.1.4.1.37950.1.1.6.1.1.7.5.8",
    entity: "ONU",
    unit: "VID",
    typeLabel: "Integer",
  },
];

const OLT_PON_METRIC_FIELDS: OltMetricFieldMeta[] = [
  {
    key: "pon_status",
    label: "Status da PON",
    shortDesc: "Estado operacional da porta PON",
    hint: "Status por porta PON (ex.: ifOperStatus por ifIndex da PON). Configure valores online/offline.",
    hasStatusMode: true,
    placeholder: "1.3.6.1.2.1.2.2.1.8",
    hasStatusValues: true,
    entity: "PON",
    unit: "—",
    typeLabel: "Status",
  },
  {
    key: "pon_rx_power",
    label: "RX da PON",
    shortDesc: "Potência óptica recebida na OLT",
    hint: "Potência recebida na porta PON da OLT. OID da tabela SNMP; sufixo apenas .PON (sem ONU).",
    placeholder: "",
    supportsDivisor: true,
    entity: "PON",
    unit: "dBm",
    typeLabel: "Gauge",
  },
  {
    key: "pon_tx_power",
    label: "TX da PON",
    shortDesc: "Potência óptica transmitida pela OLT",
    hint: "Potência transmitida na porta PON da OLT. OID da tabela SNMP; sufixo apenas .PON.",
    placeholder: "",
    supportsDivisor: true,
    entity: "PON",
    unit: "dBm",
    typeLabel: "Gauge",
  },
  {
    key: "pon_voltage",
    label: "Voltagem da PON",
    shortDesc: "Tensão do módulo óptico da PON",
    hint: "Voltagem por porta PON da OLT. OID da tabela SNMP; sufixo apenas .PON.",
    placeholder: "",
    supportsDivisor: true,
    entity: "PON",
    unit: "V",
    typeLabel: "Gauge",
  },
  {
    key: "pon_current",
    label: "Corrente da PON",
    shortDesc: "Corrente do laser / módulo óptico",
    hint: "Corrente (amperagem) por porta PON da OLT. OID da tabela SNMP; sufixo apenas .PON.",
    placeholder: "",
    supportsDivisor: true,
    entity: "PON",
    unit: "mA",
    typeLabel: "Gauge",
  },
  {
    key: "pon_temperature",
    label: "Temperatura da PON",
    shortDesc: "Temperatura do módulo óptico da PON",
    hint: "Temperatura por porta PON da OLT. OID da tabela SNMP; sufixo apenas .PON.",
    placeholder: "",
    supportsDivisor: true,
    entity: "PON",
    unit: "°C",
    typeLabel: "Gauge",
  },
];

const OLT_ALL_METRIC_FIELDS = [...OLT_METRIC_FIELDS, ...OLT_PON_METRIC_FIELDS];

function defaultMetricsForm(): OltMetricsForm {
  const out: OltMetricsForm = {};
  for (const f of OLT_ALL_METRIC_FIELDS) {
    out[f.key] = {
      enabled: f.key === "status" || f.key === "model" || f.key === "rx_power",
      oid: f.key === "status" ? "1.3.6.1.4.1.3902.1082.500.1.2.4.2.1.2" : "",
      value_divisor: f.key === "status" ? 1000 : f.key === "pon_tx_power" ? 100 : 0,
      online_values: f.key === "pon_status" ? [1] : f.hasStatusValues ? [3] : undefined,
      offline_values: f.key === "pon_status" ? [2] : f.hasStatusValues ? [] : undefined,
      status_mode: f.key === "pon_status" ? "if_mib_index" : f.key === "status" ? "rx_power_threshold" : f.hasStatusValues ? "pon_onu_suffix" : undefined,
      offline_rx_dbm: f.key === "status" ? -70 : undefined,
      ifdescr_oid: f.hasStatusValues ? "1.3.6.1.2.1.2.2.1.2" : undefined,
      ifname_oid: f.hasStatusValues ? "1.3.6.1.2.1.31.1.1.1.1" : undefined,
      ifoper_oid: f.hasStatusValues ? "1.3.6.1.2.1.2.2.1.8" : undefined,
      online_count_oid: "",
      offline_count_oid: "",
    };
  }
  return out;
}

function metricsFromApi(raw: unknown): OltMetricsForm {
  const base = defaultMetricsForm();
  if (!raw || typeof raw !== "object") return base;
  const src = raw as Record<string, OltOnuMetricDef>;
  for (const f of OLT_ALL_METRIC_FIELDS) {
    const m = src[f.key];
    if (!m) continue;
    base[f.key] = {
      enabled: m.enabled !== false,
      oid: m.oid ?? "",
      value_divisor: Number.isFinite(Number(m.value_divisor)) ? Number(m.value_divisor) : 0,
      online_values: m.online_values ?? (f.hasStatusValues ? [3] : undefined),
      offline_values: m.offline_values ?? (f.hasStatusValues ? [] : undefined),
      status_mode:
        (m.status_mode as "pon_onu_suffix" | "if_mib_index" | "pon_online_offline" | "rx_power_threshold" | undefined) ??
        (f.hasStatusValues ? "pon_onu_suffix" : undefined),
      offline_rx_dbm: Number.isFinite(Number(m.offline_rx_dbm)) ? Number(m.offline_rx_dbm) : f.key === "status" ? -70 : undefined,
      ifdescr_oid: m.ifdescr_oid ?? (f.hasStatusValues ? "1.3.6.1.2.1.2.2.1.2" : undefined),
      ifname_oid: m.ifname_oid ?? (f.hasStatusValues ? "1.3.6.1.2.1.31.1.1.1.1" : undefined),
      ifoper_oid: m.ifoper_oid ?? (f.hasStatusValues ? "1.3.6.1.2.1.2.2.1.8" : undefined),
      online_count_oid: m.online_count_oid ?? "",
      offline_count_oid: (() => {
        const off = (m.offline_count_oid ?? "").trim();
        const mode =
          (m.status_mode as "pon_onu_suffix" | "if_mib_index" | "pon_online_offline" | "rx_power_threshold" | undefined) ??
          (f.hasStatusValues ? "pon_onu_suffix" : undefined);
        if (mode === "pon_online_offline" && off.endsWith(".2.1.4")) {
          return "1.3.6.1.4.1.3709.3.6.18.2.1.6";
        }
        return off;
      })(),
    };
  }
  return base;
}

function buildMetricsPayload(form: OltMetricsForm): OltMetricsForm {
  const out: OltMetricsForm = {};
  for (const f of OLT_ALL_METRIC_FIELDS) {
    const m = form[f.key] ?? {};
    const statusMode = m.status_mode ?? (f.key === "pon_status" ? "if_mib_index" : "pon_onu_suffix");
    if (f.hasStatusValues && statusMode === "pon_online_offline") {
      const onlineOID = (m.online_count_oid ?? "").trim();
      const offlineOID = (m.offline_count_oid ?? "").trim();
      if (m.enabled === false || !onlineOID || !offlineOID) continue;
      out[f.key] = {
        enabled: true,
        oid: onlineOID,
        status_mode: "pon_online_offline",
        online_count_oid: onlineOID,
        offline_count_oid: offlineOID,
      };
      continue;
    }
    const oid =
      f.hasStatusValues && statusMode === "if_mib_index"
        ? ((m.oid ?? "").trim() || (m.ifoper_oid ?? "").trim() || "1.3.6.1.2.1.2.2.1.8")
        : (m.oid ?? "").trim();
    if (!oid) continue;
    out[f.key] = {
      enabled: m.enabled !== false,
      oid,
      value_divisor: Number.isFinite(Number(m.value_divisor)) ? Number(m.value_divisor) : 0,
      ...(f.hasStatusValues
        ? {
            online_values: Array.isArray(m.online_values)
              ? m.online_values
              : parseIntList(String(m.online_values ?? (statusMode === "if_mib_index" ? "1" : "3"))),
            offline_values: Array.isArray(m.offline_values) ? m.offline_values : parseIntList(String(m.offline_values ?? "")),
            status_mode: statusMode,
            ifdescr_oid: (m.ifdescr_oid ?? "").trim(),
            ifname_oid: (m.ifname_oid ?? "").trim(),
            ifoper_oid: (m.ifoper_oid ?? "").trim(),
            online_count_oid: (m.online_count_oid ?? "").trim(),
            offline_count_oid: (m.offline_count_oid ?? "").trim(),
            offline_rx_dbm:
              statusMode === "rx_power_threshold" && Number.isFinite(Number(m.offline_rx_dbm))
                ? Number(m.offline_rx_dbm)
                : undefined,
          }
        : {}),
    };
  }
  return out;
}

function parseIntList(raw: string): number[] {
  return raw
    .split(/[,;\s]+/)
    .map((s) => s.trim())
    .filter((s) => s !== "")
    .map((s) => Number(s.trim()))
    .filter((n) => Number.isFinite(n));
}

function hasEnabledMetrics(form: OltMetricsForm): boolean {
  return OLT_ALL_METRIC_FIELDS.some((f) => {
    const m = form[f.key];
    if (m?.enabled === false) return false;
    const mode = m?.status_mode ?? "pon_onu_suffix";
    if (f.hasStatusValues && mode === "pon_online_offline") {
      return (m?.online_count_oid ?? "").trim() !== "" && (m?.offline_count_oid ?? "").trim() !== "";
    }
    if (f.hasStatusValues && mode === "if_mib_index") {
      return (m?.oid ?? "").trim() !== "" || (m?.ifoper_oid ?? "").trim() !== "";
    }
    if (f.hasStatusValues && mode === "rx_power_threshold") {
      return (m?.oid ?? "").trim() !== "" || (form.rx_power?.oid ?? "").trim() !== "";
    }
    return (m?.oid ?? "").trim() !== "";
  });
}

function filterOltModels<T extends { model: string }>(list: T[]): T[] {
  return list.filter((m) => m.model.trim().toLowerCase() !== "padrão" && m.model.trim().toLowerCase() !== "padrao");
}

const OLT_COLLECT_METHODS: { value: string; label: string }[] = [
  { value: "if_mib_refresh", label: "SNMP — actualizar interfaces (walk completo)" },
  { value: "if_mib_snapshot", label: "SNMP — ler interfaces (rápido)" },
  { value: "onu_metrics_collect", label: "Coletar métricas SNMP das ONUs" },
  { value: "onu_snmp_walk", label: "Contagem simples via snmpwalk (legado)" },
  { value: "vsol_onu_collect", label: "VSOL — tabela legada gOnuAuthList" },
  { value: "snmp_walk", label: "SNMP — snmpwalk (OID livre)" },
  { value: "snmp_get", label: "SNMP — snmpget (vários OIDs)" },
  { value: "telnet", label: "Telnet — comando CLI" },
  { value: "datacom_build_pons", label: "Datacom — agregar PONs do walk ONU" },
  { value: "if_mib_merge_pons", label: "Derivar e fundir portas PON" },
  { value: "stabilize_pons", label: "Estabilizar PONs vs. coleta anterior" },
];

function defaultOnuReportForBrand(brandName: string): { pre_commands: string[]; commands: string[] } {
  const b = brandName.trim().toUpperCase();
  if (b.includes("ZTE")) {
    return {
      pre_commands: ["terminal length 0", "terminal page-break disable", "scroll 512"],
      commands: [
        "show gpon onu detail-info {gpon_onu}",
        "show pon onu information {gpon_onu}",
        "show pon power onu-rx {gpon_onu}",
        "show pon power onu-tx {gpon_onu}",
      ],
    };
  }
  if (b.includes("VSOL")) {
    return {
      pre_commands: ["enable", "conf terminal"],
      commands: ["show onu info {pon} {onu}", "show onu state {pon} {onu}"],
    };
  }
  return { pre_commands: [], commands: [] };
}

function defaultUnauthorizedForBrand(brandName: string): { pre_commands: string[]; command: string } {
  const b = brandName.trim().toUpperCase();
  if (b.includes("VSOL")) {
    return {
      pre_commands: ["enable", "{enable}", "configure terminal", "interface gpon 0/{pon}"],
      command: "show onu auto-find",
    };
  }
  if (b.includes("ZTE")) {
    return {
      pre_commands: ["terminal length 0", "terminal page-break disable"],
      command: "show gpon onu uncfg",
    };
  }
  return { pre_commands: [], command: "" };
}

/** Templates de autorizar/desautorizar — um comando por linha; placeholders {vlan} {onu_type} {name}. */
function defaultAuthCmdsForBrand(brandName: string): { authorize: string; deauthorize: string } {
  const b = brandName.trim().toUpperCase();
  if (b.includes("VSOL")) {
    return {
      authorize: "",
      deauthorize: "interface gpon 0/{pon}; no onu {onu}",
    };
  }
  if (b.includes("ZTE")) {
    return {
      authorize: [
        "configure terminal",
        "interface gpon_olt-1/1/{pon}",
        "onu {onu} type GU201-G sn {serial}",
        "exit",
        "interface gpon_onu-1/1/{pon}:{onu}",
        "name {name}",
        "sn-bind enable sn",
        "tcont 1 profile 1G",
        "gemport 1 name 1G tcont 1",
        "exit",
        "interface vport-1/1/{pon}.{onu}:1",
        "service-port 1 user-vlan {vlan} vlan {vlan}",
        "exit",
        "pon-onu-mng gpon_onu-1/1/{pon}:{onu}",
        "vlan port eth_0/1 mode tag vlan {vlan}",
        "vlan port eth_0/2 mode tag vlan {vlan}",
        "vlan port eth_0/3 mode tag vlan {vlan}",
        "vlan port eth_0/4 mode tag vlan {vlan}",
        "service 1 gemport 1 vlan {vlan}",
        "exit",
      ].join("\n"),
      deauthorize: "configure terminal\ninterface gpon_olt-1/1/{pon}\nno onu {onu}\nexit\nexit",
    };
  }
  return { authorize: "", deauthorize: "" };
}

function linesToText(lines?: string[]): string {
  return (lines ?? []).join("\n");
}

function textToLines(text: string): string[] {
  return text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}

function defaultPonTelnetForBrand(brandName: string): { pre_commands: string[]; commands: string[] } {
  const b = brandName.trim().toUpperCase();
  if (b.includes("ZTE")) {
    return {
      pre_commands: ["terminal length 0", "terminal page-break disable", "scroll 512"],
      commands: [
        "show pon power olt-tx gpon-olt_1/1/{pon}",
        "show pon power olt-rx gpon-olt_1/1/{pon}",
        "show optical-module-info gpon-olt_1/1/{pon}",
      ],
    };
  }
  if (b.includes("VSOL")) {
    return {
      pre_commands: ["enable", "{enable}", "conf terminal"],
      commands: ["show pon optical-transceiver-diagnosis slot 0 pon {pon}"],
    };
  }
  return { pre_commands: [], commands: [] };
}

function ponTelnetCommandsFromApi(pc?: OltPonTelnetCommands | null): { pre_commands: string[]; commands: string[] } {
  return {
    pre_commands: Array.isArray(pc?.pre_commands) ? pc.pre_commands : [],
    commands: Array.isArray(pc?.commands) ? pc.commands : [],
  };
}

function onuReportCommandsFromApi(rc?: OltOnuReportCommands | null): { pre_commands: string[]; commands: string[] } {
  const cmds =
    Array.isArray(rc?.commands) && rc.commands.length > 0
      ? rc.commands
      : rc?.command?.trim()
        ? [rc.command.trim()]
        : [];
  return {
    pre_commands: Array.isArray(rc?.pre_commands) ? rc.pre_commands : [],
    commands: cmds,
  };
}

function OltVendorsPanel() {
  const qc = useQueryClient();
  const brands = useQuery({ queryKey: ["olt-brands"], queryFn: () => apiFetch<{ brands: string[] }>("/api/v1/settings/olt-vendors") });
  const connDefaults = useQuery({
    queryKey: ["settings-conn-def"],
    queryFn: () =>
      apiFetch<{
        telnet_user: string | null;
        telnet_password_configured: boolean;
      }>("/api/v1/settings/connection/defaults"),
    staleTime: 60_000,
  });
  const oltCatalog = useQuery({
    queryKey: ["olt-models-catalog"],
    queryFn: () => apiFetch<{ catalog: Record<string, string[]> }>("/api/v1/settings/olt-vendors/catalog"),
    staleTime: 120_000,
  });
  const oltDevices = useQuery({
    queryKey: ["olt-devices-settings"],
    queryFn: () =>
      apiFetch<{ olts: Array<{ id: string; description?: string | null; ip?: string | null; brand?: string | null }> }>(
        "/api/v1/olt/devices",
      ),
    staleTime: 60_000,
  });

  const [view, setView] = useState<"list" | "edit">("list");
  const [brand, setBrand] = useState("");
  const [model, setModel] = useState("");
  const [listSearch, setListSearch] = useState("");
  const [listBrandFilter, setListBrandFilter] = useState("");
  const [editSection, setEditSection] = useState<OltEditSection>("geral");
  const [expandedMetricKey, setExpandedMetricKey] = useState<string | null>(null);
  const { push: pushToast } = useAppToast();
  const [metrics, setMetrics] = useState<OltMetricsForm>(() => defaultMetricsForm());
  const [steps, setSteps] = useState<OltCollectionStep[]>([]);
  const [onuReportPreText, setOnuReportPreText] = useState("");
  const [onuReportCommandsText, setOnuReportCommandsText] = useState("");
  const [onuReportSerialSearchText, setOnuReportSerialSearchText] = useState("");
  const [onuAuthorizeCmd, setOnuAuthorizeCmd] = useState("");
  const [onuDeauthorizeCmd, setOnuDeauthorizeCmd] = useState("");
  const [onuUnauthorizedQueryCmd, setOnuUnauthorizedQueryCmd] = useState("");
  const [onuUnauthorizedPreText, setOnuUnauthorizedPreText] = useState("");
  const [authorizeVlanSnmpOid, setAuthorizeVlanSnmpOid] = useState(DEFAULT_ZTE_VLAN_SNMP_OID);
  const [authorizeVlanCatalog, setAuthorizeVlanCatalog] = useState<OltAuthorizeVlanEntry[]>([]);
  const [vlanDiscoverOltId, setVlanDiscoverOltId] = useState("");
  const [vlanDiscoverLoading, setVlanDiscoverLoading] = useState(false);
  const [ponTelnetPreText, setPonTelnetPreText] = useState("");
  const [ponTelnetCommandsText, setPonTelnetCommandsText] = useState("");
  const [ponTelnetEnabled, setPonTelnetEnabled] = useState(false);
  const [ponTelnetMaxPerCycle, setPonTelnetMaxPerCycle] = useState("16");
  const [onuReportEnabled, setOnuReportEnabled] = useState(false);
  const [onuReportOnlineOnly, setOnuReportOnlineOnly] = useState(true);
  const [onuReportMaxPerCycle, setOnuReportMaxPerCycle] = useState("25");
  const [copyModalOpen, setCopyModalOpen] = useState(false);
  const [copyBrand, setCopyBrand] = useState("");
  const [copyModel, setCopyModel] = useState("");
  const [copyLoading, setCopyLoading] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createBrandNew, setCreateBrandNew] = useState(false);
  const [createBrand, setCreateBrand] = useState("");
  const [createBrandName, setCreateBrandName] = useState("");
  const [createModelName, setCreateModelName] = useState("");

  const catalogProfiles = useMemo(() => {
    const cat = oltCatalog.data?.catalog ?? {};
    const rows: Array<{ brand: string; model: string }> = [];
    for (const [b, models] of Object.entries(cat)) {
      for (const m of filterOltModels((models ?? []).map((model) => ({ model })))) {
        rows.push({ brand: b, model: m.model });
      }
    }
    rows.sort((a, b) => a.brand.localeCompare(b.brand, "pt") || a.model.localeCompare(b.model, "pt"));
    return rows;
  }, [oltCatalog.data]);

  const brandOptions = useMemo(() => {
    const fromApi = brands.data?.brands ?? [];
    const fromCat = Object.keys(oltCatalog.data?.catalog ?? {});
    return Array.from(new Set([...fromApi, ...fromCat])).sort((a, b) => a.localeCompare(b, "pt"));
  }, [brands.data, oltCatalog.data]);

  const filteredProfiles = useMemo(() => {
    const q = listSearch.trim().toLowerCase();
    return catalogProfiles.filter((p) => {
      if (!brandMatchesFilter(p.brand, listBrandFilter)) return false;
      if (!q) return true;
      return `${p.brand} ${p.model}`.toLowerCase().includes(q);
    });
  }, [catalogProfiles, listSearch, listBrandFilter]);

  const brandLogoFilters = OLT_BRAND_LOGOS;

  const copyModelList = useMemo(() => {
    const cat = oltCatalog.data?.catalog ?? {};
    let list = cat[copyBrand] ?? [];
    if (list.length === 0 && copyBrand) {
      const key = Object.keys(cat).find((k) => k.toLowerCase() === copyBrand.toLowerCase());
      if (key) list = cat[key] ?? [];
    }
    return filterOltModels(list.map((m) => ({ model: m }))).map((x) => x.model);
  }, [oltCatalog.data, copyBrand]);

  const vendor = useQuery({
    queryKey: ["olt-vendor-model", brand, model],
    enabled: view === "edit" && !!brand && !!model,
    queryFn: () =>
      apiFetch<{
        brand: string;
        model: string;
        onu_metrics?: OltMetricsForm;
        collection_steps?: OltCollectionStep[];
        onu_report_commands?: OltOnuReportCommands;
        pon_telnet_commands?: OltPonTelnetCommands;
      }>(`/api/v1/settings/olt-vendors/${encodeURIComponent(brand)}/models/${encodeURIComponent(model)}`),
  });

  useEffect(() => {
    if (!vendor.data || view !== "edit") return;
    setMetrics(metricsFromApi(vendor.data.onu_metrics));
    setSteps(Array.isArray(vendor.data.collection_steps) ? vendor.data.collection_steps : []);
    const rc = vendor.data.onu_report_commands;
    const parsed = onuReportCommandsFromApi(rc);
    setOnuReportPreText(parsed.pre_commands.join("\n"));
    setOnuReportCommandsText(parsed.commands.join("\n"));
    setOnuReportSerialSearchText(rc?.serial_search_command?.trim() ?? "");
    setOnuAuthorizeCmd(rc?.onu_authorize_command?.trim() ?? "");
    setOnuDeauthorizeCmd(rc?.onu_deauthorize_command?.trim() ?? "");
    setOnuUnauthorizedQueryCmd(rc?.unauthorized_onu_query_command?.trim() ?? "");
    setOnuUnauthorizedPreText(linesToText(rc?.unauthorized_onu_pre_commands));
    setAuthorizeVlanSnmpOid(rc?.authorize_vlan_snmp_oid?.trim() || DEFAULT_ZTE_VLAN_SNMP_OID);
    setAuthorizeVlanCatalog(
      Array.isArray(rc?.authorize_vlan_catalog)
        ? [...rc.authorize_vlan_catalog].sort((a, b) => (a.vid ?? 0) - (b.vid ?? 0))
        : [],
    );
    setOnuReportEnabled(rc?.enabled === true);
    setOnuReportOnlineOnly(rc?.monitor_online_only !== false);
    setOnuReportMaxPerCycle(rc?.max_onus_per_cycle != null && rc.max_onus_per_cycle > 0 ? String(rc.max_onus_per_cycle) : "25");
    const pc = vendor.data.pon_telnet_commands;
    const ponParsed = ponTelnetCommandsFromApi(pc);
    setPonTelnetPreText(ponParsed.pre_commands.join("\n"));
    setPonTelnetCommandsText(ponParsed.commands.join("\n"));
    setPonTelnetEnabled(pc?.enabled === true);
    setPonTelnetMaxPerCycle(pc?.max_pons_per_cycle != null && pc.max_pons_per_cycle > 0 ? String(pc.max_pons_per_cycle) : "16");
  }, [vendor.data, view]);

  const metricsReady = hasEnabledMetrics(metrics);
  const enabledMetricsCount = useMemo(() => {
    let n = 0;
    for (const f of OLT_ALL_METRIC_FIELDS) {
      const m = metrics[f.key];
      if (!m || m.enabled === false) continue;
      const mode = m.status_mode ?? "pon_onu_suffix";
      if (f.hasStatusValues && mode === "pon_online_offline") {
        if ((m.online_count_oid ?? "").trim() && (m.offline_count_oid ?? "").trim()) n += 1;
        continue;
      }
      if (f.hasStatusValues && mode === "if_mib_index") {
        if ((m.oid ?? "").trim() || (m.ifoper_oid ?? "").trim()) n += 1;
        continue;
      }
      if (f.hasStatusValues && mode === "rx_power_threshold") {
        if ((m.oid ?? "").trim() || (metrics.rx_power?.oid ?? "").trim()) n += 1;
        continue;
      }
      if ((m.oid ?? "").trim()) n += 1;
    }
    return n;
  }, [metrics]);

  const telnetCmdCount = useMemo(() => {
    return (
      textToLines(onuReportCommandsText).length +
      textToLines(onuUnauthorizedPreText).length +
      (onuUnauthorizedQueryCmd.trim() ? 1 : 0) +
      textToLines(onuAuthorizeCmd).length +
      textToLines(onuDeauthorizeCmd).length +
      textToLines(ponTelnetCommandsText).length +
      (onuReportSerialSearchText.trim() ? 1 : 0)
    );
  }, [
    onuReportCommandsText,
    onuUnauthorizedPreText,
    onuUnauthorizedQueryCmd,
    onuAuthorizeCmd,
    onuDeauthorizeCmd,
    ponTelnetCommandsText,
    onuReportSerialSearchText,
  ]);

  const vlanActive = authorizeVlanCatalog.filter((e) => !e.ignored).length;
  const vlanIgnored = authorizeVlanCatalog.filter((e) => e.ignored).length;

  const patch = useMutation({
    mutationFn: () => {
      const payload = buildMetricsPayload(metrics);
      const autoSteps: OltCollectionStep[] = metricsReady
        ? [{ id: "onu_metrics", method: "onu_metrics_collect", enabled: true }]
        : steps;
      const preCommands = textToLines(onuReportPreText);
      const commands = textToLines(onuReportCommandsText);
      const unauthorizedPre = textToLines(onuUnauthorizedPreText);
      const serialSearch = onuReportSerialSearchText.trim();
      const ponPreCommands = textToLines(ponTelnetPreText);
      const ponCommands = textToLines(ponTelnetCommandsText);
      const maxPons = Number.parseInt(ponTelnetMaxPerCycle, 10);
      const maxOnus = Number.parseInt(onuReportMaxPerCycle, 10);
      return apiFetch(`/api/v1/settings/olt-vendors/${encodeURIComponent(brand)}/models/${encodeURIComponent(model)}`, {
        method: "PATCH",
        json: {
          onu_metrics: payload,
          collection_steps: autoSteps,
          onu_report_commands: {
            enabled: onuReportEnabled,
            monitor_online_only: onuReportOnlineOnly,
            max_onus_per_cycle: Number.isFinite(maxOnus) && maxOnus > 0 ? maxOnus : 25,
            pre_commands: preCommands,
            commands,
            serial_search_command: serialSearch || undefined,
            onu_authorize_command: onuAuthorizeCmd.trim() || undefined,
            onu_deauthorize_command: onuDeauthorizeCmd.trim() || undefined,
            unauthorized_onu_query_command: onuUnauthorizedQueryCmd.trim() || undefined,
            unauthorized_onu_pre_commands: unauthorizedPre,
            authorize_vlan_snmp_oid: authorizeVlanSnmpOid.trim() || undefined,
            authorize_vlan_catalog: authorizeVlanCatalog.length > 0 ? authorizeVlanCatalog : undefined,
          },
          pon_telnet_commands: {
            enabled: ponTelnetEnabled,
            max_pons_per_cycle: Number.isFinite(maxPons) && maxPons > 0 ? maxPons : 16,
            pre_commands: ponPreCommands,
            commands: ponCommands,
          },
        },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["olt-vendor-model", brand, model] });
      qc.invalidateQueries({ queryKey: ["olt-models-catalog"] });
      toastOk(pushToast, `Perfil guardado: ${brand} / ${model}`);
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao salvar."),
  });

  const createModel = useMutation({
    mutationFn: ({ targetBrand, name }: { targetBrand: string; name: string }) =>
      apiFetch(`/api/v1/settings/olt-vendors/${encodeURIComponent(targetBrand)}/models`, {
        method: "POST",
        json: { model: name },
      }),
    onSuccess: (_data, { targetBrand, name }) => {
      qc.invalidateQueries({ queryKey: ["olt-brands"] });
      qc.invalidateQueries({ queryKey: ["olt-models-catalog"] });
      setBrand(targetBrand);
      setModel(name);
      setView("edit");
      setCreateModalOpen(false);
      setCreateBrandNew(false);
      setCreateBrand("");
      setCreateBrandName("");
      setCreateModelName("");
      toastOk(pushToast, `Modelo «${name}» criado (${targetBrand}).`);
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao criar modelo."),
  });

  const removeModel = useMutation({
    mutationFn: ({ targetBrand, targetModel }: { targetBrand: string; targetModel: string }) =>
      apiFetch(`/api/v1/settings/olt-vendors/${encodeURIComponent(targetBrand)}/models/${encodeURIComponent(targetModel)}`, {
        method: "DELETE",
      }),
    onSuccess: (_data, { targetBrand, targetModel }) => {
      qc.invalidateQueries({ queryKey: ["olt-brands"] });
      qc.invalidateQueries({ queryKey: ["olt-models-catalog"] });
      if (brand === targetBrand && model === targetModel) {
        setBrand("");
        setModel("");
        setView("list");
      }
      toastOk(pushToast, "Modelo removido.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao remover."),
  });

  if (brands.isLoading && oltCatalog.isLoading) return <p>A carregar…</p>;
  if (brands.isError) return <div className="msg msg--err">{(brands.error as Error).message}</div>;

  function openCreateModal() {
    setCreateBrandNew(false);
    setCreateBrand(brand || brandOptions[0] || "");
    setCreateBrandName("");
    setCreateModelName("");
    setCreateModalOpen(true);
  }

  function submitCreateModel() {
    const name = createModelName.trim();
    const targetBrand = (createBrandNew ? createBrandName : createBrand).trim();
    if (!targetBrand || !name) return;
    createModel.mutate({ targetBrand, name });
  }

  function enterEdit(targetBrand: string, targetModel: string) {
    setBrand(targetBrand);
    setModel(targetModel);
    setEditSection("geral");
    setExpandedMetricKey(null);
    setView("edit");
  }

  function backToList() {
    setView("list");
    setEditSection("geral");
    setExpandedMetricKey(null);
  }

  function toggleMetricExpand(key: string) {
    setExpandedMetricKey((prev) => (prev === key ? null : key));
  }

  function openCopyModal(forBrand?: string, forModel?: string) {
    if (forBrand && forModel) {
      setBrand(forBrand);
      setModel(forModel);
      setView("edit");
    }
    setCopyBrand(brandOptions[0] || "");
    setCopyModel("");
    setCopyModalOpen(true);
  }

  function applyReportFromSrc(src: {
    onu_metrics?: OltMetricsForm;
    collection_steps?: OltCollectionStep[];
    onu_report_commands?: OltOnuReportCommands;
    pon_telnet_commands?: OltPonTelnetCommands;
  }) {
    setMetrics(metricsFromApi(src.onu_metrics));
    setSteps(Array.isArray(src.collection_steps) ? src.collection_steps : []);
    const parsed = onuReportCommandsFromApi(src.onu_report_commands);
    setOnuReportPreText(parsed.pre_commands.join("\n"));
    setOnuReportCommandsText(parsed.commands.join("\n"));
    setOnuReportSerialSearchText(src.onu_report_commands?.serial_search_command?.trim() ?? "");
    setOnuAuthorizeCmd(src.onu_report_commands?.onu_authorize_command?.trim() ?? "");
    setOnuDeauthorizeCmd(src.onu_report_commands?.onu_deauthorize_command?.trim() ?? "");
    setOnuUnauthorizedQueryCmd(src.onu_report_commands?.unauthorized_onu_query_command?.trim() ?? "");
    setOnuUnauthorizedPreText(linesToText(src.onu_report_commands?.unauthorized_onu_pre_commands));
    setAuthorizeVlanSnmpOid(src.onu_report_commands?.authorize_vlan_snmp_oid?.trim() || DEFAULT_ZTE_VLAN_SNMP_OID);
    setAuthorizeVlanCatalog(
      Array.isArray(src.onu_report_commands?.authorize_vlan_catalog)
        ? [...src.onu_report_commands.authorize_vlan_catalog].sort((a, b) => (a.vid ?? 0) - (b.vid ?? 0))
        : [],
    );
    setOnuReportEnabled(src.onu_report_commands?.enabled === true);
    setOnuReportOnlineOnly(src.onu_report_commands?.monitor_online_only !== false);
    setOnuReportMaxPerCycle(
      src.onu_report_commands?.max_onus_per_cycle != null && src.onu_report_commands.max_onus_per_cycle > 0
        ? String(src.onu_report_commands.max_onus_per_cycle)
        : "25",
    );
    const ponParsed = ponTelnetCommandsFromApi(src.pon_telnet_commands);
    setPonTelnetPreText(ponParsed.pre_commands.join("\n"));
    setPonTelnetCommandsText(ponParsed.commands.join("\n"));
    setPonTelnetEnabled(src.pon_telnet_commands?.enabled === true);
    setPonTelnetMaxPerCycle(
      src.pon_telnet_commands?.max_pons_per_cycle != null && src.pon_telnet_commands.max_pons_per_cycle > 0
        ? String(src.pon_telnet_commands.max_pons_per_cycle)
        : "16",
    );
  }

  async function applyCopyFromProfile() {
    const srcBrand = copyBrand.trim();
    const srcModel = copyModel.trim();
    if (!srcBrand || !srcModel) return;
    if (srcBrand === brand && srcModel === model) {
      toastErr(pushToast, new Error("Escolha um perfil de origem diferente do perfil actual."));
      return;
    }
    setCopyLoading(true);
    try {
      const src = await apiFetch<{
        onu_metrics?: OltMetricsForm;
        collection_steps?: OltCollectionStep[];
        onu_report_commands?: OltOnuReportCommands;
        pon_telnet_commands?: OltPonTelnetCommands;
      }>(`/api/v1/settings/olt-vendors/${encodeURIComponent(srcBrand)}/models/${encodeURIComponent(srcModel)}`);
      applyReportFromSrc(src);
      setCopyModalOpen(false);
      toastOk(pushToast, `Perfil copiado de ${srcBrand} / ${srcModel} (SNMP e telnet). Clique em Guardar para gravar.`);
    } catch (e) {
      toastErr(pushToast, e, "Falha ao copiar perfil.");
    } finally {
      setCopyLoading(false);
    }
  }

  const modals = (
    <>
      {createModalOpen && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !createModel.isPending && setCreateModalOpen(false)}>
          <div className="modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()} style={{ maxWidth: 440 }}>
            <h3 style={{ marginTop: 0 }}>Novo perfil OLT</h3>
            <div className="field">
              <label>Marca</label>
              {!createBrandNew ? (
                <select className="input" value={createBrand} onChange={(e) => setCreateBrand(e.target.value)}>
                  <option value="">— escolher —</option>
                  {brandOptions.map((b) => (
                    <option key={b} value={b}>
                      {b}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  className="input"
                  placeholder="Ex.: Datacom"
                  value={createBrandName}
                  onChange={(e) => setCreateBrandName(e.target.value)}
                />
              )}
              <button type="button" className="btn" style={{ marginTop: 6, fontSize: 12 }} onClick={() => setCreateBrandNew((v) => !v)}>
                {createBrandNew ? "Usar marca existente" : "Criar nova marca"}
              </button>
            </div>
            <div className="field">
              <label>Nome do modelo</label>
              <input
                className="input"
                placeholder="Ex.: DM4610"
                value={createModelName}
                onChange={(e) => setCreateModelName(e.target.value)}
              />
            </div>
            <div className="row" style={{ gap: 8, justifyContent: "flex-end" }}>
              <button type="button" className="btn" disabled={createModel.isPending} onClick={() => setCreateModalOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={
                  createModel.isPending ||
                  !createModelName.trim() ||
                  !(createBrandNew ? createBrandName.trim() : createBrand.trim())
                }
                onClick={submitCreateModel}
              >
                {createModel.isPending ? "A criar…" : "Criar"}
              </button>
            </div>
          </div>
        </div>
      )}
      {copyModalOpen && (
        <div
          className="modal-backdrop"
          role="presentation"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !copyLoading) setCopyModalOpen(false);
          }}
        >
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="copy-olt-profile-title"
            onMouseDown={(e) => e.stopPropagation()}
            style={{ maxWidth: 440 }}
          >
            <h3 id="copy-olt-profile-title" style={{ marginTop: 0 }}>
              Copiar informações do perfil
            </h3>
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
              Copia OIDs SNMP, comandos telnet e mapa VLAN do perfil de origem para{" "}
              <strong>
                {brand || "—"} / {model || "—"}
              </strong>
              . Clique em Guardar para persistir.
            </p>
            <div className="field">
              <label>Marca de origem</label>
              <select
                className="select"
                style={{ width: "100%" }}
                value={copyBrand}
                onChange={(e) => {
                  setCopyBrand(e.target.value);
                  setCopyModel("");
                }}
              >
                <option value="">— escolher —</option>
                {brandOptions.map((b) => (
                  <option key={b} value={b}>
                    {b}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label>Modelo de origem</label>
              <select
                className="select"
                style={{ width: "100%" }}
                value={copyModel}
                disabled={!copyBrand}
                onChange={(e) => setCopyModel(e.target.value)}
              >
                <option value="">— escolher —</option>
                {copyModelList.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            </div>
            <div className="row" style={{ gap: 8, justifyContent: "flex-end", marginTop: 12 }}>
              <button type="button" className="btn" disabled={copyLoading} onClick={() => setCopyModalOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={copyLoading || !copyBrand.trim() || !copyModel.trim() || !brand || !model}
                onClick={() => void applyCopyFromProfile()}
              >
                {copyLoading ? "A copiar…" : "Copiar para este perfil"}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );

  return (
    <>
      <div className="olt-profiles-layout">
        <div className="card">
          <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8 }}>
            <div>
              <h2 style={{ margin: 0 }}>Perfis de OLT</h2>
              <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 4, marginBottom: 0 }}>
                Catálogo de marca + modelo com OIDs SNMP, comandos telnet e mapa VLAN. Credenciais em <strong>Rede e SNMP</strong>.
              </p>
            </div>
            <button type="button" className="btn btn--primary" onClick={openCreateModal}>
              <Plus size={16} aria-hidden /> Novo Perfil
            </button>
          </div>

          <div className="olt-profiles-toolbar">
            <div className="field olt-profiles-toolbar__search">
              <label>Pesquisar</label>
              <input
                className="input"
                placeholder="Marca ou modelo…"
                value={listSearch}
                onChange={(e) => setListSearch(e.target.value)}
              />
            </div>
            <div className="olt-brand-filters" role="group" aria-label="Filtro por marca">
              {brandLogoFilters.map((logo) => {
                const active = listBrandFilter.toLowerCase() === logo.label.toLowerCase();
                return (
                  <button
                    key={logo.key}
                    type="button"
                    className={
                      "olt-brand-filter" +
                      (logo.key === "huawei" ? " olt-brand-filter--huawei" : "") +
                      (active ? " olt-brand-filter--active" : "")
                    }
                    title={logo.label}
                    aria-pressed={active}
                    aria-label={"Filtrar por " + logo.label}
                    onClick={() => setListBrandFilter(active ? "" : logo.label)}
                  >
                    <img src={"/brand-logos/" + logo.file} alt={logo.label} />
                  </button>
                );
              })}
            </div>
          </div>

          {oltCatalog.isLoading && <p style={{ marginTop: 12 }}>A carregar catálogo…</p>}
          {oltCatalog.isError && <div className="msg msg--err">{(oltCatalog.error as Error).message}</div>}

          <div className="table-wrap" style={{ marginTop: 12 }}>
            <table className="olt-profiles-table">
              <thead>
                <tr>
                  <th>Descrição</th>
                  <th>Marca</th>
                  <th>Modelo</th>
                  <th>Status</th>
                  <th style={{ width: 110 }}>Ações</th>
                </tr>
              </thead>
              <tbody>
                {filteredProfiles.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ color: "var(--muted)" }}>
                      Nenhum perfil encontrado.
                    </td>
                  </tr>
                ) : (
                  filteredProfiles.map((p) => (
                    <tr key={p.brand + "::" + p.model}>
                      <td>{profileDescription(p.brand, p.model)}</td>
                      <td>{p.brand}</td>
                      <td className="mono">{p.model}</td>
                      <td>
                        <span className="badge badge--ok">Ativo</span>
                      </td>
                      <td>
                        <div className="olt-profiles-table__actions">
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Editar"
                            aria-label={"Editar " + p.brand + " " + p.model}
                            onClick={() => enterEdit(p.brand, p.model)}
                          >
                            <Pencil size={14} aria-hidden />
                          </button>
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Copiar de outro perfil"
                            aria-label={"Copiar para " + p.brand + " " + p.model}
                            onClick={() => openCopyModal(p.brand, p.model)}
                          >
                            <Copy size={14} aria-hidden />
                          </button>
                          <button
                            type="button"
                            className="btn btn--icon btn--danger"
                            title="Excluir"
                            aria-label={"Excluir " + p.brand + " " + p.model}
                            disabled={removeModel.isPending}
                            onClick={() => {
                              if (window.confirm("Remover modelo «" + p.model + "» da marca " + p.brand + "?")) {
                                removeModel.mutate({ targetBrand: p.brand, targetModel: p.model });
                              }
                            }}
                          >
                            <Trash2 size={14} aria-hidden />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {view === "edit" && (
        <div
          className="modal-backdrop olt-profile-modal-backdrop"
          role="presentation"
          onClick={backToList}
        >
          <div
            className="olt-profile-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="olt-profile-edit-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="olt-profile-modal__header">
              <div>
                <h2 id="olt-profile-edit-title" style={{ margin: 0 }}>
                  Editar perfil de OLT
                </h2>
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "4px 0 0" }}>
                  {brand} / {model}
                </p>
              </div>
              <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
                <button type="button" className="btn" onClick={backToList}>
                  Cancelar
                </button>
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={patch.isPending || !brand || !model}
                  onClick={() => patch.mutate()}
                >
                  {patch.isPending ? "A guardar…" : "Guardar alterações"}
                </button>
              </div>
            </div>

            {vendor.isLoading && <p style={{ margin: "12px 0 0" }}>A carregar perfil…</p>}
            {vendor.isError && <div className="msg msg--err">{(vendor.error as Error).message}</div>}

            {brand && model && vendor.data && (
              <div className="olt-profile-modal__body">
                <nav className="olt-profile-modal__nav" aria-label="Secções do perfil">
                  <div className="olt-profile-modal__nav-list">
                    {OLT_EDIT_SECTIONS.map((sec) => (
                      <button
                        key={sec.id}
                        type="button"
                        className={
                          "olt-profile-modal__nav-btn" +
                          (editSection === sec.id ? " olt-profile-modal__nav-btn--active" : "")
                        }
                        onClick={() => {
                          setEditSection(sec.id);
                          setExpandedMetricKey(null);
                        }}
                      >
                        {sec.label}
                      </button>
                    ))}
                  </div>
                  <button
                    type="button"
                    className="btn btn--danger olt-profile-modal__nav-delete"
                    disabled={removeModel.isPending}
                    onClick={() => {
                      if (window.confirm("Remover modelo «" + model + "» da marca " + brand + "?")) {
                        removeModel.mutate({ targetBrand: brand, targetModel: model });
                      }
                    }}
                  >
                    Excluir perfil
                  </button>
                </nav>

                <div className="olt-profile-modal__main">
                  {!metricsReady && editSection.startsWith("snmp") && (
                    <div className="msg msg--off" style={{ marginBottom: 12, fontSize: 12 }}>
                      Nenhuma MIB SNMP configurada para monitoramento deste modelo. Marque pelo menos uma métrica abaixo e preencha o OID da tabela.
                    </div>
                  )}

              {editSection === "geral" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Informações gerais</h3>
                  <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "center" }}>
                  <span className="badge badge--ok">{brand}</span>
                  <span className="badge">{model}</span>
                </div>
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "10px 0 0" }}>
                  O perfil OLT é identificado por <strong>marca + modelo</strong>. Para outro modelo, volte à lista e crie ou edite o perfil correspondente.
                </p>
                </div>
              )}

              {editSection === "snmp-onu" && (
                <div className="olt-profile-modal__section">
                  <OltMetricsOidTable
                    title="Métricas ONU (OIDs)"
                    description="Defina os OIDs SNMP utilizados para coleta de métricas das ONUs."
                    fields={OLT_METRIC_FIELDS}
                    metrics={metrics}
                    expandedKey={expandedMetricKey}
                    onToggleExpand={toggleMetricExpand}
                    onToggleEnabled={(key, enabled) =>
                      setMetrics((prev) => ({
                        ...prev,
                        [key]: { ...prev[key], enabled },
                      }))
                    }
                    onOidChange={(key, oid) =>
                      setMetrics((prev) => ({
                        ...prev,
                        [key]: { ...prev[key], oid },
                      }))
                    }
                    renderExpanded={(field) => {
                      if (!field.hasStatusValues) return null;
                      const m = metrics[field.key] ?? {};
                      if (m.enabled === false) {
                        return <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>Active a métrica para configurar opções avançadas.</p>;
                      }
                      const statusMode = m.status_mode ?? "pon_onu_suffix";
                      return (
                        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                          <div className="field" style={{ margin: 0 }}>
                            <label style={{ fontSize: 11 }}>Modo de leitura do status</label>
                            <select
                              className="input"
                              style={{ fontSize: 12, padding: "4px 8px" }}
                              value={statusMode}
                              onChange={(e) => {
                                const mode = e.target.value as
                                  | "pon_onu_suffix"
                                  | "if_mib_index"
                                  | "pon_online_offline"
                                  | "rx_power_threshold";
                                setMetrics((prev) => {
                                  const cur = prev[field.key] ?? {};
                                  const patch: Partial<OltOnuMetricDef> = { status_mode: mode };
                                  if (mode === "pon_online_offline") {
                                    if (!(cur.online_count_oid ?? "").trim()) {
                                      patch.online_count_oid = "1.3.6.1.4.1.3709.3.6.18.2.1.5";
                                    }
                                    const off = (cur.offline_count_oid ?? "").trim();
                                    if (!off || off.endsWith(".2.1.4")) {
                                      patch.offline_count_oid = "1.3.6.1.4.1.3709.3.6.18.2.1.6";
                                    }
                                  }
                                  return { ...prev, [field.key]: { ...cur, ...patch } };
                                });
                              }}
                            >
                              <option value="pon_onu_suffix">Tabela PON/ONU (sufixo .PON.ONU)</option>
                              <option value="if_mib_index">Interfaces (ifDescr + ifOperStatus)</option>
                              <option value="pon_online_offline">Contagem por PON (OID online + OID offline)</option>
                              <option value="rx_power_threshold">RX da ONU (limiar dBm)</option>
                            </select>
                          </div>
                          {statusMode === "rx_power_threshold" && (
                            <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                              <div className="field" style={{ margin: 0, flex: "1 1 140px" }}>
                                <label style={{ fontSize: 11 }}>Limiar (dBm) — online se RX ≥ valor</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="-70"
                                  value={Number.isFinite(Number(m.offline_rx_dbm)) ? String(m.offline_rx_dbm) : "-70"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: {
                                        ...prev[field.key],
                                        offline_rx_dbm: Number(e.target.value.replace(",", ".")),
                                      },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0, flex: "1 1 140px" }}>
                                <label style={{ fontSize: 11 }}>Divisor SNMP (ZTE: 1000)</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1000"
                                  value={
                                    Number.isFinite(Number(m.value_divisor)) && Number(m.value_divisor) > 0
                                      ? String(m.value_divisor)
                                      : "1000"
                                  }
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: {
                                        ...prev[field.key],
                                        value_divisor: Number(e.target.value || 0),
                                      },
                                    }))
                                  }
                                />
                              </div>
                            </div>
                          )}
                          {statusMode === "pon_online_offline" && (
                            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ONUs online por PON</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.4.1.3709.3.6.18.2.1.5"
                                  value={m.online_count_oid ?? ""}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], online_count_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ONUs offline por PON (col. 6 Datacom)</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.4.1.3709.3.6.18.2.1.6"
                                  value={m.offline_count_oid ?? ""}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], offline_count_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                              <p style={{ margin: 0, fontSize: 11, color: "var(--muted)" }}>
                                Datacom ponIfTable: col. 3 = total, col. 5 = online (up), col. 6 = offline (down). Col. 4 = não
                                provisionadas (geralmente 0).
                              </p>
                            </div>
                          )}
                          {statusMode === "if_mib_index" && (
                            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ifName (ONU — ZTE)</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.2.1.31.1.1.1.1"
                                  value={m.ifname_oid ?? "1.3.6.1.2.1.31.1.1.1.1"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], ifname_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ifDescr</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.2.1.2.2.1.2"
                                  value={m.ifdescr_oid ?? "1.3.6.1.2.1.2.2.1.2"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], ifdescr_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ifOperStatus</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.2.1.2.2.1.8"
                                  value={m.ifoper_oid ?? "1.3.6.1.2.1.2.2.1.8"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], ifoper_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                            </div>
                          )}
                          {statusMode !== "pon_online_offline" && statusMode !== "rx_power_threshold" && (
                            <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                              <div className="field" style={{ margin: 0, flex: "1 1 120px" }}>
                                <label style={{ fontSize: 11 }}>Valores = online</label>
                                <input
                                  className="input mono"
                                  style={{ fontSize: 12 }}
                                  placeholder={statusMode === "if_mib_index" ? "1" : "3"}
                                  value={(m.online_values ?? (statusMode === "if_mib_index" ? [1] : [3])).join(", ")}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], online_values: parseIntList(e.target.value) },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0, flex: "1 1 120px" }}>
                                <label style={{ fontSize: 11 }}>Valores = offline</label>
                                <input
                                  className="input mono"
                                  style={{ fontSize: 12 }}
                                  placeholder="(vazio = resto)"
                                  value={(m.offline_values ?? []).join(", ")}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], offline_values: parseIntList(e.target.value) },
                                    }))
                                  }
                                />
                              </div>
                            </div>
                          )}
                        </div>
                      );
                    }}
                  />
                </div>
              )}

              {editSection === "snmp-pon" && (
                <div className="olt-profile-modal__section">
                  <OltMetricsOidTable
                    title="Métricas PON (OIDs)"
                    description="Defina os OIDs SNMP utilizados para coleta de métricas das portas PON."
                    fields={OLT_PON_METRIC_FIELDS}
                    metrics={metrics}
                    expandedKey={expandedMetricKey}
                    onToggleExpand={toggleMetricExpand}
                    onToggleEnabled={(key, enabled) =>
                      setMetrics((prev) => ({
                        ...prev,
                        [key]: { ...prev[key], enabled },
                      }))
                    }
                    onOidChange={(key, oid) =>
                      setMetrics((prev) => ({
                        ...prev,
                        [key]: { ...prev[key], oid },
                      }))
                    }
                    renderExpanded={(field) => {
                      if (!field.hasStatusMode && !field.hasStatusValues && !field.supportsDivisor) return null;
                      const m = metrics[field.key] ?? {};
                      if (m.enabled === false) {
                        return <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>Active a métrica para configurar opções avançadas.</p>;
                      }
                      const ponStatusMode = m.status_mode ?? (field.key === "pon_status" ? "if_mib_index" : "pon_onu_suffix");
                      return (
                        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                          {field.hasStatusMode && (
                            <div className="field" style={{ margin: 0 }}>
                              <label style={{ fontSize: 11 }}>Modo de leitura do status</label>
                              <select
                                className="input"
                                style={{ fontSize: 12, padding: "4px 8px" }}
                                value={ponStatusMode}
                                onChange={(e) =>
                                  setMetrics((prev) => ({
                                    ...prev,
                                    [field.key]: {
                                      ...prev[field.key],
                                      status_mode: e.target.value as "pon_onu_suffix" | "if_mib_index",
                                    },
                                  }))
                                }
                              >
                                <option value="if_mib_index">Interfaces (ifDescr + ifOperStatus)</option>
                                <option value="pon_onu_suffix">Tabela SNMP (sufixo .PON)</option>
                              </select>
                            </div>
                          )}
                          {field.hasStatusValues && ponStatusMode === "if_mib_index" && (
                            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ifName (PON — ex. gpon_olt-1/1/N)</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.2.1.31.1.1.1.1"
                                  value={m.ifname_oid ?? "1.3.6.1.2.1.31.1.1.1.1"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], ifname_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ifDescr (ex. PON-1/1/N)</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.2.1.2.2.1.2"
                                  value={m.ifdescr_oid ?? "1.3.6.1.2.1.2.2.1.2"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], ifdescr_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0 }}>
                                <label style={{ fontSize: 11 }}>OID ifOperStatus</label>
                                <input
                                  className="input mono"
                                  style={{ width: "100%", fontSize: 12 }}
                                  placeholder="1.3.6.1.2.1.2.2.1.8"
                                  value={m.ifoper_oid ?? "1.3.6.1.2.1.2.2.1.8"}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], ifoper_oid: e.target.value },
                                    }))
                                  }
                                />
                              </div>
                            </div>
                          )}
                          {field.supportsDivisor && (
                            <div className="field" style={{ margin: 0 }}>
                              <label style={{ fontSize: 11 }}>Divisor do valor</label>
                              <input
                                className="input mono"
                                style={{ width: "100%", fontSize: 12 }}
                                placeholder="100"
                                value={Number.isFinite(Number(m.value_divisor)) ? String(m.value_divisor) : ""}
                                onChange={(e) =>
                                  setMetrics((prev) => ({
                                    ...prev,
                                    [field.key]: { ...prev[field.key], value_divisor: Number(e.target.value || 0) },
                                  }))
                                }
                              />
                            </div>
                          )}
                          {field.hasStatusValues && (
                            <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                              <div className="field" style={{ margin: 0, flex: "1 1 120px" }}>
                                <label style={{ fontSize: 11 }}>Valores = online</label>
                                <input
                                  className="input mono"
                                  style={{ fontSize: 12 }}
                                  placeholder="1"
                                  value={(m.online_values ?? [1]).join(", ")}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], online_values: parseIntList(e.target.value) },
                                    }))
                                  }
                                />
                              </div>
                              <div className="field" style={{ margin: 0, flex: "1 1 120px" }}>
                                <label style={{ fontSize: 11 }}>Valores = offline</label>
                                <input
                                  className="input mono"
                                  style={{ fontSize: 12 }}
                                  placeholder="2"
                                  value={(m.offline_values ?? [2]).join(", ")}
                                  onChange={(e) =>
                                    setMetrics((prev) => ({
                                      ...prev,
                                      [field.key]: { ...prev[field.key], offline_values: parseIntList(e.target.value) },
                                    }))
                                  }
                                />
                              </div>
                            </div>
                          )}
                        </div>
                      );
                    }}
                  />
                </div>
              )}

              {editSection === "telnet-onu" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Telnet — relatório ONU</h3>
                  <div style={{ marginBottom: 10 }}>
                  <span
                    className={
                      connDefaults.data?.telnet_password_configured && connDefaults.data?.telnet_user
                        ? "badge badge--ok"
                        : "badge badge--off"
                    }
                  >
                    Telnet:{" "}
                    {connDefaults.data?.telnet_password_configured && connDefaults.data?.telnet_user
                      ? `configurado (${connDefaults.data.telnet_user})`
                      : "credenciais em falta — configure em Rede e SNMP"}
                  </span>
                </div>
{/* 1. Métricas de ONU */}
          <div className="card olt-telnet-block">
            <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, flexWrap: "wrap" }}>
              <div>
                <h4 style={{ margin: "0 0 4px", fontSize: 14 }}>1. Métricas de ONU (monitoramento e relatório)</h4>
                <p style={{ fontSize: 11, color: "var(--muted)", margin: 0, maxWidth: 640 }}>
                  Usado no monitoramento periódico e no relatório telnet por ONU. Pré-comandos uma vez por ciclo; depois os
                  comandos por ONU. Placeholders: <code>{"{pon}"}</code>, <code>{"{onu}"}</code>, <code>{"{gpon_onu}"}</code>.
                </p>
              </div>
              <button
                type="button"
                className="btn"
                style={{ fontSize: 12, padding: "4px 10px" }}
                onClick={() => {
                  const d = defaultOnuReportForBrand(brand);
                  setOnuReportPreText(d.pre_commands.join("\n"));
                  setOnuReportCommandsText(d.commands.join("\n"));
                  toastOk(pushToast, "Padrão de métricas ONU carregado. Clique em Guardar.");
                }}
              >
                Padrão da marca
              </button>
            </div>
            <label className="row" style={{ gap: 8, alignItems: "flex-start", margin: "12px 0 8px", cursor: "pointer" }}>
              <input type="checkbox" checked={onuReportEnabled} onChange={(e) => setOnuReportEnabled(e.target.checked)} style={{ marginTop: 3 }} />
              <span style={{ fontSize: 13 }}>
                <strong>Coletar no monitoramento</strong> — enriquece ONUs com dados CLI (modelo, RX/TX, estado).
              </span>
            </label>
            {onuReportEnabled ? (
              <div className="row" style={{ gap: 16, flexWrap: "wrap", marginBottom: 10, alignItems: "flex-end" }}>
                <label className="row" style={{ gap: 8, cursor: "pointer", fontSize: 13 }}>
                  <input type="checkbox" checked={onuReportOnlineOnly} onChange={(e) => setOnuReportOnlineOnly(e.target.checked)} />
                  Só ONUs online
                </label>
                <div className="field" style={{ margin: 0, maxWidth: 140 }}>
                  <label htmlFor="onu-telnet-max">Máx. ONUs/ciclo</label>
                  <input
                    id="onu-telnet-max"
                    className="input"
                    type="number"
                    min={1}
                    max={200}
                    value={onuReportMaxPerCycle}
                    onChange={(e) => setOnuReportMaxPerCycle(e.target.value)}
                  />
                </div>
              </div>
            ) : null}
            <div className="field">
              <label>Pré-comandos (um por linha)</label>
              <textarea
                className="input mono"
                rows={3}
                value={onuReportPreText}
                onChange={(e) => setOnuReportPreText(e.target.value)}
                placeholder={brand.toUpperCase().includes("ZTE") ? "terminal length 0\nterminal page-break disable" : "enable\n{enable}\nconf terminal"}
              />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label>Comandos por ONU (um por linha)</label>
              <textarea
                className="input mono"
                rows={4}
                value={onuReportCommandsText}
                onChange={(e) => setOnuReportCommandsText(e.target.value)}
                placeholder={
                  brand.toUpperCase().includes("ZTE")
                    ? "show gpon onu detail-info {gpon_onu}\nshow pon power onu-rx {gpon_onu}"
                    : "show onu info {pon} {onu}\nshow onu state {pon} {onu}"
                }
              />
            </div>
          </div>
{/* 2. Pesquisa por série */}
          <div className="card olt-telnet-block">
            <h4 style={{ margin: "0 0 4px", fontSize: 14 }}>2. Pesquisa por série</h4>
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 10px", maxWidth: 640 }}>
              Aba <strong>OLT → Pesquisa</strong>. Usa os <strong>pré-comandos do bloco 1</strong> (métricas ONU) e o comando abaixo.
            </p>
            <div className="field" style={{ marginBottom: 0 }}>
              <label>Comando de pesquisa</label>
              <input
                className="input mono"
                value={onuReportSerialSearchText}
                onChange={(e) => setOnuReportSerialSearchText(e.target.value)}
                placeholder={brand.toUpperCase().includes("ZTE") ? "show gpon onu by sn {serial}" : "show onu info {pon}"}
              />
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>
                Com <code>{"{serial}"}</code> a OLT procura directamente (ZTE). Sem <code>{"{serial}"}</code>, lista e filtra no
                NetQuasar (VSOL). Use <code>{"{pon}"}</code> para uma porta; na Pesquisa pode escolher «Todas».
              </p>
            </div>
          </div>
                </div>
              )}

              {editSection === "telnet-unauth" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Telnet — não autorizadas</h3>
                  {/* 3. ONUs não autorizadas */}
          <div className="card olt-telnet-block">
            <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, flexWrap: "wrap" }}>
              <div>
                <h4 style={{ margin: "0 0 4px", fontSize: 14 }}>3. ONUs não autorizadas</h4>
                <p style={{ fontSize: 11, color: "var(--muted)", margin: 0, maxWidth: 640 }}>
                  Aba <strong>OLT → Não autorizadas</strong>. Pré-comandos e comando <strong>próprios</strong> — não partilham com
                  métricas de ONU. Ex. VSOL: entrar em <code>interface gpon 0/4</code> e depois <code>show onu auto-find</code>.
                </p>
              </div>
              <button
                type="button"
                className="btn"
                style={{ fontSize: 12, padding: "4px 10px" }}
                onClick={() => {
                  const d = defaultUnauthorizedForBrand(brand);
                  setOnuUnauthorizedPreText(d.pre_commands.join("\n"));
                  setOnuUnauthorizedQueryCmd(d.command);
                  toastOk(pushToast, "Padrão de não autorizadas carregado. Ajuste a porta GPON se necessário e guarde.");
                }}
              >
                Padrão da marca
              </button>
            </div>
            <div className="field" style={{ marginTop: 12 }}>
              <label>Pré-comandos (um por linha)</label>
              <textarea
                className="input mono"
                rows={4}
                value={onuUnauthorizedPreText}
                onChange={(e) => setOnuUnauthorizedPreText(e.target.value)}
                placeholder={
                  brand.toUpperCase().includes("VSOL")
                    ? "enable\n{enable}\nconfigure terminal\ninterface gpon 0/{pon}"
                    : "terminal length 0"
                }
              />
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>
                VSOL: use <code>{"interface gpon 0/{pon}"}</code> — o NetQuasar percorre cada porta PON do snapshot (ex.{" "}
                <code>0/4</code>) e executa <code>show onu auto-find</code> em cada uma.
              </p>
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label>Comando de consulta</label>
              <input
                className="input mono"
                value={onuUnauthorizedQueryCmd}
                onChange={(e) => setOnuUnauthorizedQueryCmd(e.target.value)}
                placeholder={brand.toUpperCase().includes("VSOL") ? "show onu auto-find" : "show gpon onu uncfg"}
              />
            </div>
          </div>
                </div>
              )}

              {editSection === "telnet-auth" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Telnet — autorizar / VLAN</h3>
                  <div className="card olt-telnet-block">
                  <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, flexWrap: "wrap" }}>
                    <div>
                      <h4 style={{ margin: "0 0 4px", fontSize: 14 }}>Autorizar / desautorizar ONU</h4>
                      <p style={{ fontSize: 11, color: "var(--muted)", margin: 0, maxWidth: 640 }}>
                        Acções manuais na OLT. Usam os pré-comandos do relatório ONU. Envie <strong>um comando por linha</strong>.
                        Placeholders: <code>{"{pon}"}</code>, <code>{"{onu}"}</code>, <code>{"{serial}"}</code>,{" "}
                        <code>{"{vlan}"}</code> (mapa PON), <code>{"{name}"}</code> (padrão <code>PON-ID</code>).
                        Tipo fixo <code>GU201-G</code>. Um comando por linha; eth_0/1…eth_0/4.
                      </p>
                    </div>
                    <button
                      type="button"
                      className="btn"
                      style={{ fontSize: 12, padding: "4px 10px" }}
                      onClick={() => {
                        const d = defaultAuthCmdsForBrand(brand);
                        if (d.deauthorize) setOnuDeauthorizeCmd(d.deauthorize);
                        if (d.authorize) setOnuAuthorizeCmd(d.authorize);
                        toastOk(
                          pushToast,
                          d.authorize || d.deauthorize
                            ? "Padrão da marca carregado. Ajuste o script se necessário e clique em Guardar."
                            : "Sem padrão de autorização/desautorização para esta marca.",
                        );
                      }}
                    >
                      Padrão da marca
                    </button>
                  </div>

            <div
              style={{
                marginTop: 14,
                padding: 12,
                border: "1px solid var(--border)",
                borderRadius: 8,
                background: "var(--panel2, transparent)",
              }}
            >
              <h5 style={{ margin: "0 0 6px", fontSize: 13 }}>Mapa VLAN ↔ PON (SNMP)</h5>
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 10px", maxWidth: 720 }}>
                Obrigatório para autorizar: a VLAN vem sempre deste mapa (sem valor reserva). Descubra as VLANs na OLT,
                marque o que deve ser ignorado (ex. VLAN 1 e GERENCIA) e guarde o perfil.
              </p>
              <div className="field" style={{ marginBottom: 8 }}>
                <label>OID SNMP do catálogo</label>
                <input
                  className="input mono"
                  value={authorizeVlanSnmpOid}
                  onChange={(e) => setAuthorizeVlanSnmpOid(e.target.value)}
                  placeholder={DEFAULT_ZTE_VLAN_SNMP_OID}
                />
              </div>
              <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "flex-end", marginBottom: 10 }}>
                <div className="field" style={{ margin: 0, minWidth: 220, flex: "1 1 220px" }}>
                  <label>OLT para descoberta</label>
                  <select
                    className="input"
                    value={vlanDiscoverOltId}
                    onChange={(e) => setVlanDiscoverOltId(e.target.value)}
                    disabled={vlanDiscoverLoading}
                  >
                    <option value="">Seleccione…</option>
                    {(oltDevices.data?.olts ?? []).map((o) => (
                      <option key={o.id} value={o.id}>
                        {o.description ?? o.id}
                        {o.ip ? ` (${o.ip})` : ""}
                        {o.brand ? ` · ${o.brand}` : ""}
                      </option>
                    ))}
                  </select>
                </div>
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={!vlanDiscoverOltId || vlanDiscoverLoading}
                  onClick={() => {
                    void (async () => {
                      if (!vlanDiscoverOltId) return;
                      setVlanDiscoverLoading(true);
                      try {
                        const res = await apiFetch<{
                          ok?: boolean;
                          oid?: string;
                          entries?: OltAuthorizeVlanEntry[];
                          total?: number;
                          error?: string;
                        }>(`/api/v1/olt/devices/${vlanDiscoverOltId}/discover-vlans`, {
                          method: "POST",
                          json: { oid: authorizeVlanSnmpOid.trim() || DEFAULT_ZTE_VLAN_SNMP_OID },
                          timeoutMs: 90_000,
                        });
                        const prevByVid = new Map(authorizeVlanCatalog.map((e) => [e.vid, e.ignored === true]));
                        const next = (res.entries ?? []).map((e) => ({
                          ...e,
                          ignored: prevByVid.has(e.vid) ? prevByVid.get(e.vid) : e.ignored === true,
                        }));
                        setAuthorizeVlanCatalog(next);
                        if (res.oid) setAuthorizeVlanSnmpOid(res.oid);
                        toastOk(
                          pushToast,
                          `${next.length} VLAN(s) descobertas. Marque as que devem ser ignoradas e Guarde o perfil.`,
                        );
                      } catch (err) {
                        toastErr(pushToast, err, "Falha na descoberta SNMP de VLANs.");
                      } finally {
                        setVlanDiscoverLoading(false);
                      }
                    })();
                  }}
                >
                  {vlanDiscoverLoading ? "A consultar…" : "Descobrir VLANs (SNMP)"}
                </button>
                {authorizeVlanCatalog.length > 0 ? (
                  <button
                    type="button"
                    className="btn"
                    disabled={vlanDiscoverLoading}
                    onClick={() => setAuthorizeVlanCatalog([])}
                  >
                    Limpar mapa
                  </button>
                ) : null}
              </div>
              {authorizeVlanCatalog.length > 0 ? (
                <div className="table-wrap" style={{ maxHeight: 280, overflow: "auto" }}>
                  <table style={{ fontSize: 11, width: "100%" }}>
                    <thead>
                      <tr>
                        <th style={{ width: 72 }}>Ignorar</th>
                        <th>VID</th>
                        <th>Nome</th>
                        <th>Descrição</th>
                        <th>PON</th>
                      </tr>
                    </thead>
                    <tbody>
                      {authorizeVlanCatalog.map((e, idx) => (
                        <tr key={e.vid} style={e.ignored ? { opacity: 0.55 } : undefined}>
                          <td>
                            <input
                              type="checkbox"
                              checked={e.ignored === true}
                              title={e.ignored ? "Ignorada no provisionamento" : "Usada no provisionamento"}
                              onChange={(ev) => {
                                setAuthorizeVlanCatalog((prev) =>
                                  prev.map((row, i) => (i === idx ? { ...row, ignored: ev.target.checked } : row)),
                                );
                              }}
                            />
                          </td>
                          <td className="mono">{e.vid}</td>
                          <td className="mono">{e.name || "—"}</td>
                          <td>{e.description || "—"}</td>
                          <td className="mono">{e.pon && e.pon > 0 ? e.pon : "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
                  Ainda sem mapa. Escolha uma OLT e clique em Descobrir.
                </p>
              )}
              {authorizeVlanCatalog.length > 0 ? (
                <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
                  Activas: {authorizeVlanCatalog.filter((e) => !e.ignored).length} · Ignoradas:{" "}
                  {authorizeVlanCatalog.filter((e) => e.ignored).length}
                </p>
              ) : null}
            </div>

                  <div className="field" style={{ marginTop: 12 }}>
                    <label>Autorizar ONU</label>
                    <OltCmdTextarea
                      value={onuAuthorizeCmd}
                      onChange={setOnuAuthorizeCmd}
                      placeholder={
                        brand.toUpperCase().includes("ZTE")
                          ? "configure terminal\ninterface gpon_olt-1/1/{pon}\nonu {onu} type {onu_type} sn {serial}\n…"
                          : "ont add {pon} {onu} sn-auth {serial} omci ..."
                      }
                    />
                  </div>
                  <div className="field" style={{ marginBottom: 0 }}>
                    <label>Desautorizar ONU</label>
                    <OltCmdTextarea
                      value={onuDeauthorizeCmd}
                      onChange={setOnuDeauthorizeCmd}
                      placeholder={
                        brand.toUpperCase().includes("ZTE")
                          ? "configure terminal\ninterface gpon_olt-1/1/{pon}\nno onu {onu}\nexit\nexit"
                          : "ont delete {pon} {onu}"
                      }
                    />
                  </div>
                </div>
                </div>
              )}

              {editSection === "telnet-pon" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Telnet — PON/SFP</h3>
                  {/* 5. PON / SFP */}
          <div className="card olt-telnet-block">
            <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, flexWrap: "wrap" }}>
              <div>
                <h4 style={{ margin: "0 0 4px", fontSize: 14 }}>5. Métricas PON / SFP (GBIC)</h4>
                <p style={{ fontSize: 11, color: "var(--muted)", margin: 0, maxWidth: 640 }}>
                  Voltagem, temperatura, TX/RX e bias do módulo óptico por porta PON. Pré-comandos e comandos próprios.
                  Placeholder: <code>{"{pon}"}</code>.
                </p>
              </div>
              <button
                type="button"
                className="btn"
                style={{ fontSize: 12, padding: "4px 10px" }}
                onClick={() => {
                  const d = defaultPonTelnetForBrand(brand);
                  setPonTelnetPreText(d.pre_commands.join("\n"));
                  setPonTelnetCommandsText(d.commands.join("\n"));
                  toastOk(pushToast, "Padrão PON carregado. Clique em Guardar.");
                }}
              >
                Padrão da marca
              </button>
            </div>
            <label className="row" style={{ gap: 8, alignItems: "flex-start", margin: "12px 0 8px", cursor: "pointer" }}>
              <input type="checkbox" checked={ponTelnetEnabled} onChange={(e) => setPonTelnetEnabled(e.target.checked)} style={{ marginTop: 3 }} />
              <span style={{ fontSize: 13 }}>
                <strong>Coletar no monitoramento</strong> — actualiza a tabela de PONs após cada coleta.
              </span>
            </label>
            {ponTelnetEnabled ? (
              <div className="field" style={{ margin: "0 0 10px", maxWidth: 140 }}>
                <label htmlFor="pon-telnet-max">Máx. PONs/ciclo</label>
                <input
                  id="pon-telnet-max"
                  className="input"
                  type="number"
                  min={1}
                  max={64}
                  value={ponTelnetMaxPerCycle}
                  onChange={(e) => setPonTelnetMaxPerCycle(e.target.value)}
                />
              </div>
            ) : null}
            <div className="field">
              <label>Pré-comandos (um por linha)</label>
              <OltCmdTextarea
                value={ponTelnetPreText}
                onChange={setPonTelnetPreText}
                placeholder={
                  brand.toUpperCase().includes("ZTE")
                    ? "terminal length 0\nterminal page-break disable"
                    : "enable\n{enable}\nconf terminal"
                }
              />
            </div>
            <div className="field" style={{ marginBottom: 0 }}>
              <label>Comandos por PON (um por linha)</label>
              <OltCmdTextarea
                value={ponTelnetCommandsText}
                onChange={setPonTelnetCommandsText}
                placeholder={
                  brand.toUpperCase().includes("ZTE")
                    ? "show pon power olt-tx gpon-olt_1/1/{pon}\nshow optical-module-info gpon-olt_1/1/{pon}"
                    : "show pon optical-transceiver-diagnosis slot 0 pon {pon}"
                }
              />
            </div>
          </div>
                </div>
              )}

              {editSection === "advanced" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Opções avançadas</h3>
                  {steps.length === 0 && <p style={{ fontSize: 12, color: "var(--muted)" }}>Nenhum passo extra.</p>}
            {steps.map((st, idx) => (
              <div key={`${st.id ?? st.method}-${idx}`} className="card" style={{ marginTop: 8, padding: 10 }}>
                <div className="field" style={{ margin: 0 }}>
                  <label>Método</label>
                  <select
                    className="input"
                    value={st.method}
                    onChange={(e) => {
                      const next = [...steps];
                      next[idx] = { ...next[idx], method: e.target.value };
                      setSteps(next);
                    }}
                  >
                    {OLT_COLLECT_METHODS.map((m) => (
                      <option key={m.value} value={m.value}>
                        {m.label}
                      </option>
                    ))}
                  </select>
                </div>
                <button type="button" className="btn btn--danger" style={{ marginTop: 8 }} onClick={() => setSteps(steps.filter((_, i) => i !== idx))}>
                  Remover passo
                </button>
              </div>
            ))}
            <button
              type="button"
              className="btn"
              style={{ marginTop: 8 }}
              onClick={() => setSteps([...steps, { id: `step_${steps.length + 1}`, method: "telnet", enabled: true }])}
            >
              Adicionar passo extra
            </button>
                </div>
              )}

              {editSection === "vars" && (
                <div className="olt-profile-modal__section">
                  <h3 className="olt-profile-modal__section-title">Variáveis</h3>
                  <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
                    Placeholders disponíveis nos comandos telnet deste perfil.
                  </p>
                  <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                    {PROFILE_PLACEHOLDERS.map((ph) => (
                      <code key={ph} style={{ fontSize: 12 }}>
                        {ph}
                      </code>
                    ))}
                  </div>
                </div>
              )}
                </div>

                <aside className="olt-profile-modal__aside">
                  <div className="card" style={{ margin: 0 }}>
                <h3 style={{ marginTop: 0, fontSize: 14 }}>Resumo</h3>
                <ul style={{ margin: 0, paddingLeft: 18, fontSize: 12, lineHeight: 1.6 }}>
                  <li>
                    Métricas SNMP activas: <strong>{enabledMetricsCount}</strong>
                  </li>
                  <li>
                    Comandos telnet: <strong>{telnetCmdCount}</strong>
                  </li>
                  <li>
                    Mapa VLAN:{" "}
                    {authorizeVlanCatalog.length === 0 ? (
                      <span style={{ color: "var(--muted)" }}>vazio</span>
                    ) : (
                      <>
                        <strong>{vlanActive}</strong> activas · <strong>{vlanIgnored}</strong> ignoradas
                      </>
                    )}
                  </li>
                  <li>
                    Relatório ONU: {onuReportEnabled ? <span className="badge badge--ok">on</span> : <span className="badge">off</span>}
                  </li>
                  <li>
                    PON/SFP: {ponTelnetEnabled ? <span className="badge badge--ok">on</span> : <span className="badge">off</span>}
                  </li>
                </ul>
                <h4 style={{ fontSize: 13, margin: "14px 0 6px" }}>Variáveis</h4>
                <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                  {PROFILE_PLACEHOLDERS.map((ph) => (
                    <code key={ph} style={{ fontSize: 11 }}>
                      {ph}
                    </code>
                  ))}
                </div>
              </div>
                </aside>
              </div>
            )}
          </div>
        </div>
      )}

      {modals}
    </>
  );
}


export { OltVendorsPanel };
