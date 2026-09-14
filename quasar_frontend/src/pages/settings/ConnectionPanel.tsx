import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";

export function ConnectionPanel() {
  type CategoryOverrides = {
    cpu_oid?: string;
    cpu_available_oid?: string;
    memory_used_oid?: string;
    memory_size_oid?: string;
    temp_oid?: string;
    uptime_oid?: string;
    brand_oids?: string[];
    model_oids?: string[];
    serial_oids?: string[];
    software_oids?: string[];
    hardware_oids?: string[];
    sysname_oids?: string[];
    sysdescr_oids?: string[];
    interface_oids?: string[];
    optical_oids?: string[];
    pon_oids?: string[];
    onu_oids?: string[];
    bridge_oids?: string[];
    traffic_oids?: string[];
    custom_oids?: string[];
    /** OID normalizado (sem ponto inicial) → descrição mostrada no relatório. */
    oid_labels?: Record<string, string>;
  };
  type OverridesDoc = {
    olt?: CategoryOverrides;
    mikrotik?: CategoryOverrides;
    bng?: CategoryOverrides;
    servidor?: CategoryOverrides;
    bridge?: CategoryOverrides;
  };
  type OidExtraCategory = "olt" | "mikrotik" | "bng" | "servidor" | "bridge";
  type OidArrayKey = keyof Pick<
    CategoryOverrides,
    | "brand_oids"
    | "model_oids"
    | "serial_oids"
    | "software_oids"
    | "hardware_oids"
    | "sysname_oids"
    | "sysdescr_oids"
    | "interface_oids"
    | "traffic_oids"
    | "optical_oids"
    | "pon_oids"
    | "onu_oids"
    | "bridge_oids"
    | "custom_oids"
  >;
  type OidExtraKind =
    | "brand"
    | "model"
    | "serial"
    | "software"
    | "hardware"
    | "sysname"
    | "sysdescr"
    | "interface"
    | "traffic"
    | "optical"
    | "pon"
    | "onu"
    | "bridge"
    | "custom";
  type OidKindMeta = { value: OidExtraKind; label: string; jsonKey: OidArrayKey; defaultLabel: string };
  type ExtraOidRow = { id: string; kind: OidExtraKind; oid: string; label: string };

  const OID_KIND_GROUPS: { label: string; kinds: OidKindMeta[] }[] = [
    {
      label: "Inventário / identificação",
      kinds: [
        { value: "brand", label: "Fabricante / marca", jsonKey: "brand_oids", defaultLabel: "Fabricante" },
        { value: "model", label: "Modelo", jsonKey: "model_oids", defaultLabel: "Modelo" },
        { value: "serial", label: "Número de série", jsonKey: "serial_oids", defaultLabel: "Número de série" },
        { value: "software", label: "Versão de software / firmware", jsonKey: "software_oids", defaultLabel: "Versão de software" },
        { value: "hardware", label: "Versão de hardware", jsonKey: "hardware_oids", defaultLabel: "Versão de hardware" },
        { value: "sysname", label: "Nome do sistema (sysName)", jsonKey: "sysname_oids", defaultLabel: "Nome do sistema" },
        { value: "sysdescr", label: "Descrição do sistema (sysDescr)", jsonKey: "sysdescr_oids", defaultLabel: "Descrição do sistema" },
      ],
    },
    {
      label: "Rede / telemetria",
      kinds: [
        { value: "interface", label: "Interface", jsonKey: "interface_oids", defaultLabel: "Interface" },
        { value: "traffic", label: "Tráfego (banda RX/TX etc.)", jsonKey: "traffic_oids", defaultLabel: "Tráfego" },
        { value: "optical", label: "Óptica / SFP", jsonKey: "optical_oids", defaultLabel: "Óptica" },
        { value: "pon", label: "PON", jsonKey: "pon_oids", defaultLabel: "PON" },
        { value: "onu", label: "ONU", jsonKey: "onu_oids", defaultLabel: "ONU" },
        { value: "bridge", label: "Bridge", jsonKey: "bridge_oids", defaultLabel: "Bridge" },
      ],
    },
    {
      label: "Outros",
      kinds: [{ value: "custom", label: "Outro / personalizado", jsonKey: "custom_oids", defaultLabel: "Leitura extra" }],
    },
  ];

  const OID_KIND_META: OidKindMeta[] = OID_KIND_GROUPS.flatMap((g) => g.kinds);
  const OID_KIND_BY_VALUE = Object.fromEntries(OID_KIND_META.map((k) => [k.value, k])) as Record<OidExtraKind, OidKindMeta>;
  const OID_ARRAY_KEYS: OidArrayKey[] = OID_KIND_META.map((k) => k.jsonKey);

  /** OIDs reservados nos cartões de telemetria avançada (não aparecem na lista de extras). */
  const RESERVED_OID_SLOTS: Partial<Record<OidExtraCategory, Partial<Record<OidExtraKind, number>>>> = {
    olt: { onu: 1, pon: 2 },
    mikrotik: { interface: 1, traffic: 2, optical: 2 },
  };

  const compact = (arr: Array<string | undefined | null>): string[] =>
    arr.map((s) => String(s ?? "").trim()).filter((s) => s.length > 0);

  const newOidRowId = (): string =>
    typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `oid-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;

  const oidLabelMapFromUnknown = (raw: unknown): Record<string, string> => {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
      const kk = String(k ?? "")
        .trim()
        .replace(/^\./, "");
      const vv = String(v ?? "").trim();
      if (kk && vv) out[kk] = vv;
    }
    return out;
  };

  const mergeCategoryOidLabels = (baselineBlk: CategoryOverrides | undefined, rows: ExtraOidRow[]): Record<string, string> | undefined => {
    const m: Record<string, string> = { ...oidLabelMapFromUnknown(baselineBlk?.oid_labels) };
    for (const r of rows) {
      const o = String(r.oid ?? "")
        .trim()
        .replace(/^\./, "");
      const lbl = String(r.label ?? "").trim();
      if (!o) continue;
      if (lbl) m[o] = lbl;
      else delete m[o];
    }
    return Object.keys(m).length ? m : undefined;
  };

  const oidsInCategoryArrays = (blk: CategoryOverrides): Set<string> => {
    const s = new Set<string>();
    for (const key of OID_ARRAY_KEYS) {
      for (const x of blk[key] ?? []) {
        const o = String(x).trim().replace(/^\./, "");
        if (o) s.add(o);
      }
    }
    return s;
  };

  const pruneOidLabelsToBlock = (blk: CategoryOverrides, labels: Record<string, string> | undefined): Record<string, string> | undefined => {
    if (!labels) return undefined;
    const allowed = oidsInCategoryArrays(blk);
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(labels)) {
      const kk = String(k).trim().replace(/^\./, "");
      if (!kk || !String(v).trim()) continue;
      if (allowed.has(kk)) out[kk] = String(v).trim();
    }
    return Object.keys(out).length ? out : undefined;
  };

  const emptyExtraRows = (): Record<OidExtraCategory, ExtraOidRow[]> => ({
    olt: [],
    mikrotik: [],
    bng: [],
    servidor: [],
    bridge: [],
  });

  const OID_EXTRA_CATEGORY_LABELS: Record<OidExtraCategory, string> = {
    olt: "OLT",
    mikrotik: "MikroTik",
    bng: "BNG",
    servidor: "Servidor",
    bridge: "Pontes",
  };

  type TelemetryOidValues = {
    cpu: string;
    cpuAvail: string;
    memUsed: string;
    memSize: string;
    temp: string;
    uptime: string;
  };

  const renderBaseTelemetryFields = (values: TelemetryOidValues, onChange: (patch: Partial<TelemetryOidValues>) => void) => (
    <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
      <div className="field">
        <label>CPU utilizada (uso / carga)</label>
        <input className="input mono" value={values.cpu} onChange={(e) => onChange({ cpu: e.target.value })} />
      </div>
      <div className="field">
        <label>CPU disponível (% idle)</label>
        <input className="input mono" value={values.cpuAvail} onChange={(e) => onChange({ cpuAvail: e.target.value })} placeholder="opcional" />
      </div>
      <div className="field">
        <label>Memória em uso</label>
        <input className="input mono" value={values.memUsed} onChange={(e) => onChange({ memUsed: e.target.value })} />
      </div>
      <div className="field">
        <label>Memória total</label>
        <input className="input mono" value={values.memSize} onChange={(e) => onChange({ memSize: e.target.value })} />
      </div>
      <div className="field">
        <label>Temperatura</label>
        <input className="input mono" value={values.temp} onChange={(e) => onChange({ temp: e.target.value })} />
      </div>
      <div className="field">
        <label>Tempo ligado (uptime)</label>
        <input className="input mono" value={values.uptime} onChange={(e) => onChange({ uptime: e.target.value })} />
      </div>
    </div>
  );

  /** Junta OIDs extra por tipo; mantém ordem e remove duplicados vazios. */
  const mergeOidsByKind = (rows: ExtraOidRow[]): Record<OidExtraKind, string[]> => {
    const acc = {} as Record<OidExtraKind, string[]>;
    const seen = {} as Record<OidExtraKind, Set<string>>;
    for (const meta of OID_KIND_META) {
      acc[meta.value] = [];
      seen[meta.value] = new Set();
    }
    for (const r of rows) {
      const o = String(r.oid ?? "").trim();
      if (!o) continue;
      if (seen[r.kind].has(o)) continue;
      seen[r.kind].add(o);
      acc[r.kind].push(o);
    }
    return acc;
  };

  const mergeCategoryOidArrays = (
    block: CategoryOverrides,
    rows: ExtraOidRow[],
    reserved?: Partial<Record<OidExtraKind, string[]>>,
  ) => {
    const merged = mergeOidsByKind(rows);
    for (const meta of OID_KIND_META) {
      const combined = compact([...(reserved?.[meta.value] ?? []), ...merged[meta.value]]);
      if (combined.length) (block as Record<string, unknown>)[meta.jsonKey] = combined;
      else delete (block as Record<string, unknown>)[meta.jsonKey];
    }
    Object.keys(block).forEach((k) => {
      const v = (block as Record<string, unknown>)[k];
      if (v === undefined || (Array.isArray(v) && v.length === 0) || v === "") {
        delete (block as Record<string, unknown>)[k];
      }
    });
  };

  const loadCategoryExtraRows = (
    cat: OidExtraCategory,
    block: CategoryOverrides,
    labels: Record<string, string>,
    into: ExtraOidRow[],
    fromArr: (labels: Record<string, string>, kind: OidExtraKind, list: string[] | undefined) => ExtraOidRow[],
  ) => {
    for (const meta of OID_KIND_META) {
      const skip = RESERVED_OID_SLOTS[cat]?.[meta.value] ?? 0;
      const arr = (block[meta.jsonKey] ?? []).slice(skip);
      into.push(...fromArr(labels, meta.value, arr));
    }
  };

  /**
   * Lê o JSON salvo e separa (a) campos reservados dos cartões OLT/Mikrotik
   * e (b) restantes em linhas editáveis por categoria.
   */
  const extraRowsFromOverridesDoc = (doc: OverridesDoc): Record<OidExtraCategory, ExtraOidRow[]> => {
    const out = emptyExtraRows();
    const fromArr = (labels: Record<string, string>, kind: OidExtraKind, list: string[] | undefined): ExtraOidRow[] =>
      (list ?? [])
        .map((oid) => {
          const o = String(oid).trim();
          if (!o) return null;
          const norm = o.replace(/^\./, "");
          const label = String(labels[norm] ?? labels[o] ?? "").trim();
          return { id: newOidRowId(), kind, oid: o, label };
        })
        .filter((r): r is ExtraOidRow => r != null);

    const o = doc.olt ?? {};
    const m = doc.mikrotik ?? {};
    const bn = doc.bng ?? {};
    const s = doc.servidor ?? {};
    const b = doc.bridge ?? {};
    const lo = oidLabelMapFromUnknown(o.oid_labels);
    const lm = oidLabelMapFromUnknown(m.oid_labels);
    const lbn = oidLabelMapFromUnknown(bn.oid_labels);
    const ls = oidLabelMapFromUnknown(s.oid_labels);
    const lb = oidLabelMapFromUnknown(b.oid_labels);

    loadCategoryExtraRows("olt", o, lo, out.olt, fromArr);
    loadCategoryExtraRows("mikrotik", m, lm, out.mikrotik, fromArr);
    loadCategoryExtraRows("bng", bn, lbn, out.bng, fromArr);
    loadCategoryExtraRows("servidor", s, ls, out.servidor, fromArr);
    loadCategoryExtraRows("bridge", b, lb, out.bridge, fromArr);

    return out;
  };

  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["settings-conn-def"],
    queryFn: () =>
      apiFetch<{
        snmp_community: unknown;
        snmp_community_value: string;
        snmp_community_configured: boolean;
        telnet_user: string | null;
        telnet_password: string;
        telnet_password_configured: boolean;
        telnet_enable: string;
        telnet_enable_configured: boolean;
        ssh_user: string | null;
        ssh_password: string;
        ssh_password_configured: boolean;
        oid_defaults: {
          olt: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          mikrotik: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          bng: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
          server: { cpu_oid: string; cpu_available_oid?: string; memory_used_oid: string; memory_size_oid: string; temp_oid: string; uptime_oid: string };
        };
        snmp_oid_overrides: unknown;
        updated_at: string;
      }>("/api/v1/settings/connection/defaults"),
  });
  const [snmp, setSnmp] = useState("");
  const [tu, setTu] = useState("");
  const [tp, setTp] = useState("");
  const [te, setTe] = useState("");
  const [su, setSu] = useState("");
  const [sp, setSp] = useState("");
  const [oltCpu, setOltCpu] = useState("");
  const [oltCpuAvail, setOltCpuAvail] = useState("");
  const [oltMemUsed, setOltMemUsed] = useState("");
  const [oltMemSize, setOltMemSize] = useState("");
  const [oltTemp, setOltTemp] = useState("");
  const [oltUptime, setOltUptime] = useState("");
  const [mkCpu, setMkCpu] = useState("");
  const [mkCpuAvail, setMkCpuAvail] = useState("");
  const [mkMemUsed, setMkMemUsed] = useState("");
  const [mkMemSize, setMkMemSize] = useState("");
  const [mkTemp, setMkTemp] = useState("");
  const [mkUptime, setMkUptime] = useState("");
  const [bngCpu, setBngCpu] = useState("");
  const [bngCpuAvail, setBngCpuAvail] = useState("");
  const [bngMemUsed, setBngMemUsed] = useState("");
  const [bngMemSize, setBngMemSize] = useState("");
  const [bngTemp, setBngTemp] = useState("");
  const [bngUptime, setBngUptime] = useState("");
  const [svCpu, setSvCpu] = useState("");
  const [svCpuAvail, setSvCpuAvail] = useState("");
  const [svMemUsed, setSvMemUsed] = useState("");
  const [svMemSize, setSvMemSize] = useState("");
  const [svTemp, setSvTemp] = useState("");
  const [svUptime, setSvUptime] = useState("");
  const [oltOnuTotalOid, setOltOnuTotalOid] = useState("");
  const [oltPonTxOid, setOltPonTxOid] = useState("");
  const [oltPonStatusOid, setOltPonStatusOid] = useState("");
  const [mkInterfacesStatusOid, setMkInterfacesStatusOid] = useState("");
  const [mkBandwidthRxOid, setMkBandwidthRxOid] = useState("");
  const [mkBandwidthTxOid, setMkBandwidthTxOid] = useState("");
  const [mkSfpTxOid, setMkSfpTxOid] = useState("");
  const [mkSfpRxOid, setMkSfpRxOid] = useState("");
  /** Base vinda do servidor (preserva scalars/hand-edits não cobertos pela UI). */
  const [overridesBaseline, setOverridesBaseline] = useState<OverridesDoc>({});
  const [extraOidRows, setExtraOidRows] = useState<Record<OidExtraCategory, ExtraOidRow[]>>(emptyExtraRows);
  const [showGeneratedJson, setShowGeneratedJson] = useState(false);

  useEffect(() => {
    if (!q.data) return;
    setSnmp((v) => (v === "" ? q.data.snmp_community_value ?? "" : v));
    setTu(q.data.telnet_user ?? "");
    setSu(q.data.ssh_user ?? "");
    setOltCpu(q.data.oid_defaults?.olt?.cpu_oid ?? "");
    setOltCpuAvail(q.data.oid_defaults?.olt?.cpu_available_oid ?? "");
    setOltMemUsed(q.data.oid_defaults?.olt?.memory_used_oid ?? "");
    setOltMemSize(q.data.oid_defaults?.olt?.memory_size_oid ?? "");
    setOltTemp(q.data.oid_defaults?.olt?.temp_oid ?? "");
    setOltUptime(q.data.oid_defaults?.olt?.uptime_oid ?? "");
    setMkCpu(q.data.oid_defaults?.mikrotik?.cpu_oid ?? "");
    setMkCpuAvail(q.data.oid_defaults?.mikrotik?.cpu_available_oid ?? "");
    setMkMemUsed(q.data.oid_defaults?.mikrotik?.memory_used_oid ?? "");
    setMkMemSize(q.data.oid_defaults?.mikrotik?.memory_size_oid ?? "");
    setMkTemp(q.data.oid_defaults?.mikrotik?.temp_oid ?? "");
    setMkUptime(q.data.oid_defaults?.mikrotik?.uptime_oid ?? "");
    setBngCpu(q.data.oid_defaults?.bng?.cpu_oid ?? "");
    setBngCpuAvail(q.data.oid_defaults?.bng?.cpu_available_oid ?? "");
    setBngMemUsed(q.data.oid_defaults?.bng?.memory_used_oid ?? "");
    setBngMemSize(q.data.oid_defaults?.bng?.memory_size_oid ?? "");
    setBngTemp(q.data.oid_defaults?.bng?.temp_oid ?? "");
    setBngUptime(q.data.oid_defaults?.bng?.uptime_oid ?? "");
    setSvCpu(q.data.oid_defaults?.server?.cpu_oid ?? "");
    setSvCpuAvail(q.data.oid_defaults?.server?.cpu_available_oid ?? "");
    setSvMemUsed(q.data.oid_defaults?.server?.memory_used_oid ?? "");
    setSvMemSize(q.data.oid_defaults?.server?.memory_size_oid ?? "");
    setSvTemp(q.data.oid_defaults?.server?.temp_oid ?? "");
    setSvUptime(q.data.oid_defaults?.server?.uptime_oid ?? "");
    try {
      const parsed = (q.data.snmp_oid_overrides ?? {}) as OverridesDoc;
      setOverridesBaseline(JSON.parse(JSON.stringify(parsed)) as OverridesDoc);
      const olt = parsed?.olt ?? {};
      const mikrotik = parsed?.mikrotik ?? {};
      setOltOnuTotalOid(olt.onu_oids?.[0] ?? "");
      setOltPonTxOid(olt.pon_oids?.[0] ?? "");
      setOltPonStatusOid(olt.pon_oids?.[1] ?? "");
      setMkInterfacesStatusOid(mikrotik.interface_oids?.[0] ?? "");
      setMkBandwidthRxOid(mikrotik.traffic_oids?.[0] ?? "");
      setMkBandwidthTxOid(mikrotik.traffic_oids?.[1] ?? "");
      setMkSfpTxOid(mikrotik.optical_oids?.[0] ?? "");
      setMkSfpRxOid(mikrotik.optical_oids?.[1] ?? "");
      setExtraOidRows(extraRowsFromOverridesDoc(parsed));
    } catch {
      setOverridesBaseline({});
      setExtraOidRows(emptyExtraRows());
    }
  }, [q.data]);

  const builtOverridesPreview = (): OverridesDoc => {
    const base = JSON.parse(JSON.stringify(overridesBaseline)) as OverridesDoc;
    base.olt = base.olt ?? {};
    base.mikrotik = base.mikrotik ?? {};
    base.bng = base.bng ?? {};
    base.servidor = base.servidor ?? {};
    base.bridge = base.bridge ?? {};

    mergeCategoryOidArrays(base.olt as CategoryOverrides, extraOidRows.olt, {
      onu: compact([oltOnuTotalOid]),
      pon: compact([oltPonTxOid, oltPonStatusOid]),
    });
    delete (base.olt as CategoryOverrides).oid_labels;
    const oltOidLabels = pruneOidLabelsToBlock(
      base.olt as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.olt, extraOidRows.olt),
    );
    if (oltOidLabels) (base.olt as CategoryOverrides).oid_labels = oltOidLabels;

    mergeCategoryOidArrays(base.mikrotik as CategoryOverrides, extraOidRows.mikrotik, {
      interface: compact([mkInterfacesStatusOid]),
      traffic: compact([mkBandwidthRxOid, mkBandwidthTxOid]),
      optical: compact([mkSfpTxOid, mkSfpRxOid]),
    });
    delete (base.mikrotik as CategoryOverrides).oid_labels;
    const mkOidLabels = pruneOidLabelsToBlock(
      base.mikrotik as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.mikrotik, extraOidRows.mikrotik),
    );
    if (mkOidLabels) (base.mikrotik as CategoryOverrides).oid_labels = mkOidLabels;

    mergeCategoryOidArrays(base.bng as CategoryOverrides, extraOidRows.bng);
    delete (base.bng as CategoryOverrides).oid_labels;
    const bngOidLabels = pruneOidLabelsToBlock(
      base.bng as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.bng, extraOidRows.bng),
    );
    if (bngOidLabels) (base.bng as CategoryOverrides).oid_labels = bngOidLabels;

    mergeCategoryOidArrays(base.servidor as CategoryOverrides, extraOidRows.servidor);
    delete (base.servidor as CategoryOverrides).oid_labels;
    const srvOidLabels = pruneOidLabelsToBlock(
      base.servidor as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.servidor, extraOidRows.servidor),
    );
    if (srvOidLabels) (base.servidor as CategoryOverrides).oid_labels = srvOidLabels;

    mergeCategoryOidArrays(base.bridge as CategoryOverrides, extraOidRows.bridge);
    delete (base.bridge as CategoryOverrides).oid_labels;
    const brOidLabels = pruneOidLabelsToBlock(
      base.bridge as CategoryOverrides,
      mergeCategoryOidLabels(overridesBaseline.bridge, extraOidRows.bridge),
    );
    if (brOidLabels) (base.bridge as CategoryOverrides).oid_labels = brOidLabels;

    (["olt", "mikrotik", "bng", "servidor", "bridge"] as const).forEach((ck) => {
      const blk = base[ck] as Record<string, unknown> | undefined;
      if (blk && Object.keys(blk).length === 0) {
        delete base[ck];
      }
    });
    return base;
  };

  const addExtraRow = (cat: OidExtraCategory, kind?: OidExtraKind) => {
    const k = kind ?? "brand";
    setExtraOidRows((prev) => ({
      ...prev,
      [cat]: [...prev[cat], { id: newOidRowId(), kind: k, oid: "", label: OID_KIND_BY_VALUE[k]?.defaultLabel ?? "" }],
    }));
  };

  const removeExtraRow = (cat: OidExtraCategory, id: string) =>
    setExtraOidRows((prev) => ({ ...prev, [cat]: prev[cat].filter((r) => r.id !== id) }));

  const updateExtraRow = (cat: OidExtraCategory, id: string, patchRow: Partial<Pick<ExtraOidRow, "kind" | "oid" | "label">>) =>
    setExtraOidRows((prev) => ({
      ...prev,
      [cat]: prev[cat].map((r) => {
        if (r.id !== id) return r;
        const next = { ...r, ...patchRow };
        if (patchRow.kind && patchRow.kind !== r.kind && !String(next.label ?? "").trim()) {
          next.label = OID_KIND_BY_VALUE[patchRow.kind]?.defaultLabel ?? "";
        }
        return next;
      }),
    }));

  const renderOidExtrasBlock = (cat: OidExtraCategory, title: string) => {
    const rows = extraOidRows[cat];
    return (
      <div className="settings-conn-block" style={{ marginTop: 10 }}>
        <h4 style={{ marginTop: 0 }}>
          {title}
          <InfoHint label="Leituras SNMP extra">
            <p>
              Um identificador SNMP por linha. Escolha o tipo (fabricante, modelo, série, interface, PON, etc.) para organizar os dados ao salvar.
              A descrição aparece nos relatórios de telemetria.
            </p>
          </InfoHint>
        </h4>
        {rows.length === 0 ? (
          <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Nenhum extra — use «Adicionar» para incluir mais leituras.</p>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {rows.map((r) => (
              <div key={r.id} className="row" style={{ flexWrap: "wrap", gap: 6, alignItems: "flex-end" }}>
                <select
                  title="Tipo de métrica"
                  aria-label="Tipo de métrica SNMP"
                  className="select"
                  style={{ minWidth: 220, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.kind}
                  onChange={(e) => updateExtraRow(cat, r.id, { kind: e.target.value as OidExtraKind })}
                >
                  {OID_KIND_GROUPS.map((group) => (
                    <optgroup key={group.label} label={group.label}>
                      {group.kinds.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </optgroup>
                  ))}
                </select>
                <input
                  title="Identificador numérico SNMP"
                  aria-label="Identificador SNMP"
                  className="input mono"
                  style={{ flex: "1 1 160px", minWidth: 140, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.oid}
                  onChange={(e) => updateExtraRow(cat, r.id, { oid: e.target.value })}
                />
                <input
                  title="Descrição no relatório"
                  aria-label="Descrição da leitura SNMP extra"
                  className="input"
                  placeholder="Descrição (relatório)"
                  style={{ flex: "1 1 140px", minWidth: 120, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                  value={r.label}
                  onChange={(e) => updateExtraRow(cat, r.id, { label: e.target.value })}
                />
                <button type="button" className="btn" style={{ padding: "4px 8px", fontSize: 11 }} onClick={() => removeExtraRow(cat, r.id)}>
                  âˆ’
                </button>
              </div>
            ))}
          </div>
        )}
        <div className="row" style={{ marginTop: 8, gap: 6 }}>
          <button type="button" className="btn btn--primary" style={{ padding: "4px 10px", fontSize: 11 }} onClick={() => addExtraRow(cat)}>
            Adicionar
          </button>
        </div>
      </div>
    );
  };

  const patch = useMutation({
    mutationFn: () => {
      const parsedOverrides = builtOverridesPreview();
      return apiFetch("/api/v1/settings/connection/defaults", {
        method: "PATCH",
        json: {
          snmp_community: snmp || undefined,
          telnet_user: tu || undefined,
          telnet_password: tp || undefined,
          telnet_enable: te || undefined,
          ssh_user: su || undefined,
          ssh_password: sp || undefined,
          olt_cpu_oid: oltCpu || undefined,
          olt_cpu_available_oid: oltCpuAvail || undefined,
          olt_memory_used_oid: oltMemUsed || undefined,
          olt_memory_size_oid: oltMemSize || undefined,
          olt_temp_oid: oltTemp || undefined,
          olt_uptime_oid: oltUptime || undefined,
          mikrotik_cpu_oid: mkCpu || undefined,
          mikrotik_cpu_available_oid: mkCpuAvail || undefined,
          mikrotik_memory_used_oid: mkMemUsed || undefined,
          mikrotik_memory_size_oid: mkMemSize || undefined,
          mikrotik_temp_oid: mkTemp || undefined,
          mikrotik_uptime_oid: mkUptime || undefined,
          bng_cpu_oid: bngCpu || undefined,
          bng_cpu_available_oid: bngCpuAvail || undefined,
          bng_memory_used_oid: bngMemUsed || undefined,
          bng_memory_size_oid: bngMemSize || undefined,
          bng_temp_oid: bngTemp || undefined,
          bng_uptime_oid: bngUptime || undefined,
          server_cpu_oid: svCpu || undefined,
          server_cpu_available_oid: svCpuAvail || undefined,
          server_memory_used_oid: svMemUsed || undefined,
          server_memory_size_oid: svMemSize || undefined,
          server_temp_oid: svTemp || undefined,
          server_uptime_oid: svUptime || undefined,
          snmp_oid_overrides: parsedOverrides,
        },
      });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settings-conn-def"] }),
  });

  if (q.isLoading) return <p>A carregar…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Credenciais e leituras SNMP por defeito</h2>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>
        Valores aplicados quando o equipamento não traz credenciais próprias. Palavras-passe não são mostradas ao abrir esta página.
        {q.data?.updated_at ? ` Última alteração: ${q.data.updated_at}` : ""}
      </p>
      <div className="row" style={{ gap: 10, marginBottom: 8 }}>
        <span className={q.data?.snmp_community_configured ? "badge badge--ok" : "badge badge--off"}>
          Comunidade SNMP: {q.data?.snmp_community_configured ? "definida" : "não definida"}
        </span>
        <span className={q.data?.telnet_password_configured ? "badge badge--ok" : "badge badge--off"}>
          Palavra-passe Telnet: {q.data?.telnet_password_configured ? "definida" : "não definida"}
        </span>
        <span className={q.data?.ssh_password_configured ? "badge badge--ok" : "badge badge--off"}>
          Palavra-passe SSH: {q.data?.ssh_password_configured ? "definida" : "não definida"}
        </span>
      </div>
      <div className="settings-conn-section" style={{ marginTop: 14 }}>
        <h3 style={{ marginTop: 0 }}>Credenciais</h3>
        <div className="field">
          <label>Comunidade SNMP padrão</label>
          <input className="input" value={snmp} onChange={(e) => setSnmp(e.target.value)} />
        </div>
        <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
          <div className="field" style={{ minWidth: 220 }}><label>Usuário Telnet</label><input className="input" value={tu} onChange={(e) => setTu(e.target.value)} /></div>
          <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe Telnet</label><input className="input" type="password" value={tp} onChange={(e) => setTp(e.target.value)} /></div>
          <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe enable (Telnet)</label><input className="input" type="password" value={te} onChange={(e) => setTe(e.target.value)} /></div>
        </div>
        <div className="row" style={{ flexWrap: "wrap", gap: 8, marginTop: 8 }}>
          <div className="field" style={{ minWidth: 220 }}><label>Usuário SSH</label><input className="input" value={su} onChange={(e) => setSu(e.target.value)} /></div>
          <div className="field" style={{ minWidth: 220 }}><label>Palavra-passe SSH</label><input className="input" type="password" value={sp} onChange={(e) => setSp(e.target.value)} /></div>
        </div>
      </div>
      <div className="settings-conn-section">
        <h3 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Leituras SNMP por tipo de equipamento
          <InfoHint label="OIDs SNMP preferidos">
            <p>
              Se preencher, estes endereços têm prioridade sobre a descoberta automática. Em «CPU utilizada» indique a carga; em «CPU disponível» use normalmente
              a percentagem em idle (ociosidade). O painel tenta primeiro a utilizada e só depois deriva a partir da disponível (100 − idle).
            </p>
          </InfoHint>
        </h3>
        <div className="settings-conn-block">
          <h4>OLT</h4>
          {renderBaseTelemetryFields(
            { cpu: oltCpu, cpuAvail: oltCpuAvail, memUsed: oltMemUsed, memSize: oltMemSize, temp: oltTemp, uptime: oltUptime },
            (p) => {
              if (p.cpu !== undefined) setOltCpu(p.cpu);
              if (p.cpuAvail !== undefined) setOltCpuAvail(p.cpuAvail);
              if (p.memUsed !== undefined) setOltMemUsed(p.memUsed);
              if (p.memSize !== undefined) setOltMemSize(p.memSize);
              if (p.temp !== undefined) setOltTemp(p.temp);
              if (p.uptime !== undefined) setOltUptime(p.uptime);
            },
          )}
        </div>
        <div className="settings-conn-block">
          <h4>MikroTik</h4>
          {renderBaseTelemetryFields(
            { cpu: mkCpu, cpuAvail: mkCpuAvail, memUsed: mkMemUsed, memSize: mkMemSize, temp: mkTemp, uptime: mkUptime },
            (p) => {
              if (p.cpu !== undefined) setMkCpu(p.cpu);
              if (p.cpuAvail !== undefined) setMkCpuAvail(p.cpuAvail);
              if (p.memUsed !== undefined) setMkMemUsed(p.memUsed);
              if (p.memSize !== undefined) setMkMemSize(p.memSize);
              if (p.temp !== undefined) setMkTemp(p.temp);
              if (p.uptime !== undefined) setMkUptime(p.uptime);
            },
          )}
        </div>
        <div className="settings-conn-block">
          <h4>Servidor</h4>
          {renderBaseTelemetryFields(
            { cpu: svCpu, cpuAvail: svCpuAvail, memUsed: svMemUsed, memSize: svMemSize, temp: svTemp, uptime: svUptime },
            (p) => {
              if (p.cpu !== undefined) setSvCpu(p.cpu);
              if (p.cpuAvail !== undefined) setSvCpuAvail(p.cpuAvail);
              if (p.memUsed !== undefined) setSvMemUsed(p.memUsed);
              if (p.memSize !== undefined) setSvMemSize(p.memSize);
              if (p.temp !== undefined) setSvTemp(p.temp);
              if (p.uptime !== undefined) setSvUptime(p.uptime);
            },
          )}
        </div>
      </div>

      <div className="settings-conn-section">
        <h3 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Telemetria avançada
          <InfoHint label="PON, interfaces e SFP">
            <p>Campos rápidos para métricas frequentes em OLT e MikroTik. O restante pode ser configurado na secção de leituras extra.</p>
          </InfoHint>
        </h3>
        <div className="settings-conn-block">
          <h4>OLT — PON / GBIC / ONU</h4>
          <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Total de ONUs</label>
              <input className="input mono" value={oltOnuTotalOid} onChange={(e) => setOltOnuTotalOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Potência TX da PON</label>
              <input className="input mono" value={oltPonTxOid} onChange={(e) => setOltPonTxOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Status da PON</label>
              <input className="input mono" value={oltPonStatusOid} onChange={(e) => setOltPonStatusOid(e.target.value)} />
            </div>
          </div>
        </div>
        <div className="settings-conn-block">
          <h4 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
            MikroTik — interfaces / SFP
            <InfoHint label="MikroTik SFP e interfaces">
              <p>
                A página de interfaces faz walk em <span className="mono">mtxrOpticalTable</span> (<span className="mono">1.3.6.1.4.1.14988.1.1.19</span>, MIB MIKROTIK) e em{" "}
                <span className="mono">mtxrInterfaceStatsName</span> (<span className="mono">1.3.6.1.4.1.14988.1.1.14.1.1.2</span>) para obter o nome igual ao{" "}
                <span className="mono">ifName</span>. Potências: colunas <strong>9</strong> (TX) e <strong>10</strong> (RX), tipo <strong>IDiv1000</strong> (milésimos de dBm). O índice da linha mtxr não é o ifIndex; o cruzamento usa o nome de <span className="mono">…14.1.1.2</span>, o valor de{" "}
                <span className="mono">mtxrOpticalIndex</span> (col.1) quando coincidir com um ifIndex, e heurísticas sobre <span className="mono">mtxrOpticalName</span> (col.2). Os campos abaixo são OIDs <strong>opcionais</strong> para telemetria SNMP GET.
              </p>
            </InfoHint>
          </h4>
          <div className="row" style={{ flexWrap: "wrap", gap: 8 }}>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Status das interfaces</label>
              <input className="input mono" value={mkInterfacesStatusOid} onChange={(e) => setMkInterfacesStatusOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Banda recebida (RX)</label>
              <input className="input mono" value={mkBandwidthRxOid} onChange={(e) => setMkBandwidthRxOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Banda enviada (TX)</label>
              <input className="input mono" value={mkBandwidthTxOid} onChange={(e) => setMkBandwidthTxOid(e.target.value)} />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Potência SFP (TX)</label>
              <input
                className="input mono"
                value={mkSfpTxOid}
                onChange={(e) => setMkSfpTxOid(e.target.value)}
                placeholder="1.3.6.1.4.1.14988.1.1.19.1.1.9"
              />
            </div>
            <div className="field" style={{ minWidth: 260 }}>
              <label>Potência SFP (RX)</label>
              <input
                className="input mono"
                value={mkSfpRxOid}
                onChange={(e) => setMkSfpRxOid(e.target.value)}
                placeholder="1.3.6.1.4.1.14988.1.1.19.1.1.10"
              />
            </div>
          </div>
        </div>
      </div>

      <div className="settings-conn-section">
        <h3 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          Leituras SNMP extra
          <InfoHint label="OIDs adicionais">
            <p>Use quando precisar de mais objetos além dos cartões acima. Ao salvar, tudo é enviado para o servidor de forma estruturada (sem editar JSON à mão).</p>
          </InfoHint>
        </h3>
        {renderOidExtrasBlock("olt", "OLT")}
        {renderOidExtrasBlock("mikrotik", "MikroTik")}
        {renderOidExtrasBlock("servidor", "Servidor")}
        {renderOidExtrasBlock("bridge", "Pontes")}
      </div>
      <div className="settings-conn-section">
        <div className="settings-conn-block">
          <h4 style={{ marginTop: 0 }}>Resumo dos extras configurados</h4>
        {(["olt", "mikrotik", "bng", "servidor", "bridge"] as const).map((cat) => {
          const block = builtOverridesPreview()[cat];
          const list = OID_ARRAY_KEYS.flatMap((key) => block?.[key] ?? []).filter((v) => String(v).trim() !== "");
          return (
            <div key={cat} style={{ marginBottom: 8 }}>
              <strong>{OID_EXTRA_CATEGORY_LABELS[cat]}</strong>
              {list.length === 0 ? (
                <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--muted)" }}>Sem extras configurados.</p>
              ) : (
                <div className="row" style={{ gap: 6, flexWrap: "wrap", marginTop: 4 }}>
                  {list.map((oid) => (
                    <span key={`${cat}-${oid}`} className="badge badge--off mono" title={oid}>
                      {oid}
                    </span>
                  ))}
                </div>
              )}
            </div>
          );
        })}
        </div>
      </div>
      <div className="field" style={{ marginTop: 12 }}>
        <label className="row" style={{ gap: 8, alignItems: "center", cursor: "pointer" }}>
          <input type="checkbox" checked={showGeneratedJson} onChange={(e) => setShowGeneratedJson(e.target.checked)} />
          Mostrar pré-visualização técnica (JSON)
        </label>
        {showGeneratedJson && (
          <pre className="mono" style={{ fontSize: 10, marginTop: 8, padding: 8, overflow: "auto", maxHeight: 240, background: "var(--panel2, #161b22)", borderRadius: 6 }}>
            {JSON.stringify(builtOverridesPreview(), null, 2)}
          </pre>
        )}
      </div>
      <button type="button" className="btn btn--primary" style={{ marginTop: 12 }} disabled={patch.isPending} onClick={() => patch.mutate()}>
        Salvar credenciais e SNMP
      </button>
      {patch.isError && <div className="msg msg--err">{(patch.error as Error).message}</div>}
      {patch.isSuccess && <div className="msg msg--ok">Alterações salvas.</div>}
    </div>
  );
}
