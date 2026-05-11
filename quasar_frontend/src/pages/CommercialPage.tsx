import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch, downloadBlob } from "../lib/api";
import { apiUrl, getStoredApiKey, isAdminUser } from "../lib/auth";
import { formatAlertDateTimePt } from "../lib/alertLabels";
import { formatYearMonthPt, monthSelectChoicesWithFallback, recentYearMonthChoices } from "../lib/yearMonthPt";

type Locality = { id: string; name: string; region_code?: string | null; created_at?: string };
type MonthlyRecord = {
  id: string;
  locality_id: string;
  year_month: string;
  client_count: number;
  created_at?: string;
};

type AggregatesResponse = {
  total_clients: number;
  month: string;
};
type MonthCompareRow = {
  locality_id: string;
  locality_name: string;
  current: number;
  previous: number;
  delta: number;
  delta_percent: number;
};

type CmpSortKey = "locality" | "previous" | "current" | "delta" | "delta_pct";

type OltDeviceLite = {
  id: string;
  description?: string | null;
  category?: string | null;
  locality_id?: string | null;
  locality_name?: string | null;
  ip?: string | null;
};

type OltCollectRow = {
  olt_id: string;
  olt_description: string;
  locality_id: string;
  locality_name: string;
  online: number;
  offline: number;
  total: number;
  error?: string;
};

function computeOnuTotalsFromSnapshot(payload: Record<string, unknown>): { online: number; offline: number; total: number } {
  const computed = (payload.computed ?? {}) as Record<string, unknown>;
  const pons = Array.isArray(payload.pons_table) ? (payload.pons_table as Array<Record<string, unknown>>) : [];
  const toInt = (v: unknown): number => (Number.isFinite(Number(v)) ? Number(v) : 0);
  const fromComputed = {
    online: toInt(computed.onu_online_sum),
    offline: toInt(computed.onu_offline_sum),
    total: toInt(computed.onu_total_sum),
  };
  if (fromComputed.total > 0 || fromComputed.online > 0 || fromComputed.offline > 0) return fromComputed;
  let online = 0;
  let offline = 0;
  let total = 0;
  for (const p of pons) {
    online += toInt(p.onu_online);
    offline += toInt(p.onu_offline);
    total += toInt(p.onu_total);
  }
  return { online, offline, total };
}

function seededYearMonth(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

export function CommercialPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const seed = seededYearMonth();
  const monthChoices = useMemo(() => recentYearMonthChoices(72), []);
  const locs = useQuery({
    queryKey: ["commercial-loc"],
    queryFn: () => apiFetch<{ localities: Locality[] }>("/api/v1/commercial/localities"),
  });
  const recs = useQuery({
    queryKey: ["commercial-rec"],
    queryFn: () => apiFetch<{ records: MonthlyRecord[] }>("/api/v1/commercial/monthly-records"),
  });
  const [month, setMonth] = useState(seed);
  const [tgSendConfirmOpen, setTgSendConfirmOpen] = useState(false);
  const agg = useQuery({
    queryKey: ["commercial-agg", month],
    queryFn: () => apiFetch<AggregatesResponse>(`/api/v1/commercial/aggregates?month=${encodeURIComponent(month)}`),
  });
  const cmp = useQuery({
    queryKey: ["commercial-cmp", month],
    queryFn: () => apiFetch<{ month: string; previous_month: string; rows: MonthCompareRow[] }>(`/api/v1/commercial/comparison?month=${encodeURIComponent(month)}`),
  });
  const [cmpSort, setCmpSort] = useState<{ key: CmpSortKey; dir: "asc" | "desc" }>({ key: "locality", dir: "asc" });

  const cmpSortedRows = useMemo(() => {
    const rows = [...(cmp.data?.rows ?? [])];
    const { key, dir } = cmpSort;
    const mul = dir === "asc" ? 1 : -1;
    rows.sort((a, b) => {
      let d = 0;
      switch (key) {
        case "locality":
          d = a.locality_name.localeCompare(b.locality_name, "pt");
          break;
        case "previous":
          d = a.previous - b.previous;
          break;
        case "current":
          d = a.current - b.current;
          break;
        case "delta":
          d = a.delta - b.delta;
          break;
        case "delta_pct": {
          const ap = Number.isFinite(a.delta_percent) ? a.delta_percent : 0;
          const bp = Number.isFinite(b.delta_percent) ? b.delta_percent : 0;
          d = ap - bp;
          break;
        }
      }
      if (d !== 0) return mul * d;
      return a.locality_id.localeCompare(b.locality_id);
    });
    return rows;
  }, [cmp.data?.rows, cmpSort]);

  const onCmpSortClick = useCallback((nextKey: CmpSortKey) => {
    setCmpSort((prev) =>
      prev.key === nextKey ? { key: nextKey, dir: prev.dir === "asc" ? "desc" : "asc" } : { key: nextKey, dir: "asc" },
    );
  }, []);

  const cmpSortMark = useCallback((key: CmpSortKey) => (cmpSort.key === key ? (cmpSort.dir === "asc" ? " ▲" : " ▼") : ""), [cmpSort]);
  const devices = useQuery({
    queryKey: ["commercial-olt-devices"],
    queryFn: () => apiFetch<{ devices: OltDeviceLite[] }>("/api/v1/devices"),
  });

  const locById = useMemo(() => {
    const m = new Map<string, string>();
    for (const l of locs.data?.localities ?? []) m.set(l.id, l.name);
    return m;
  }, [locs.data?.localities]);

  const chartRows = useMemo(() => {
    const totals = new Map<string, number>();
    for (const r of recs.data?.records ?? []) {
      if (r.year_month !== month) continue;
      totals.set(r.locality_id, (totals.get(r.locality_id) ?? 0) + r.client_count);
    }
    const rows = [...totals.entries()].map(([lid, count]) => ({
      lid,
      label: locById.get(lid) ?? lid,
      count,
    }));
    rows.sort((a, b) => b.count - a.count);
    const max = Math.max(1, ...rows.map((r) => r.count));
    const sum = rows.reduce((acc, r) => acc + r.count, 0);
    return { rows, max, sum };
  }, [recs.data?.records, month, locById]);

  const recsSorted = useMemo(() => {
    return [...(recs.data?.records ?? [])].sort((a, b) => {
      if (a.year_month !== b.year_month) return b.year_month.localeCompare(a.year_month);
      const na = locById.get(a.locality_id) ?? a.locality_id;
      const nb = locById.get(b.locality_id) ?? b.locality_id;
      return na.localeCompare(nb, "pt");
    });
  }, [recs.data?.records, locById]);

  const [locModalOpen, setLocModalOpen] = useState(false);
  const [mainTab, setMainTab] = useState<"resumo" | "localidades" | "registros">("resumo");
  const [newRecMenuOpen, setNewRecMenuOpen] = useState(false);
  const newRecMenuRef = useRef<HTMLDivElement>(null);
  const [singleRecModalOpen, setSingleRecModalOpen] = useState(false);
  const [recEditOpen, setRecEditOpen] = useState(false);
  const [editRecRow, setEditRecRow] = useState<MonthlyRecord | null>(null);
  const [recEditYm, setRecEditYm] = useState("");
  const [recEditCnt, setRecEditCnt] = useState("");
  const [locName, setLocName] = useState("");
  const [locRc, setLocRc] = useState("");
  const createLoc = useMutation({
    mutationFn: () => apiFetch("/api/v1/commercial/localities", { method: "POST", json: { name: locName, region_code: locRc || null } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commercial-loc"] });
      setLocName("");
      setLocRc("");
      setLocModalOpen(false);
      setTgMsg({ ok: true, text: "Localidade adicionada com sucesso." });
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message }),
  });

  const [editingLocId, setEditingLocId] = useState<string | null>(null);
  const [editLocName, setEditLocName] = useState("");
  const [editLocRc, setEditLocRc] = useState("");
  const patchLoc = useMutation({
    mutationFn: ({ id, name, region_code }: { id: string; name: string; region_code: string | null }) =>
      apiFetch(`/api/v1/commercial/localities/${id}`, { method: "PATCH", json: { name, region_code } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commercial-loc"] });
      setEditingLocId(null);
      setTgMsg({ ok: true, text: "Guardado com sucesso (localidade)." });
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message || "Falha ao guardar (localidade)." }),
  });
  const delLoc = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/localities/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commercial-loc"] });
      qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      setEditingLocId(null);
    },
  });

  const [lid, setLid] = useState("");
  const [ym, setYm] = useState(seed);
  const [cnt, setCnt] = useState("0");
  const [bulkModalOpen, setBulkModalOpen] = useState(false);
  const [bulkModalMonth, setBulkModalMonth] = useState(seed);
  const [bulkCounts, setBulkCounts] = useState<Record<string, string>>({});
  const createRec = useMutation({
    mutationFn: () =>
      apiFetch<{ upserted?: number }>("/api/v1/commercial/monthly-records/bulk", {
        method: "POST",
        json: { records: [{ locality_id: lid, year_month: ym, client_count: Number(cnt) }] },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      qc.invalidateQueries({ queryKey: ["commercial-agg"] });
      qc.invalidateQueries({ queryKey: ["commercial-cmp"] });
      const locLabel = lid ? locById.get(lid) ?? lid : "—";
      setSingleRecModalOpen(false);
      setTgMsg({
        ok: true,
        text: `Registo guardado: ${locLabel} — ${cnt} cliente(s) em ${formatYearMonthPt(ym)}.`,
      });
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message }),
  });

  const bulkFillAllLocalities = useMutation({
    mutationFn: (records: Array<{ locality_id: string; year_month: string; client_count: number }>) =>
      apiFetch<{ upserted: number }>("/api/v1/commercial/monthly-records/bulk", { method: "POST", json: { records } }),
    onSuccess: (data, records) => {
      const ymShown = records[0]?.year_month ?? "";
      qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      qc.invalidateQueries({ queryKey: ["commercial-agg"] });
      qc.invalidateQueries({ queryKey: ["commercial-cmp"] });
      setBulkModalOpen(false);
      setTgMsg({
        ok: true,
        text: `${data.upserted} localidade(s) actualizada(s) de uma vez (${formatYearMonthPt(ymShown)}).`,
      });
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message }),
  });

  const patchRec = useMutation({
    mutationFn: ({ id, year_month, client_count }: { id: string; year_month: string; client_count: number }) =>
      apiFetch(`/api/v1/commercial/monthly-records/${id}`, { method: "PATCH", json: { year_month, client_count } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      qc.invalidateQueries({ queryKey: ["commercial-agg"] });
      qc.invalidateQueries({ queryKey: ["commercial-cmp"] });
      setRecEditOpen(false);
      setEditRecRow(null);
      setTgMsg({ ok: true, text: "Registo actualizado." });
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message }),
  });

  const delRec = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/commercial/monthly-records/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      qc.invalidateQueries({ queryKey: ["commercial-agg"] });
      qc.invalidateQueries({ queryKey: ["commercial-cmp"] });
      setTgMsg({ ok: true, text: "Registo eliminado." });
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message }),
  });

  const [tgMsg, setTgMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [oltCollectModalOpen, setOltCollectModalOpen] = useState(false);
  const [oltCollectConfirmOpen, setOltCollectConfirmOpen] = useState(false);
  const [oltCollectRunning, setOltCollectRunning] = useState(false);
  const [oltCollectRows, setOltCollectRows] = useState<OltCollectRow[]>([]);
  const [oltSelectedIds, setOltSelectedIds] = useState<string[]>([]);
  const [oltPreSelectedIds, setOltPreSelectedIds] = useState<string[]>([]);
  const [oltPreFilterLocality, setOltPreFilterLocality] = useState("");
  const [oltPreFilterText, setOltPreFilterText] = useState("");
  const [oltCollectLog, setOltCollectLog] = useState<string[]>([]);

  const oltCandidates = useMemo(() => {
    const out: Array<{ id: string; description: string; locality_id: string; locality_name: string; ip?: string | null }> = [];
    for (const d of devices.data?.devices ?? []) {
      if (String(d.category ?? "").trim().toLowerCase() !== "olt" || !d.locality_id) continue;
      const localityId = String(d.locality_id);
      const localityName = locById.get(localityId) ?? localityId;
      out.push({
        id: d.id,
        description: (d.description && String(d.description).trim()) || d.id,
        locality_id: localityId,
        locality_name: localityName,
        ip: d.ip ?? null,
      });
    }
    out.sort((a, b) => a.description.localeCompare(b.description));
    return out;
  }, [devices.data?.devices, locById]);

  const oltCandidatesFiltered = useMemo(() => {
    const q = oltPreFilterText.trim().toLowerCase();
    return oltCandidates.filter((o) => {
      if (oltPreFilterLocality && o.locality_id !== oltPreFilterLocality) return false;
      if (!q) return true;
      return o.description.toLowerCase().includes(q) || o.locality_name.toLowerCase().includes(q) || o.id.toLowerCase().includes(q);
    });
  }, [oltCandidates, oltPreFilterLocality, oltPreFilterText]);

  useEffect(() => {
    if (!oltCollectModalOpen) return;
    if (oltPreSelectedIds.length > 0) return;
    setOltPreSelectedIds(oltCandidates.map((o) => o.id));
  }, [oltCollectModalOpen, oltCandidates, oltPreSelectedIds.length]);

  useEffect(() => {
    if (!newRecMenuOpen) return;
    const close = (e: MouseEvent) => {
      if (newRecMenuRef.current && !newRecMenuRef.current.contains(e.target as Node)) setNewRecMenuOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [newRecMenuOpen]);

  const selectedRows = useMemo(() => oltCollectRows.filter((r) => oltSelectedIds.includes(r.olt_id) && !r.error), [oltCollectRows, oltSelectedIds]);
  const byLocalitySelected = useMemo(() => {
    const m = new Map<string, { locality_id: string; locality_name: string; client_count: number; olt_count: number }>();
    for (const r of selectedRows) {
      const ex = m.get(r.locality_id);
      if (ex) {
        ex.client_count += r.total;
        ex.olt_count += 1;
      } else {
        m.set(r.locality_id, { locality_id: r.locality_id, locality_name: r.locality_name, client_count: r.total, olt_count: 1 });
      }
    }
    return [...m.values()].sort((a, b) => a.locality_name.localeCompare(b.locality_name));
  }, [selectedRows]);
  const sendTg = useMutation({
    mutationFn: () =>
      apiFetch<{ ok?: boolean; sent?: boolean; month?: string }>("/api/v1/commercial/reports/send-telegram", {
        method: "POST",
        json: { month },
      }),
    onSuccess: () =>
      setTgMsg({
        ok: true,
        text: `Relatório comercial enviado por Telegram (${formatYearMonthPt(month)}), com texto formatado pelo servidor.`,
      }),
    onError: (e: Error) =>
      setTgMsg({
        ok: false,
        text: `Envio Telegram falhou: ${e.message}`,
      }),
  });

  async function exportCsv() {
    const headers = new Headers();
    const k = getStoredApiKey();
    if (k) headers.set("X-API-Key", k);
    const res = await fetch(apiUrl(`/api/v1/commercial/reports/export?format=csv&month=${encodeURIComponent(month)}`), { headers });
    if (!res.ok) throw new Error(await res.text());
    downloadBlob(`commercial_${month}.csv`, await res.blob());
  }

  async function runOltCollection(targetOltIds: string[]) {
    setOltCollectRunning(true);
    setOltCollectRows([]);
    setOltSelectedIds([]);
    setOltCollectLog([]);
    try {
      const locMap = new Map<string, string>();
      for (const l of locs.data?.localities ?? []) {
        locMap.set(l.id, l.name);
      }
      const targetSet = new Set(targetOltIds);
      const olts = (devices.data?.devices ?? []).filter(
        (d) => String(d.category ?? "").trim().toLowerCase() === "olt" && !!d.locality_id && targetSet.has(d.id),
      );
      if (olts.length === 0) {
        setOltCollectLog(["Nenhuma OLT selecionada para coletar."]);
        return;
      }
      const out: OltCollectRow[] = [];
      for (const d of olts) {
        const localityId = String(d.locality_id);
        const localityName = locMap.get(localityId) ?? localityId;
        const label = (d.description && String(d.description).trim()) || d.id;
        setOltCollectLog((m) => [...m, `Coletando ${label}…`]);
        try {
          const snap = await apiFetch<Record<string, unknown>>(`/api/v1/olt/devices/${d.id}/refresh`, { method: "POST", json: {} });
          const totals = computeOnuTotalsFromSnapshot(snap);
          out.push({
            olt_id: d.id,
            olt_description: label,
            locality_id: localityId,
            locality_name: localityName,
            online: totals.online,
            offline: totals.offline,
            total: totals.total,
          });
          setOltCollectLog((m) => [...m, `${label}: total ${totals.total} (${totals.online} on / ${totals.offline} off)`]);
        } catch (e) {
          out.push({
            olt_id: d.id,
            olt_description: label,
            locality_id: localityId,
            locality_name: localityName,
            online: 0,
            offline: 0,
            total: 0,
            error: (e as Error).message || String(e),
          });
          setOltCollectLog((m) => [...m, `${label}: erro de coleta`]);
        }
      }
      setOltCollectRows(out);
      setOltSelectedIds(out.filter((r) => !r.error).map((r) => r.olt_id));
      setOltCollectLog((m) => [...m, "Coleta finalizada. Revise e confirme a gravação."]);
    } finally {
      setOltCollectRunning(false);
    }
  }

  const saveCollected = useMutation({
    mutationFn: async () =>
      apiFetch<{ upserted: number }>("/api/v1/commercial/monthly-records/bulk", {
        method: "POST",
        json: { records: byLocalitySelected.map((r) => ({ locality_id: r.locality_id, year_month: month, client_count: r.client_count })) },
      }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["commercial-rec"] });
      qc.invalidateQueries({ queryKey: ["commercial-agg"] });
      qc.invalidateQueries({ queryKey: ["commercial-cmp"] });
      setTgMsg({
        ok: true,
        text: `Registos comerciais actualizados (${data.upserted} localidade(s)) para ${formatYearMonthPt(month)}.`,
      });
      setOltCollectConfirmOpen(false);
      setOltCollectModalOpen(false);
    },
    onError: (e: Error) => setTgMsg({ ok: false, text: e.message }),
  });

  const seedBulkCountsForMonth = (m: string) => {
    const out: Record<string, string> = {};
    const allRecs = recs.data?.records ?? [];
    for (const loc of locs.data?.localities ?? []) {
      const hit = allRecs.find((r) => r.locality_id === loc.id && r.year_month === m);
      out[loc.id] = hit !== undefined ? String(hit.client_count) : "";
    }
    setBulkCounts(out);
  };

  function openCommercialBulkModal() {
    const localities = locs.data?.localities ?? [];
    if (localities.length === 0) {
      setTgMsg({ ok: false, text: "Não há localidades para preencher." });
      return;
    }
    const initialM = /^\d{4}-\d{2}$/.test(month) ? month : seed;
    if (!/^\d{4}-\d{2}$/.test(initialM)) {
      setTgMsg({ ok: false, text: "Escolha um mês de referência válido no separador Resumo (agregados)." });
      return;
    }
    setBulkModalMonth(initialM);
    seedBulkCountsForMonth(initialM);
    setBulkModalOpen(true);
  }

  function submitCommercialBulkModal() {
    const localities = locs.data?.localities ?? [];
    if (localities.length === 0) return;
    if (!/^\d{4}-\d{2}$/.test(bulkModalMonth)) {
      setTgMsg({ ok: false, text: "Escolha um mês de referência no modal." });
      return;
    }
    const payloads: Array<{ locality_id: string; year_month: string; client_count: number }> = [];
    for (const loc of localities) {
      const raw = (bulkCounts[loc.id] ?? "").trim();
      const v = raw === "" ? 0 : Number(raw);
      if (!Number.isFinite(v) || v < 0 || !Number.isInteger(v)) {
        setTgMsg({ ok: false, text: `Quantidade inválida para «${loc.name}»: use número inteiro ≥ 0 ou deixe vazio (= 0).` });
        return;
      }
      payloads.push({ locality_id: loc.id, year_month: bulkModalMonth, client_count: v });
    }

    const allRecs = recs.data?.records ?? [];
    const replacements: { name: string; was: number; will: number }[] = [];
    for (const loc of localities) {
      const hit = allRecs.find((r) => r.locality_id === loc.id && r.year_month === bulkModalMonth);
      const nv = payloads.find((p) => p.locality_id === loc.id)?.client_count ?? 0;
      if (hit && hit.client_count !== nv) {
        replacements.push({ name: loc.name, was: hit.client_count, will: nv });
      }
    }

    if (replacements.length > 0) {
      const lines = replacements.slice(0, 14).map((x) => `• ${x.name}: ${x.was} → ${x.will} cliente(s)`);
      const more = replacements.length > 14 ? `\n… e mais ${replacements.length - 14} alteração(ões).` : "";
      const ok = confirm(
        `Existem ${replacements.length} localidade(s) com valores diferentes em ${formatYearMonthPt(bulkModalMonth)}. ` +
          `Os números actuais serão substituídos pelos que indicou:\n\n${lines.join("\n")}${more}\n\nGuardar todas?`,
      );
      if (!ok) return;
    }

    bulkFillAllLocalities.mutate(payloads);
  }

  if (locs.isLoading || recs.isLoading || devices.isLoading) return <p>Carregando…</p>;
  if (locs.isError) return <div className="msg msg--err">{(locs.error as Error).message}</div>;
  if (recs.isError) return <div className="msg msg--err">{(recs.error as Error).message}</div>;
  if (devices.isError) return <div className="msg msg--err">{(devices.error as Error).message}</div>;

  return (
    <>
      <div className="page-heading">
        <h1>Base comercial</h1>
        <PageCountPill label="Registros comerciais" count={(recs.data?.records ?? []).length} />
      </div>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>Localidades, registos mensais por localidade e totais agregados (opcionalmente filtrados por mês).</p>
      {tgMsg && (
        <div className={`page-toast ${tgMsg.ok ? "page-toast--ok" : "page-toast--err"}`} role="alert">
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setTgMsg(null)}>
            ×
          </button>
          {tgMsg.text}
        </div>
      )}
      <div className="tabs" style={{ flexWrap: "wrap", marginBottom: 16 }}>
        <button type="button" className={mainTab === "resumo" ? "active" : ""} onClick={() => setMainTab("resumo")}>
          Resumo
        </button>
        <button type="button" className={mainTab === "localidades" ? "active" : ""} onClick={() => setMainTab("localidades")}>
          Localidades
        </button>
        <button type="button" className={mainTab === "registros" ? "active" : ""} onClick={() => setMainTab("registros")}>
          Registros
        </button>
      </div>
      {mainTab === "resumo" && (
        <>
          {canMutate ? (
            <div className="row" style={{ marginBottom: 12, alignItems: "flex-start", gap: 10, flexWrap: "wrap" }}>
              <button
                type="button"
                className="btn btn--primary"
                onClick={() => {
                  setOltCollectModalOpen(true);
                  setOltCollectConfirmOpen(false);
                  setOltCollectRows([]);
                  setOltSelectedIds([]);
                  setOltCollectLog([]);
                  setOltPreFilterText("");
                  setOltPreFilterLocality("");
                  setOltPreSelectedIds(oltCandidates.map((o) => o.id));
                }}
              >
                Coletar ONUs das OLTs por localidade
              </button>
              <div ref={newRecMenuRef} style={{ position: "relative" }}>
                <button
                  type="button"
                  className="btn"
                  aria-haspopup="menu"
                  aria-expanded={newRecMenuOpen}
                  onClick={() => setNewRecMenuOpen((o) => !o)}
                >
                  Novo Registro ▾
                </button>
                {newRecMenuOpen && (
                  <div
                    role="menu"
                    style={{
                      position: "absolute",
                      top: "100%",
                      left: 0,
                      marginTop: 4,
                      zIndex: 50,
                      minWidth: 260,
                      background: "var(--panel)",
                      border: "1px solid var(--border)",
                      borderRadius: "var(--radius)",
                      boxShadow: "0 8px 24px rgba(0,0,0,0.35)",
                      padding: 6,
                      display: "flex",
                      flexDirection: "column",
                      gap: 4,
                    }}
                  >
                    <button
                      type="button"
                      role="menuitem"
                      className="btn"
                      style={{ width: "100%", justifyContent: "flex-start" }}
                      onClick={() => {
                        setNewRecMenuOpen(false);
                        setLid("");
                        setYm(/^\d{4}-\d{2}$/.test(month) ? month : seed);
                        setCnt("0");
                        setSingleRecModalOpen(true);
                      }}
                    >
                      Novo registro individual
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className="btn"
                      style={{ width: "100%", justifyContent: "flex-start" }}
                      onClick={() => {
                        setNewRecMenuOpen(false);
                        openCommercialBulkModal();
                      }}
                    >
                      Novo registro em massa
                    </button>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <p style={{ color: "var(--muted)", fontSize: 12, marginBottom: 12 }}>Modo só leitura: consulte agregados e exportações; criação de registos e coletas OLT são reservadas a administradores.</p>
          )}
      {oltCollectModalOpen && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !oltCollectRunning && !saveCollected.isPending && setOltCollectModalOpen(false)}>
          <div
            className="card"
            role="dialog"
            aria-modal="true"
            aria-labelledby="commercial-olt-collect-title"
            style={{ maxWidth: 960, width: "100%", margin: "6vh auto", maxHeight: "86vh", overflow: "auto" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <h2 id="commercial-olt-collect-title" style={{ marginTop: 0 }}>
              Coleta ONUs por OLT
            </h2>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              O sistema executa o snapshot SNMP de cada OLT vinculada a uma localidade, mostra online/offline/total por OLT e permite selecionar quais coletas entram no total final de cada localidade.
            </p>
            <div className="card" style={{ padding: 10 }}>
              <h3 style={{ marginTop: 0, marginBottom: 8 }}>Filtro e seleção de OLTs para coletar</h3>
              <div className="row" style={{ gap: 8, marginBottom: 8 }}>
                <input
                  className="input"
                  placeholder="Filtrar por OLT / localidade / ID"
                  value={oltPreFilterText}
                  onChange={(e) => setOltPreFilterText(e.target.value)}
                  style={{ minWidth: 260 }}
                />
                <select className="select" value={oltPreFilterLocality} onChange={(e) => setOltPreFilterLocality(e.target.value)}>
                  <option value="">Todas as localidades</option>
                  {(locs.data?.localities ?? []).map((l) => (
                    <option key={l.id} value={l.id}>
                      {l.name}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  className="btn"
                  onClick={() =>
                    setOltPreSelectedIds((prev) => Array.from(new Set([...prev, ...oltCandidatesFiltered.map((o) => o.id)])))
                  }
                >
                  Marcar filtradas
                </button>
                <button
                  type="button"
                  className="btn"
                  onClick={() => setOltPreSelectedIds((prev) => prev.filter((id) => !oltCandidatesFiltered.some((o) => o.id === id)))}
                >
                  Desmarcar filtradas
                </button>
              </div>
              <div className="table-wrap" style={{ maxHeight: 180, overflow: "auto" }}>
                <table style={{ fontSize: 11 }}>
                  <thead>
                    <tr>
                      <th />
                      <th>OLT</th>
                      <th>Localidade</th>
                      <th>IP</th>
                    </tr>
                  </thead>
                  <tbody>
                    {oltCandidatesFiltered.map((o) => (
                      <tr key={`pre-${o.id}`}>
                        <td>
                          <input
                            type="checkbox"
                            checked={oltPreSelectedIds.includes(o.id)}
                            disabled={oltCollectRunning || saveCollected.isPending}
                            onChange={(e) =>
                              setOltPreSelectedIds((prev) =>
                                e.target.checked ? Array.from(new Set([...prev, o.id])) : prev.filter((id) => id !== o.id),
                              )
                            }
                          />
                        </td>
                        <td>
                          {o.description}
                          <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                            {o.id}
                          </div>
                        </td>
                        <td>{o.locality_name}</td>
                        <td className="mono">{o.ip ?? "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <p style={{ fontSize: 11, color: "var(--muted)", marginBottom: 0 }}>
                Selecionadas para coleta: <span className="mono">{oltPreSelectedIds.length}</span> de <span className="mono">{oltCandidates.length}</span>
              </p>
            </div>
            <div className="row" style={{ gap: 8, marginBottom: 8 }}>
              <button
                type="button"
                className="btn btn--primary"
                disabled={oltCollectRunning || saveCollected.isPending}
                onClick={() => {
                  if (oltPreSelectedIds.length === 0) {
                    setTgMsg({ ok: false, text: "Selecione ao menos 1 OLT antes de iniciar a coleta." });
                    return;
                  }
                  if (!confirm(`Iniciar coleta de ONUs em ${oltPreSelectedIds.length} OLT(s) selecionada(s)?`)) return;
                  void runOltCollection(oltPreSelectedIds);
                }}
              >
                {oltCollectRunning ? "Coletando…" : "Iniciar coleta"}
              </button>
              <button
                type="button"
                className="btn"
                disabled={oltCollectRunning || saveCollected.isPending || oltCollectRows.length === 0}
                onClick={() => setOltCollectConfirmOpen(true)}
              >
                Salvar selecionadas em {formatYearMonthPt(month)}
              </button>
              <button type="button" className="btn" disabled={oltCollectRunning || saveCollected.isPending} onClick={() => setOltCollectModalOpen(false)}>
                Fechar
              </button>
            </div>
            {oltCollectRows.length > 0 && (
              <div className="row" style={{ gap: 8, marginBottom: 8 }}>
                <button
                  type="button"
                  className="btn"
                  onClick={() => setOltSelectedIds(oltCollectRows.filter((r) => !r.error).map((r) => r.olt_id))}
                >
                  Marcar todas
                </button>
                <button type="button" className="btn" onClick={() => setOltSelectedIds([])}>
                  Desmarcar todas
                </button>
              </div>
            )}
            {oltCollectRows.length > 0 && (
              <div className="table-wrap" style={{ maxHeight: 320, overflow: "auto" }}>
                <table style={{ fontSize: 11 }}>
                  <thead>
                    <tr>
                      <th />
                      <th>OLT</th>
                      <th>Localidade</th>
                      <th className="mono">Online</th>
                      <th className="mono">Offline</th>
                      <th className="mono">Total</th>
                      <th>Resultado</th>
                    </tr>
                  </thead>
                  <tbody>
                    {oltCollectRows.map((r) => (
                      <tr key={r.olt_id}>
                        <td>
                          <input
                            type="checkbox"
                            checked={oltSelectedIds.includes(r.olt_id)}
                            disabled={!!r.error || oltCollectRunning || saveCollected.isPending}
                            onChange={(e) =>
                              setOltSelectedIds((prev) =>
                                e.target.checked ? Array.from(new Set([...prev, r.olt_id])) : prev.filter((id) => id !== r.olt_id),
                              )
                            }
                          />
                        </td>
                        <td>
                          {r.olt_description}
                          <div className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                            {r.olt_id}
                          </div>
                        </td>
                        <td>{r.locality_name}</td>
                        <td className="mono">{r.online}</td>
                        <td className="mono">{r.offline}</td>
                        <td className="mono">{r.total}</td>
                        <td>{r.error ? <span className="badge badge--err">erro</span> : <span className="badge badge--ok">ok</span>}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {byLocalitySelected.length > 0 && (
              <>
                <h3 style={{ marginTop: 12 }}>Prévia por localidade (selecionadas)</h3>
                <div className="table-wrap">
                  <table style={{ fontSize: 11 }}>
                    <thead>
                      <tr>
                        <th>Localidade</th>
                        <th className="mono">OLT(s)</th>
                        <th className="mono">Clientes (online+offline)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {byLocalitySelected.map((r) => (
                        <tr key={r.locality_id}>
                          <td>{r.locality_name}</td>
                          <td className="mono">{r.olt_count}</td>
                          <td className="mono">{r.client_count}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            )}
            {oltCollectLog.length > 0 && (
              <pre className="mono" style={{ background: "var(--panel2)", borderRadius: "var(--radius)", padding: 8, maxHeight: 160, overflow: "auto", fontSize: 11 }}>
                {oltCollectLog.join("\n")}
              </pre>
            )}
            {oltCollectConfirmOpen && (
              <div className="card" style={{ borderColor: "var(--warn)", marginTop: 8 }}>
                <h3 style={{ marginTop: 0 }}>Confirmar gravação em {formatYearMonthPt(month)}</h3>
                <p style={{ fontSize: 12, color: "var(--muted)" }}>
                  Serão gravados/atualizados {byLocalitySelected.length} registros de localidades com a soma das OLTs marcadas.
                </p>
                <div className="row" style={{ gap: 8 }}>
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={saveCollected.isPending || byLocalitySelected.length === 0}
                    onClick={() => {
                      if (!confirm(`Salvar ${byLocalitySelected.length} localidade(s) com base nas coletas selecionadas?`)) return;
                      saveCollected.mutate();
                    }}
                  >
                    {saveCollected.isPending ? "Salvando…" : "Confirmar e salvar"}
                  </button>
                  <button type="button" className="btn" disabled={saveCollected.isPending} onClick={() => setOltCollectConfirmOpen(false)}>
                    Cancelar
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="card">
        <h2>Agregados</h2>
        <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "flex-end" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 200 }}>
            <label htmlFor="commercial-agg-month" style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
              Mês de referência
            </label>
            <select id="commercial-agg-month" className="select" value={month} onChange={(e) => setMonth(e.target.value)}>
              {monthSelectChoicesWithFallback(monthChoices, month).map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>
          <button
            type="button"
            className="btn"
            disabled={agg.isFetching}
            onClick={() => {
              void agg.refetch().then((r) => {
                if (r.error) setTgMsg({ ok: false, text: (r.error as Error).message });
                else setTgMsg({ ok: true, text: `Totais actualizados (${formatYearMonthPt(month)}).` });
              });
            }}
          >
            {agg.isFetching ? "A actualizar…" : "Atualizar totais"}
          </button>
          <button type="button" className="btn" onClick={() => exportCsv().catch((e) => setTgMsg({ ok: false, text: String(e) }))}>
            Export CSV
          </button>
          {canMutate ? (
            <button type="button" className="btn" disabled={sendTg.isPending} onClick={() => setTgSendConfirmOpen(true)}>
              Telegram relatório
            </button>
          ) : null}
        </div>
        {agg.isError && <div className="msg msg--err">{(agg.error as Error).message}</div>}
        {agg.data && (
          <p style={{ marginTop: 12 }}>
            <strong>Total de clientes</strong>{" "}
            <span className="mono" style={{ fontSize: 18 }}>
              {agg.data.total_clients}
            </span>{" "}
            {agg.data.month ? (
              <>
                em <strong>{formatYearMonthPt(agg.data.month)}</strong>
              </>
            ) : (
              <span style={{ color: "var(--muted)" }}>(sem filtro de mês no servidor — month vazio na resposta)</span>
            )}
          </p>
        )}
      </div>

      <div className="card">
        <h2>Comparativo mês atual vs anterior</h2>
        <p style={{ color: "var(--muted)", fontSize: 12 }}>
          Comparação para <strong>{formatYearMonthPt(month)}</strong> contra o mês anterior. Variações por localidade em clientes e em percentagem.
        </p>
        {cmp.isError && <div className="msg msg--err">{(cmp.error as Error).message}</div>}
        <div className="table-wrap">
          <table style={{ fontSize: 11 }}>
            <thead>
              <tr>
                <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onCmpSortClick("locality")} title="Ordenar">
                  Localidade{cmpSortMark("locality")}
                </th>
                <th
                  style={{ cursor: "pointer", userSelect: "none" }}
                  onClick={() => onCmpSortClick("previous")}
                  title="Ordenar"
                >
                  {cmp.data?.previous_month ? formatYearMonthPt(cmp.data.previous_month) : "Mês anterior"}
                  {cmpSortMark("previous")}
                </th>
                <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onCmpSortClick("current")} title="Ordenar">
                  {cmp.data?.month ? formatYearMonthPt(cmp.data.month) : formatYearMonthPt(month)}
                  {cmpSortMark("current")}
                </th>
                <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onCmpSortClick("delta")} title="Ordenar">
                  Variação (abs.){cmpSortMark("delta")}
                </th>
                <th style={{ cursor: "pointer", userSelect: "none" }} onClick={() => onCmpSortClick("delta_pct")} title="Ordenar">
                  Variação (%){cmpSortMark("delta_pct")}
                </th>
              </tr>
            </thead>
            <tbody>
              {cmpSortedRows.map((r) => (
                <tr key={r.locality_id}>
                  <td>{r.locality_name}</td>
                  <td className="mono">{r.previous}</td>
                  <td className="mono">{r.current}</td>
                  <td className="mono">{r.delta >= 0 ? `+${r.delta}` : `${r.delta}`}</td>
                  <td className="mono" style={{ color: r.delta > 0 ? "var(--ok)" : r.delta < 0 ? "var(--err)" : "var(--muted)" }}>
                    {Number.isFinite(Number(r.delta_percent)) ? `${r.delta_percent >= 0 ? "+" : ""}${r.delta_percent.toFixed(1)}%` : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="card">
        <h2>Clientes por localidade (mês seleccionado)</h2>
        <p style={{ color: "var(--muted)", fontSize: 12 }}>
          Soma de clientes nos registos de <strong>{formatYearMonthPt(month)}</strong>
          {chartRows.sum > 0 ? (
            <>
              {" "}
              · total no mês: <strong>{chartRows.sum}</strong>
            </>
          ) : null}
          . A percentagem mostra a fatia em relação a esse total no mês.
        </p>
        {chartRows.rows.length === 0 ? (
          <p style={{ color: "var(--muted)" }}>Sem dados para {formatYearMonthPt(month)}.</p>
        ) : (
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {chartRows.rows.map((r) => (
              <li key={r.lid} style={{ marginBottom: 10 }}>
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: 13, marginBottom: 4, gap: 8 }}>
                  <span>{r.label}</span>
                  <span className="mono" style={{ textAlign: "right" }}>
                    {r.count}
                    {chartRows.sum > 0 ? (
                      <span style={{ color: "var(--muted)", marginLeft: 8 }}>
                        ({((r.count / chartRows.sum) * 100).toFixed(1)}% do total)
                      </span>
                    ) : null}
                  </span>
                </div>
                <div style={{ height: 8, background: "var(--panel2)", borderRadius: 4, overflow: "hidden" }}>
                  <div
                    style={{
                      width: `${(r.count / chartRows.max) * 100}%`,
                      height: "100%",
                      background: "var(--accent)",
                      borderRadius: 4,
                    }}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
        </>
      )}

      {mainTab === "localidades" && (
        <div className="card">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, flexWrap: "wrap", marginBottom: 10 }}>
            <h2 style={{ margin: 0 }}>Localidades cadastradas</h2>
            {canMutate ? (
              <button
                type="button"
                className="btn btn--primary"
                style={{ width: 40, height: 40, padding: 0, fontSize: 22, lineHeight: 1, borderRadius: "var(--radius, 8px)", flexShrink: 0 }}
                title="Nova localidade"
                aria-label="Adicionar nova localidade"
                onClick={() => {
                  setLocName("");
                  setLocRc("");
                  setLocModalOpen(true);
                }}
              >
                +
              </button>
            ) : null}
          </div>
          <p style={{ color: "var(--muted)", fontSize: 12 }}>
            Editar nome ou código de região; excluir remove a localidade (registos mensais associados podem falhar se a API restringir).
          </p>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Nome</th>
                  <th>Código região</th>
                  <th>Criada em</th>
                  <th style={{ width: 220 }}>Acções</th>
                </tr>
              </thead>
              <tbody>
                {(locs.data?.localities ?? []).map((l) => (
                  <tr key={l.id}>
                    {editingLocId === l.id ? (
                      <>
                        <td>
                          <input className="input" style={{ width: "100%" }} value={editLocName} onChange={(e) => setEditLocName(e.target.value)} />
                        </td>
                        <td>
                          <input className="input mono" style={{ width: "100%" }} value={editLocRc} onChange={(e) => setEditLocRc(e.target.value)} />
                        </td>
                        <td className="mono" style={{ fontSize: 11 }}>
                          {l.created_at ? formatAlertDateTimePt(String(l.created_at)) : "—"}
                        </td>
                        <td>
                          {canMutate ? (
                            <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                              <button
                                type="button"
                                className="btn btn--primary"
                                disabled={!editLocName.trim() || patchLoc.isPending}
                                onClick={() => patchLoc.mutate({ id: l.id, name: editLocName.trim(), region_code: editLocRc.trim() || null })}
                              >
                                Guardar
                              </button>
                              <button type="button" className="btn" onClick={() => setEditingLocId(null)}>
                                Cancelar
                              </button>
                            </div>
                          ) : (
                            "—"
                          )}
                        </td>
                      </>
                    ) : (
                      <>
                        <td>{l.name}</td>
                        <td className="mono">{l.region_code ?? "—"}</td>
                        <td className="mono" style={{ fontSize: 11 }}>
                          {l.created_at ? formatAlertDateTimePt(String(l.created_at)) : "—"}
                        </td>
                        <td>
                          {canMutate ? (
                            <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                              <button
                                type="button"
                                className="btn"
                                onClick={() => {
                                  setEditingLocId(l.id);
                                  setEditLocName(l.name);
                                  setEditLocRc(l.region_code ?? "");
                                }}
                              >
                                Editar
                              </button>
                              <button
                                type="button"
                                className="btn"
                                disabled={delLoc.isPending}
                                onClick={() => {
                                  if (confirm(`Excluir localidade «${l.name}»?`)) delLoc.mutate(l.id);
                                }}
                              >
                                Excluir
                              </button>
                            </div>
                          ) : (
                            <span style={{ color: "var(--muted)" }}>—</span>
                          )}
                        </td>
                      </>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {(locs.data?.localities ?? []).length === 0 && <p style={{ color: "var(--muted)" }}>Ainda não há localidades.</p>}
          {patchLoc.isError && <div className="msg msg--err">{(patchLoc.error as Error).message}</div>}
          {delLoc.isError && <div className="msg msg--err">{(delLoc.error as Error).message}</div>}
        </div>
      )}

      {mainTab === "registros" && (
        <div className="card">
          <h2 style={{ marginTop: 0 }}>Registros mensais</h2>
          <p style={{ color: "var(--muted)", fontSize: 12 }}>
            Lista dos últimos registos na base (até 500). Use Editar para alterar mês ou quantidade; a localidade não pode ser mudada aqui.
          </p>
          {recsSorted.length === 0 ? (
            <p style={{ color: "var(--muted)", margin: 0 }}>Sem registos na base.</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Localidade</th>
                    <th>Mês</th>
                    <th>Clientes</th>
                    <th>Criado</th>
                    <th style={{ width: 200 }}>Acções</th>
                  </tr>
                </thead>
                <tbody>
                  {recsSorted.map((r) => (
                    <tr key={r.id}>
                      <td>{locById.get(r.locality_id) ?? <span className="mono">{r.locality_id}</span>}</td>
                      <td>{formatYearMonthPt(r.year_month)}</td>
                      <td className="mono">{r.client_count}</td>
                      <td className="mono" style={{ fontSize: 11 }}>
                        {r.created_at ? formatAlertDateTimePt(String(r.created_at)) : "—"}
                      </td>
                      <td>
                        {canMutate ? (
                          <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
                            <button
                              type="button"
                              className="btn"
                              onClick={() => {
                                setEditRecRow(r);
                                setRecEditYm(r.year_month);
                                setRecEditCnt(String(r.client_count));
                                setRecEditOpen(true);
                              }}
                            >
                              Editar
                            </button>
                            <button
                              type="button"
                              className="btn"
                              disabled={delRec.isPending}
                              onClick={() => {
                                const locLabel = locById.get(r.locality_id) ?? r.locality_id;
                                if (confirm(`Eliminar registo de ${locLabel} em ${formatYearMonthPt(r.year_month)} (${r.client_count} clientes)?`)) {
                                  delRec.mutate(r.id);
                                }
                              }}
                            >
                              Eliminar
                            </button>
                          </div>
                        ) : (
                          <span style={{ color: "var(--muted)" }}>—</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {bulkModalOpen && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !bulkFillAllLocalities.isPending && setBulkModalOpen(false)}>
          <div
            className="card"
            role="dialog"
            aria-modal="true"
            aria-labelledby="commercial-bulk-edit-title"
            style={{ maxWidth: 560, width: "100%", margin: "6vh auto", maxHeight: "88vh", overflow: "hidden", display: "flex", flexDirection: "column" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <h2 id="commercial-bulk-edit-title" style={{ marginTop: 0 }}>
              Quantidades por localidade
            </h2>
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
              Mês aplicado a todas as linhas ao guardar. Ao mudar o mês, os campos recarregam com os valores já gravados nesse período (edições não guardadas são
              descartadas).
            </p>
            <div style={{ marginBottom: 12 }}>
              <label htmlFor="commercial-bulk-modal-month" style={{ fontSize: 11, color: "var(--muted)", display: "block", marginBottom: 4 }}>
                Mês de referência
              </label>
              <select
                id="commercial-bulk-modal-month"
                className="select"
                style={{ width: "100%", maxWidth: 320 }}
                value={bulkModalMonth}
                onChange={(e) => {
                  const m = e.target.value;
                  setBulkModalMonth(m);
                  seedBulkCountsForMonth(m);
                }}
                disabled={bulkFillAllLocalities.isPending}
              >
                {monthSelectChoicesWithFallback(monthChoices, bulkModalMonth).map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="table-wrap" style={{ flex: 1, overflow: "auto", maxHeight: "min(52vh, 480px)", border: "1px solid var(--border)", borderRadius: "var(--radius)" }}>
              <table style={{ fontSize: 12, width: "100%" }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: "left" }}>Localidade</th>
                    <th style={{ width: 120 }}>Quantidade</th>
                  </tr>
                </thead>
                <tbody>
                  {[...(locs.data?.localities ?? [])]
                    .sort((a, b) => a.name.localeCompare(b.name))
                    .map((loc) => (
                      <tr key={loc.id}>
                        <td>{loc.name}</td>
                        <td>
                          <input
                            className="input mono"
                            style={{ width: "100%", minWidth: 88 }}
                            type="number"
                            min={0}
                            step={1}
                            inputMode="numeric"
                            aria-label={`Clientes para ${loc.name}`}
                            value={bulkCounts[loc.id] ?? ""}
                            onChange={(e) => setBulkCounts((prev) => ({ ...prev, [loc.id]: e.target.value }))}
                            disabled={bulkFillAllLocalities.isPending}
                          />
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
            <div className="row" style={{ gap: 8, marginTop: 14, flexWrap: "wrap", justifyContent: "flex-end" }}>
              <button type="button" className="btn" disabled={bulkFillAllLocalities.isPending} onClick={() => setBulkModalOpen(false)}>
                Cancelar
              </button>
              <button type="button" className="btn btn--primary" disabled={bulkFillAllLocalities.isPending} onClick={() => submitCommercialBulkModal()}>
                {bulkFillAllLocalities.isPending ? "A guardar…" : "Guardar todas"}
              </button>
            </div>
          </div>
        </div>
      )}

      {singleRecModalOpen && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !createRec.isPending && setSingleRecModalOpen(false)}>
          <div
            className="card"
            role="dialog"
            aria-modal="true"
            aria-labelledby="commercial-new-single-rec-title"
            style={{ maxWidth: 520, width: "100%", margin: "10vh auto" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <h2 id="commercial-new-single-rec-title" style={{ marginTop: 0 }}>
              Novo registro individual
            </h2>
            <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
              Um registo por localidade e mês. Guardar substitui o valor existente para essa combinação.
            </p>
            <div className="row" style={{ flexWrap: "wrap", gap: 8, alignItems: "flex-end" }}>
              <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 180, flex: "1 1 160px" }}>
                <label htmlFor="commercial-locality-single" style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
                  Localidade
                </label>
                <select id="commercial-locality-single" className="select" style={{ width: "100%" }} value={lid} onChange={(e) => setLid(e.target.value)}>
                  <option value="">Escolher localidade…</option>
                  {(locs.data?.localities ?? []).map((l) => (
                    <option key={l.id} value={l.id}>
                      {l.name}
                    </option>
                  ))}
                </select>
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 200, flex: "1 1 180px" }}>
                <label htmlFor="commercial-ym-select" style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
                  Mês de referência
                </label>
                <select id="commercial-ym-select" className="select" style={{ width: "100%" }} value={ym} onChange={(e) => setYm(e.target.value)}>
                  {monthSelectChoicesWithFallback(monthChoices, ym).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 120, flex: "1 1 100px" }}>
                <label htmlFor="commercial-client-count" style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
                  Quantidade de clientes
                </label>
                <input
                  id="commercial-client-count"
                  className="input"
                  type="number"
                  min={0}
                  placeholder="0"
                  value={cnt}
                  onChange={(e) => setCnt(e.target.value)}
                />
              </div>
            </div>
            <div className="row" style={{ gap: 8, marginTop: 14, flexWrap: "wrap" }}>
              <button
                type="button"
                className="btn btn--primary"
                disabled={!lid || createRec.isPending}
                onClick={() => {
                  const n = Number(cnt);
                  if (!Number.isFinite(n) || n < 0 || !Number.isInteger(n)) {
                    setTgMsg({ ok: false, text: "Use um número inteiro ≥ 0 para clientes." });
                    return;
                  }
                  createRec.mutate();
                }}
              >
                {createRec.isPending ? "A guardar…" : "Guardar registo"}
              </button>
              <button type="button" className="btn" disabled={createRec.isPending} onClick={() => setSingleRecModalOpen(false)}>
                Cancelar
              </button>
            </div>
            {createRec.isError && (
              <div className="msg msg--err" style={{ marginTop: 12 }}>
                {(createRec.error as Error).message}
              </div>
            )}
          </div>
        </div>
      )}

      {recEditOpen && editRecRow && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !patchRec.isPending && setRecEditOpen(false)}>
          <div
            className="card"
            role="dialog"
            aria-modal="true"
            aria-labelledby="commercial-edit-rec-title"
            style={{ maxWidth: 480, width: "100%", margin: "10vh auto" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <h2 id="commercial-edit-rec-title" style={{ marginTop: 0 }}>
              Editar registo
            </h2>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              Localidade: <strong>{locById.get(editRecRow.locality_id) ?? editRecRow.locality_id}</strong>
            </p>
            <div className="field">
              <label htmlFor="commercial-rec-edit-ym">Mês de referência</label>
              <select id="commercial-rec-edit-ym" className="select" style={{ width: "100%" }} value={recEditYm} onChange={(e) => setRecEditYm(e.target.value)}>
                {monthSelectChoicesWithFallback(monthChoices, recEditYm).map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="commercial-rec-edit-cnt">Quantidade de clientes</label>
              <input
                id="commercial-rec-edit-cnt"
                className="input"
                type="number"
                min={0}
                step={1}
                value={recEditCnt}
                onChange={(e) => setRecEditCnt(e.target.value)}
              />
            </div>
            <div className="row" style={{ gap: 8, marginTop: 14, flexWrap: "wrap" }}>
              <button
                type="button"
                className="btn btn--primary"
                disabled={patchRec.isPending || !/^\d{4}-\d{2}$/.test(recEditYm)}
                onClick={() => {
                  const n = Number(recEditCnt);
                  if (!Number.isFinite(n) || n < 0 || !Number.isInteger(n)) {
                    setTgMsg({ ok: false, text: "Use um número inteiro ≥ 0 para clientes." });
                    return;
                  }
                  patchRec.mutate({ id: editRecRow.id, year_month: recEditYm, client_count: n });
                }}
              >
                {patchRec.isPending ? "A guardar…" : "Guardar alterações"}
              </button>
              <button type="button" className="btn" disabled={patchRec.isPending} onClick={() => { setRecEditOpen(false); setEditRecRow(null); }}>
                Cancelar
              </button>
            </div>
            {patchRec.isError && <div className="msg msg--err" style={{ marginTop: 12 }}>{(patchRec.error as Error).message}</div>}
          </div>
        </div>
      )}

      {locModalOpen && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !createLoc.isPending && setLocModalOpen(false)}>
          <div
            className="card"
            role="dialog"
            aria-modal="true"
            aria-labelledby="commercial-new-locality-title"
            style={{ maxWidth: 480, width: "100%", margin: "12vh auto" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <h2 id="commercial-new-locality-title" style={{ marginTop: 0 }}>
              Nova localidade
            </h2>
            <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
              Identificação utilizada nas OLT e nos registos mensais da base comercial.
            </p>
            <div className="field">
              <label>Nome</label>
              <input className="input" placeholder="Nome *" autoFocus value={locName} onChange={(e) => setLocName(e.target.value)} />
            </div>
            <div className="field">
              <label>Código da região (opcional)</label>
              <input className="input" placeholder="Ex.: RJ, SP…" value={locRc} onChange={(e) => setLocRc(e.target.value)} />
            </div>
            <div className="row" style={{ gap: 8, marginTop: 12 }}>
              <button type="button" className="btn btn--primary" disabled={!locName.trim() || createLoc.isPending} onClick={() => createLoc.mutate()}>
                {createLoc.isPending ? "A guardar…" : "Guardar localidade"}
              </button>
              <button type="button" className="btn" disabled={createLoc.isPending} onClick={() => setLocModalOpen(false)}>
                Cancelar
              </button>
            </div>
            {createLoc.isError && (
              <div className="msg msg--err" style={{ marginTop: 12 }}>
                {(createLoc.error as Error).message}
              </div>
            )}
          </div>
        </div>
      )}

      {tgSendConfirmOpen && (
        <div
          className="modal-backdrop"
          role="dialog"
          aria-modal="true"
          aria-labelledby="tg-send-title"
          onClick={(e) => e.target === e.currentTarget && !sendTg.isPending && setTgSendConfirmOpen(false)}
        >
          <div className="modal" style={{ maxWidth: 420 }} onClick={(e) => e.stopPropagation()}>
            <h3 id="tg-send-title">Enviar relatório por Telegram?</h3>
            <p style={{ fontSize: 13, color: "var(--muted)", marginTop: 0 }}>
              Será enviado o agregado de <strong>{formatYearMonthPt(month)}</strong> para o chat configurado em Telegram (relatórios).
            </p>
            <div className="row" style={{ gap: 8, marginTop: 12, justifyContent: "flex-end" }}>
              <button type="button" className="btn" disabled={sendTg.isPending} onClick={() => setTgSendConfirmOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={sendTg.isPending}
                onClick={() => {
                  sendTg.mutate(undefined, {
                    onSettled: () => setTgSendConfirmOpen(false),
                  });
                }}
              >
                {sendTg.isPending ? "A enviar…" : "Confirmar envio"}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
