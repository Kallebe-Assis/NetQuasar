import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Play, Radio, Square, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { formatBytes } from "../../lib/formatBytes";
import { EM_DASH } from "../../lib/formatDisplay";
import { toastErr, toastOk } from "../../lib/operationToast";

type PacketRow = {
  seq: number;
  ts: string;
  length: number;
  protocol: string;
  src_ip?: string;
  dst_ip?: string;
  src_port?: number;
  dst_port?: number;
  info: string;
};

type PacketDetail = PacketRow & { hex: string };

type SessionStatusResp = {
  id: string;
  name: string;
  source: string;
  interface: string;
  status: "running" | "stopped" | "saved";
  started_at: string;
  stopped_at?: string;
  packet_count: number;
  total_bytes: number;
  error?: string;
};

type CaptureSummary = {
  id: string;
  name: string;
  description: string;
  source: "local" | "device";
  device_id?: string;
  interface: string;
  started_at: string;
  stopped_at?: string;
  packet_count: number;
  total_bytes: number;
};

type DeviceOpt = { id: string; description?: string | null; category?: string | null; ip?: string | null };

const PROTO_TONE: Record<string, string> = {
  TCP: "blue",
  UDP: "purple",
  ICMP: "orange",
  ICMPV6: "orange",
  ARP: "gray",
  DNS: "green",
};

function protoTone(p: string): string {
  return PROTO_TONE[p.toUpperCase()] ?? "gray";
}

function fmtTime(ts: string): string {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return EM_DASH;
  return d.toLocaleTimeString("pt-PT", { hour12: false }) + "." + String(d.getMilliseconds()).padStart(3, "0");
}

function endpoint(ip?: string, port?: number): string {
  if (!ip) return EM_DASH;
  return port ? `${ip}:${port}` : ip;
}

function PacketTable({
  packets,
  onSelect,
  selectedSeq,
  autoScroll,
}: {
  packets: PacketRow[];
  onSelect: (seq: number) => void;
  selectedSeq: number | null;
  autoScroll?: boolean;
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);

  useEffect(() => {
    if (!autoScroll || !stickToBottom.current) return;
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [packets.length, autoScroll]);

  return (
    <div
      className="table-wrap sniffer-table-wrap"
      ref={wrapRef}
      onScroll={() => {
        const el = wrapRef.current;
        if (!el) return;
        stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
      }}
    >
      <table className="conn-table mono sniffer-table" style={{ fontSize: 11.5 }}>
        <thead>
          <tr>
            <th>#</th>
            <th>Hora</th>
            <th>Origem</th>
            <th>Destino</th>
            <th>Protocolo</th>
            <th>Tamanho</th>
            <th>Info</th>
          </tr>
        </thead>
        <tbody>
          {packets.map((p) => (
            <tr
              key={p.seq}
              className={`row-interactive${selectedSeq === p.seq ? " row-interactive--selected" : ""}`}
              onClick={() => onSelect(p.seq)}
            >
              <td>{p.seq}</td>
              <td>{fmtTime(p.ts)}</td>
              <td>{endpoint(p.src_ip, p.src_port)}</td>
              <td>{endpoint(p.dst_ip, p.dst_port)}</td>
              <td>
                <span className={`sniffer-proto sniffer-proto--${protoTone(p.protocol)}`}>{p.protocol}</span>
              </td>
              <td>{p.length}</td>
              <td className="sniffer-table__info">{p.info}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {packets.length === 0 ? <p style={{ padding: 12, color: "var(--muted)", fontSize: 12 }}>Nenhum pacote ainda.</p> : null}
      <div ref={bottomRef} />
    </div>
  );
}

function PacketDetailPanel({ packet, loading, onClose }: { packet: PacketDetail | null; loading: boolean; onClose: () => void }) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div className="modal modal--wide sniffer-detail" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
          <h3 style={{ margin: 0 }}>Pacote {packet ? `#${packet.seq}` : ""}</h3>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        {loading || !packet ? (
          <p style={{ color: "var(--muted)", fontSize: 12 }}>A carregar…</p>
        ) : (
          <>
            <div className="sniffer-detail__meta">
              <div>
                <span className="sniffer-detail__k">Hora</span>
                <span className="mono">{new Date(packet.ts).toLocaleString("pt-PT")}</span>
              </div>
              <div>
                <span className="sniffer-detail__k">Protocolo</span>
                <span className={`sniffer-proto sniffer-proto--${protoTone(packet.protocol)}`}>{packet.protocol}</span>
              </div>
              <div>
                <span className="sniffer-detail__k">Origem</span>
                <span className="mono">{endpoint(packet.src_ip, packet.src_port)}</span>
              </div>
              <div>
                <span className="sniffer-detail__k">Destino</span>
                <span className="mono">{endpoint(packet.dst_ip, packet.dst_port)}</span>
              </div>
              <div>
                <span className="sniffer-detail__k">Tamanho</span>
                <span className="mono">{packet.length} bytes</span>
              </div>
              <div>
                <span className="sniffer-detail__k">Info</span>
                <span>{packet.info}</span>
              </div>
            </div>
            <pre className="sniffer-hex mono">{packet.hex}</pre>
          </>
        )}
      </div>
    </div>
  );
}

export function SnifferTab() {
  const { push: pushToast } = useAppToast();
  const qc = useQueryClient();

  // --- iniciar captura ---
  const [source, setSource] = useState<"local" | "device">("local");
  const [ifaceLocal, setIfaceLocal] = useState("any");
  const [deviceId, setDeviceId] = useState("");
  const [ifaceRemote, setIfaceRemote] = useState("any");
  const [sessionName, setSessionName] = useState("");

  const ifacesQ = useQuery({
    queryKey: ["sniffer-interfaces"],
    queryFn: () => apiFetch<{ interfaces: { name: string; label: string }[] }>("/api/v1/tools/sniffer/interfaces"),
  });
  const devicesQ = useQuery({
    queryKey: ["devices"],
    queryFn: () => apiFetch<{ devices: DeviceOpt[] }>("/api/v1/devices"),
  });

  // --- sessão ao vivo ---
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [liveStatus, setLiveStatus] = useState<SessionStatusResp["status"] | null>(null);
  const [liveMeta, setLiveMeta] = useState<{ count: number; bytes: number } | null>(null);
  const [liveError, setLiveError] = useState<string | null>(null);
  const [packets, setPackets] = useState<PacketRow[]>([]);
  const [showSaveDiscard, setShowSaveDiscard] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [saveDesc, setSaveDesc] = useState("");
  const lastSeqRef = useRef(0);
  const pollTimerRef = useRef<number | null>(null);
  const stopPromptShown = useRef(false);

  useEffect(() => {
    if (!activeSessionId) return;
    let cancelled = false;

    async function poll() {
      try {
        const res = await apiFetch<{ packets: PacketRow[]; packet_count: number; total_bytes: number; status: string }>(
          `/api/v1/tools/sniffer/sessions/${activeSessionId}/packets?since=${lastSeqRef.current}`,
        );
        if (cancelled) return;
        if (res.packets.length > 0) {
          lastSeqRef.current = res.packets[res.packets.length - 1].seq;
          setPackets((prev) => [...prev, ...res.packets]);
        }
        setLiveMeta({ count: res.packet_count, bytes: res.total_bytes });
        if (res.status !== "running") {
          setLiveStatus(res.status as "stopped" | "saved");
          if (res.status === "stopped" && !stopPromptShown.current) {
            stopPromptShown.current = true;
            try {
              const st = await apiFetch<SessionStatusResp>(`/api/v1/tools/sniffer/sessions/${activeSessionId}`);
              if (st.error) setLiveError(st.error);
            } catch {
              /* sessão pode já ter sido descartada — ignora */
            }
            setSaveName((n) => n || sessionName || "Captura");
            setShowSaveDiscard(true);
          }
          return;
        }
      } catch {
        return; // sessão desapareceu (descartada noutro sítio) — pára silenciosamente
      }
      if (!cancelled) pollTimerRef.current = window.setTimeout(poll, 700);
    }
    void poll();
    return () => {
      cancelled = true;
      if (pollTimerRef.current != null) window.clearTimeout(pollTimerRef.current);
    };
  }, [activeSessionId, sessionName]);

  const capturesQ = useQuery({
    queryKey: ["sniffer-captures"],
    queryFn: () => apiFetch<{ captures: CaptureSummary[] }>("/api/v1/tools/sniffer/captures"),
  });

  const startMut = useMutation({
    mutationFn: (body: Record<string, unknown>) => apiFetch<{ id: string }>("/api/v1/tools/sniffer/sessions", { method: "POST", json: body }),
    onSuccess: (res) => {
      setPackets([]);
      lastSeqRef.current = 0;
      stopPromptShown.current = false;
      setLiveError(null);
      setLiveMeta({ count: 0, bytes: 0 });
      setLiveStatus("running");
      setActiveSessionId(res.id);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao iniciar a captura."),
  });

  const stopMut = useMutation({
    mutationFn: () => apiFetch(`/api/v1/tools/sniffer/sessions/${activeSessionId}/stop`, { method: "POST" }),
    onError: (e) => toastErr(pushToast, e, "Falha ao parar a captura."),
  });

  const saveMut = useMutation({
    mutationFn: () =>
      apiFetch<{ id: string; packet_count: number }>(`/api/v1/tools/sniffer/sessions/${activeSessionId}/save`, {
        method: "POST",
        json: { name: saveName.trim(), description: saveDesc.trim() },
      }),
    onSuccess: (res) => {
      toastOk(pushToast, `Captura guardada (${res.packet_count} pacote(s)).`);
      setShowSaveDiscard(false);
      setActiveSessionId(null);
      setPackets([]);
      void qc.invalidateQueries({ queryKey: ["sniffer-captures"] });
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao guardar a captura."),
  });

  const discardMut = useMutation({
    mutationFn: () => apiFetch(`/api/v1/tools/sniffer/sessions/${activeSessionId}/discard`, { method: "POST" }),
    onSuccess: () => {
      toastOk(pushToast, "Captura descartada.");
      setShowSaveDiscard(false);
      setActiveSessionId(null);
      setPackets([]);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao descartar a captura."),
  });

  // --- captura guardada em visualização ---
  const [viewingCaptureId, setViewingCaptureId] = useState<string | null>(null);
  const [filters, setFilters] = useState({ ip: "", protocol: "", min_len: "", max_len: "", from: "", to: "", q: "" });
  const [appliedFilters, setAppliedFilters] = useState(filters);

  const capturePacketsQ = useQuery({
    queryKey: ["sniffer-capture-packets", viewingCaptureId, appliedFilters],
    enabled: !!viewingCaptureId,
    queryFn: () => {
      const params = new URLSearchParams();
      if (appliedFilters.ip.trim()) params.set("ip", appliedFilters.ip.trim());
      if (appliedFilters.protocol.trim()) params.set("protocol", appliedFilters.protocol.trim());
      if (appliedFilters.min_len.trim()) params.set("min_len", appliedFilters.min_len.trim());
      if (appliedFilters.max_len.trim()) params.set("max_len", appliedFilters.max_len.trim());
      if (appliedFilters.from.trim()) params.set("from", new Date(appliedFilters.from).toISOString());
      if (appliedFilters.to.trim()) params.set("to", new Date(appliedFilters.to).toISOString());
      if (appliedFilters.q.trim()) params.set("q", appliedFilters.q.trim());
      return apiFetch<{ packets: PacketRow[]; matched: number; limit: number }>(
        `/api/v1/tools/sniffer/captures/${viewingCaptureId}/packets?${params.toString()}`,
      );
    },
  });

  const deleteCaptureMut = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/tools/sniffer/captures/${id}`, { method: "DELETE" }),
    onSuccess: (_data, id) => {
      toastOk(pushToast, "Captura excluída.");
      void qc.invalidateQueries({ queryKey: ["sniffer-captures"] });
      if (viewingCaptureId === id) setViewingCaptureId(null);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao excluir captura."),
  });

  // --- detalhe de pacote ---
  const [detailSeq, setDetailSeq] = useState<number | null>(null);
  const [detailPacket, setDetailPacket] = useState<PacketDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  async function openDetail(seq: number) {
    setDetailSeq(seq);
    setDetailLoading(true);
    setDetailPacket(null);
    try {
      const url = viewingCaptureId
        ? `/api/v1/tools/sniffer/captures/${viewingCaptureId}/packets/${seq}`
        : `/api/v1/tools/sniffer/sessions/${activeSessionId}/packets/${seq}`;
      const res = await apiFetch<PacketDetail>(url);
      setDetailPacket(res);
    } catch (e) {
      toastErr(pushToast, e, "Falha ao carregar o detalhe do pacote.");
      setDetailSeq(null);
    } finally {
      setDetailLoading(false);
    }
  }

  const isCapturing = !!activeSessionId && liveStatus === "running";
  const capturePending = !!activeSessionId && liveStatus !== "saved";

  const viewingCapture = useMemo(
    () => capturesQ.data?.captures.find((c) => c.id === viewingCaptureId) ?? null,
    [capturesQ.data?.captures, viewingCaptureId],
  );

  function startCapture() {
    const name = sessionName.trim();
    if (source === "local") {
      startMut.mutate({ source: "local", interface: ifaceLocal, name });
    } else {
      if (!deviceId) {
        toastErr(pushToast, new Error("Escolha um equipamento."));
        return;
      }
      startMut.mutate({ source: "device", device_id: deviceId, interface: ifaceRemote.trim() || "any", name });
    }
  }

  return (
    <div className="sniffer-tab">
      {!capturePending ? (
        <div className="card sniffer-start">
          <div className="row" style={{ gap: 10, flexWrap: "wrap", alignItems: "flex-end" }}>
            <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Origem</span>
              <select className="select" value={source} onChange={(e) => setSource(e.target.value as "local" | "device")}>
                <option value="local">Servidor NetQuasar (local)</option>
                <option value="device">Equipamento remoto (SSH)</option>
              </select>
            </label>

            {source === "local" ? (
              <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                <span style={{ fontSize: 12, color: "var(--muted)" }}>Interface</span>
                <select className="select" value={ifaceLocal} onChange={(e) => setIfaceLocal(e.target.value)} style={{ minWidth: 180 }}>
                  {(ifacesQ.data?.interfaces ?? [{ name: "any", label: "Todas as interfaces" }]).map((i) => (
                    <option key={i.name} value={i.name}>
                      {i.label}
                    </option>
                  ))}
                </select>
              </label>
            ) : (
              <>
                <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Equipamento</span>
                  <select className="select" value={deviceId} onChange={(e) => setDeviceId(e.target.value)} style={{ minWidth: 220 }}>
                    <option value="">— Seleccione —</option>
                    {(devicesQ.data?.devices ?? []).map((d) => (
                      <option key={d.id} value={d.id}>
                        {d.description ?? d.id} {d.ip ? `(${d.ip})` : ""}
                      </option>
                    ))}
                  </select>
                </label>
                <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Interface remota</span>
                  <input className="input mono" value={ifaceRemote} onChange={(e) => setIfaceRemote(e.target.value)} placeholder="any" style={{ width: 120 }} />
                </label>
              </>
            )}

            <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>Nome (opcional)</span>
              <input className="input" value={sessionName} onChange={(e) => setSessionName(e.target.value)} placeholder="Ex.: Diagnóstico cliente X" style={{ minWidth: 200 }} />
            </label>

            <button type="button" className="btn btn--primary" disabled={startMut.isPending} onClick={startCapture}>
              <Play size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
              {startMut.isPending ? "A iniciar…" : "Iniciar captura"}
            </button>
          </div>
          {source === "device" ? (
            <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 8 }}>
              Requer utilizador/palavra-passe SSH configurados no equipamento e <span className="mono">tcpdump</span> instalado
              com privilégio de captura (root/CAP_NET_RAW). Funciona bem em servidores Linux; a maioria de routers/OLTs não expõe
              um shell com tcpdump.
            </p>
          ) : (
            <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 8 }}>
              Captura o tráfego de/para o próprio servidor NetQuasar (rede Docker) — não o tráfego geral da sua rede local.
            </p>
          )}
        </div>
      ) : null}

      {capturePending ? (
        <div className="card sniffer-live">
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 10 }}>
            <div className="row" style={{ gap: 10, alignItems: "center" }}>
              {isCapturing ? <span className="sniffer-live-dot" aria-hidden /> : null}
              <strong>{isCapturing ? "A capturar…" : "Captura parada"}</strong>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>
                {source === "local" ? `interface ${ifaceLocal}` : `equipamento (${ifaceRemote})`} · {liveMeta?.count ?? 0} pacote(s) ·{" "}
                {formatBytes(liveMeta?.bytes ?? 0)}
              </span>
            </div>
            {isCapturing ? (
              <button type="button" className="btn btn--danger" disabled={stopMut.isPending} onClick={() => stopMut.mutate()}>
                <Square size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
                {stopMut.isPending ? "A parar…" : "Parar captura"}
              </button>
            ) : null}
          </div>
          {liveError ? <p className="msg msg--err" style={{ marginTop: 8, fontSize: 12 }}>{liveError}</p> : null}
          <div style={{ marginTop: 10 }}>
            <PacketTable packets={packets} onSelect={openDetail} selectedSeq={detailSeq} autoScroll={isCapturing} />
          </div>
        </div>
      ) : null}

      {!capturePending ? (
        <div className="card">
          <h3 style={{ marginTop: 0 }}>Capturas guardadas</h3>
          {capturesQ.isPending ? <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar…</p> : null}
          {!capturesQ.isPending && (capturesQ.data?.captures.length ?? 0) === 0 ? (
            <p style={{ fontSize: 12, color: "var(--muted)" }}>Nenhuma captura guardada ainda.</p>
          ) : null}
          {(capturesQ.data?.captures.length ?? 0) > 0 ? (
            <div className="table-wrap">
              <table className="conn-table" style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>Nome</th>
                    <th>Origem</th>
                    <th>Interface</th>
                    <th>Pacotes</th>
                    <th>Tamanho</th>
                    <th>Iniciada em</th>
                    <th style={{ width: 96 }} />
                  </tr>
                </thead>
                <tbody>
                  {(capturesQ.data?.captures ?? []).map((c) => (
                    <tr key={c.id} className="row-interactive" onClick={() => { setViewingCaptureId(c.id); setFilters({ ip: "", protocol: "", min_len: "", max_len: "", from: "", to: "", q: "" }); setAppliedFilters({ ip: "", protocol: "", min_len: "", max_len: "", from: "", to: "", q: "" }); }}>
                      <td>{c.name}</td>
                      <td>{c.source === "local" ? "Local" : "Equipamento"}</td>
                      <td className="mono">{c.interface || EM_DASH}</td>
                      <td>{c.packet_count}</td>
                      <td>{formatBytes(c.total_bytes)}</td>
                      <td className="mono">{new Date(c.started_at).toLocaleString("pt-PT")}</td>
                      <td>
                        <button
                          type="button"
                          className="btn btn--icon"
                          title="Excluir captura"
                          aria-label="Excluir captura"
                          onClick={(e) => {
                            e.stopPropagation();
                            if (window.confirm(`Excluir a captura «${c.name}»? Esta ação não pode ser desfeita.`)) {
                              deleteCaptureMut.mutate(c.id);
                            }
                          }}
                        >
                          <Trash2 size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </div>
      ) : null}

      {viewingCaptureId && viewingCapture ? (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setViewingCaptureId(null)}>
          <div className="modal modal--wide sniffer-view" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
              <h3 style={{ margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
                <Radio size={16} aria-hidden /> {viewingCapture.name}
              </h3>
              <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={() => setViewingCaptureId(null)}>
                <X size={16} />
              </button>
            </div>

            <div className="row sniffer-filters" style={{ gap: 8, flexWrap: "wrap", marginBottom: 10 }}>
              <input className="input" placeholder="IP contém…" value={filters.ip} onChange={(e) => setFilters({ ...filters, ip: e.target.value })} style={{ width: 140 }} />
              <select className="select" value={filters.protocol} onChange={(e) => setFilters({ ...filters, protocol: e.target.value })} style={{ width: 120 }}>
                <option value="">Protocolo</option>
                <option value="TCP">TCP</option>
                <option value="UDP">UDP</option>
                <option value="ICMP">ICMP</option>
                <option value="ARP">ARP</option>
                <option value="DNS">DNS</option>
              </select>
              <input className="input mono" placeholder="Tam. mín." value={filters.min_len} onChange={(e) => setFilters({ ...filters, min_len: e.target.value.replace(/\D/g, "") })} style={{ width: 90 }} />
              <input className="input mono" placeholder="Tam. máx." value={filters.max_len} onChange={(e) => setFilters({ ...filters, max_len: e.target.value.replace(/\D/g, "") })} style={{ width: 90 }} />
              <input className="input" type="datetime-local" value={filters.from} onChange={(e) => setFilters({ ...filters, from: e.target.value })} style={{ width: 176 }} />
              <input className="input" type="datetime-local" value={filters.to} onChange={(e) => setFilters({ ...filters, to: e.target.value })} style={{ width: 176 }} />
              <input className="input" placeholder="Pesquisar (info, IP, protocolo)…" value={filters.q} onChange={(e) => setFilters({ ...filters, q: e.target.value })} style={{ flex: 1, minWidth: 160 }} />
              <button type="button" className="btn btn--primary" onClick={() => setAppliedFilters(filters)}>
                Filtrar
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  const empty = { ip: "", protocol: "", min_len: "", max_len: "", from: "", to: "", q: "" };
                  setFilters(empty);
                  setAppliedFilters(empty);
                }}
              >
                Limpar
              </button>
            </div>

            {capturePacketsQ.data ? (
              <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
                {capturePacketsQ.data.matched} correspondência(s)
                {capturePacketsQ.data.matched > capturePacketsQ.data.packets.length
                  ? ` — mostrando as primeiras ${capturePacketsQ.data.packets.length} (refine o filtro para ver mais)`
                  : ""}
              </p>
            ) : null}
            {capturePacketsQ.isFetching ? <p style={{ fontSize: 12, color: "var(--muted)" }}>A pesquisar…</p> : null}
            <PacketTable packets={capturePacketsQ.data?.packets ?? []} onSelect={openDetail} selectedSeq={detailSeq} />
          </div>
        </div>
      ) : null}

      {detailSeq != null ? (
        <PacketDetailPanel packet={detailPacket} loading={detailLoading} onClose={() => setDetailSeq(null)} />
      ) : null}

      {showSaveDiscard ? (
        <div className="modal-backdrop" role="presentation">
          <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 440 }} onMouseDown={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Guardar esta captura?</h3>
            <p style={{ fontSize: 12, color: "var(--muted)" }}>
              {liveMeta?.count ?? 0} pacote(s) capturados. Guardar grava um ficheiro .pcap pesquisável (por IP, protocolo, tamanho e
              data); descartar apaga tudo agora.
            </p>
            <div className="field">
              <label>Nome</label>
              <input className="input" value={saveName} onChange={(e) => setSaveName(e.target.value)} />
            </div>
            <div className="field">
              <label>Descrição (opcional)</label>
              <input className="input" value={saveDesc} onChange={(e) => setSaveDesc(e.target.value)} />
            </div>
            <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
              <button type="button" className="btn btn--danger" disabled={discardMut.isPending || saveMut.isPending} onClick={() => discardMut.mutate()}>
                {discardMut.isPending ? "A descartar…" : "Descartar"}
              </button>
              <button type="button" className="btn btn--primary" disabled={discardMut.isPending || saveMut.isPending || !saveName.trim()} onClick={() => saveMut.mutate()}>
                {saveMut.isPending ? "A guardar…" : "Guardar"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
