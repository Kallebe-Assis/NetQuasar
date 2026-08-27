import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  Compass,
  Copy,
  Fingerprint,
  Gauge,
  Network,
  RefreshCw,
  Repeat2,
  Router,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  X,
} from "lucide-react";
import { useMemo, useState } from "react";
import {
  buildTelnetReportSections,
  buildUnifiedReportTable,
  formatTelnetReportPlainText,
  type OltTelnetReportField,
  type OltTelnetReportStep,
} from "../../lib/oltTelnetReportFormat";
import { EM_DASH } from "../../lib/formatDisplay";

export type OltOnuReportPonMeta = {
  pon?: number | null;
  onu?: number | null;
  pon_description?: string | null;
  pon_vlan?: string | number | null;
};

type Props = {
  open: boolean;
  loading: boolean;
  title: string;
  oltDescription?: string;
  steps: OltTelnetReportStep[];
  ponMeta?: OltOnuReportPonMeta | null;
  onClose: () => void;
  onRefresh?: () => void;
  refreshing?: boolean;
};

function ponMetaRows(meta?: OltOnuReportPonMeta | null): OltTelnetReportField[] {
  if (!meta) return [];
  const pon = meta.pon != null && Number(meta.pon) > 0 ? String(meta.pon) : "";
  return [
    { label: "PON", value: pon || EM_DASH },
    { label: "Descrição da PON", value: String(meta.pon_description ?? "").trim() || EM_DASH },
    { label: "VLAN", value: String(meta.pon_vlan ?? "").trim() || EM_DASH },
  ];
}

// --- leitura tolerante dos campos já parseados (buildUnifiedReportTable) --------------------
// Vendors/comandos diferentes produzem rótulos ligeiramente diferentes — em vez de assumir um
// nome exato, procura por qualquer um dos aliases (exato) e, em último caso, por substring.

function findField(rows: OltTelnetReportField[], ...labels: string[]): string | undefined {
  const wanted = new Set(labels.map((l) => l.toLowerCase()));
  const hit = rows.find((r) => wanted.has(r.label.toLowerCase()));
  const v = hit?.value?.trim();
  return v && v !== EM_DASH ? v : undefined;
}

function findFieldLoose(rows: OltTelnetReportField[], substr: string): string | undefined {
  const s = substr.toLowerCase();
  const hit = rows.find((r) => r.label.toLowerCase().includes(s));
  const v = hit?.value?.trim();
  return v && v !== EM_DASH ? v : undefined;
}

function parseDbm(v?: string): number | null {
  if (!v) return null;
  const m = v.match(/-?\d+(\.\d+)?/);
  if (!m) return null;
  const n = Number(m[0]);
  return Number.isFinite(n) ? n : null;
}

// Faixas de referência GPON (sensibilidade típica de ONU: -8 a -28/-30 dBm) — usadas só para
// colorir o medidor visual; os alertas reais de potência continuam a usar os limiares
// configuráveis em Configurações -> Alertas, independentes daqui.
const RX_MIN = -30;
const RX_MAX = -8;
const RX_BAD_AT = -27;
const RX_OK_AT = -23;
const TX_MIN = -5;
const TX_MAX = 8;

type PowerBand = "ruim" | "aceitavel" | "bom";

function rxBand(v: number): PowerBand {
  if (v <= RX_BAD_AT) return "ruim";
  if (v <= RX_OK_AT) return "aceitavel";
  return "bom";
}

function clampPercent(v: number, min: number, max: number): number {
  const pct = ((v - min) / (max - min)) * 100;
  return Math.max(2, Math.min(98, pct));
}

const BAND_COLOR: Record<PowerBand, string> = { ruim: "var(--err)", aceitavel: "var(--warn)", bom: "var(--ok)" };

function PowerGauge({ label, dbmText }: { label: string; dbmText?: string }) {
  const v = parseDbm(dbmText);
  if (v == null) return null;
  const isRx = label === "RX";
  const band = isRx ? rxBand(v) : "bom";
  const color = isRx ? BAND_COLOR[band] : "var(--warn)";
  const pct = isRx ? clampPercent(v, RX_MIN, RX_MAX) : clampPercent(v, TX_MIN, TX_MAX);
  return (
    <div className="onu-report-gauge" style={{ borderColor: `color-mix(in srgb, ${color} 45%, var(--border))`, background: `color-mix(in srgb, ${color} 10%, var(--panel2))` }}>
      <div className="onu-report-gauge__value" style={{ color }}>
        {v.toFixed(3)} dBm
      </div>
      <div className="onu-report-gauge__track">
        <div className="onu-report-gauge__fill" style={{ width: `${pct}%`, background: color }} />
      </div>
      {isRx ? (
        <div className="onu-report-gauge__bands">
          <span style={{ color: band === "ruim" ? BAND_COLOR.ruim : "var(--muted)" }}>Ruim</span>
          <span style={{ color: band === "aceitavel" ? BAND_COLOR.aceitavel : "var(--muted)" }}>Aceitável</span>
          <span style={{ color: band === "bom" ? BAND_COLOR.bom : "var(--muted)" }}>Bom</span>
        </div>
      ) : null}
      <div className="onu-report-gauge__label">{label} Level</div>
    </div>
  );
}

function InfoRow({ icon, label, value }: { icon: React.ReactNode; label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="onu-report-inforow">
      <span className="onu-report-inforow__icon">{icon}</span>
      <div>
        <div className="onu-report-inforow__label">{label}:</div>
        <div className="onu-report-inforow__value">{value}</div>
      </div>
    </div>
  );
}

function Card({
  icon,
  title,
  children,
  tone,
}: {
  icon: React.ReactNode;
  title: string;
  children: React.ReactNode;
  tone?: "ok" | "err";
}) {
  return (
    <div className={`onu-report-card${tone ? ` onu-report-card--${tone}` : ""}`}>
      <div className="onu-report-card__head">
        <span className="onu-report-card__icon">{icon}</span>
        <span className="onu-report-card__title">{title}</span>
      </div>
      <div className="onu-report-card__body">{children}</div>
    </div>
  );
}

function KV({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="onu-report-kv">
      <span className="onu-report-kv__label">{label}</span>
      <span className="onu-report-kv__value">{value}</span>
    </div>
  );
}

export function OltOnuTelnetReportModal({ open, loading, title, oltDescription, steps, ponMeta, onClose, onRefresh, refreshing }: Props) {
  const [showRaw, setShowRaw] = useState(false);
  const sections = useMemo(() => buildTelnetReportSections(steps), [steps]);
  const rows = useMemo(() => {
    const extra = ponMetaRows(ponMeta);
    const extraKeys = new Set(extra.map((r) => r.label.toLowerCase()));
    const parsed = buildUnifiedReportTable(sections).filter((r) => !extraKeys.has(r.label.toLowerCase()));
    return [...extra, ...parsed];
  }, [sections, ponMeta]);

  if (!open) return null;

  async function copyAll() {
    const text = formatTelnetReportPlainText(rows, title);
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      /* ignore */
    }
  }

  const name = findField(rows, "Nome");
  const serial = findField(rows, "SN");
  const pon = ponMeta?.pon != null && Number(ponMeta.pon) > 0 ? String(ponMeta.pon) : undefined;
  const subtitleParts = [
    name && serial ? `${name} (${serial})` : name || serial,
    oltDescription,
    pon ? `PON ${pon}` : undefined,
  ].filter(Boolean);
  const subtitle = subtitleParts.length > 0 ? subtitleParts.join(" · ") : title;

  const rx = findField(rows, "RX");
  const tx = findField(rows, "TX");
  const distance = findField(rows, "Distância");
  const uptime = findField(rows, "Tempo online");
  const vlan = findField(rows, "VLAN");
  const iface = findField(rows, "Interface", "Interface PON");
  const admin = findField(rows, "Admin");
  const estado = findField(rows, "Estado");
  const statusOnu = findField(rows, "Status ONU");
  const config = findField(rows, "Config");
  const modelo = findField(rows, "Modelo");
  const modeloReportado = findField(rows, "Modelo reportado");
  const hw = findField(rows, "HW");
  const sw = findField(rows, "SW");
  const auth = findField(rows, "Autenticação");
  const speed = findField(rows, "Velocidade actual", "Velocidade config.");
  const fec = findField(rows, "FEC");
  const dba = findFieldLoose(rows, "dba");

  const configFail = config ? /fail|erro|error/i.test(config) : false;
  const configOk = config ? !configFail && /success|ok|normal/i.test(config) : false;

  const hasLinkStatus = rx || tx || distance || uptime;
  const hasNetworkCard = vlan || iface;
  const hasDeviceStateCard = admin || estado || statusOnu;
  const hasHardwareCard = modelo || modeloReportado || hw || sw || serial;
  const hasParamsCard = auth || speed || fec || dba;

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !loading && onClose()}>
      <div
        className="modal modal--wide onu-report-modal"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 780, maxHeight: "92vh", display: "flex", flexDirection: "column" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexShrink: 0 }}>
          <div style={{ minWidth: 0 }}>
            <h3 style={{ marginTop: 0, marginBottom: 4 }}>Detalhes da ONU</h3>
            <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>{subtitle}</p>
          </div>
          <div className="row" style={{ gap: 6, flexShrink: 0 }}>
            <button type="button" className="btn btn--icon" title="Copiar" disabled={loading || rows.length === 0} onClick={() => void copyAll()}>
              <Copy size={16} />
            </button>
            <button type="button" className="btn btn--icon" title="Fechar" onClick={onClose}>
              <X size={16} />
            </button>
          </div>
        </div>

        {rows.length === 0 && loading ? (
          <p style={{ margin: "16px 0" }}>A recolher dados via telnet…</p>
        ) : rows.length === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 12, margin: "16px 0" }}>Nenhum dado estruturado na resposta.</p>
        ) : (
          <div style={{ overflow: "auto", flex: 1, minHeight: 0, marginTop: 14 }}>
            {hasLinkStatus ? (
              <Card icon={<ShieldCheck size={16} />} title="Status do Link">
                <div className="onu-report-linkstatus">
                  <PowerGauge label="RX" dbmText={rx} />
                  <PowerGauge label="TX" dbmText={tx} />
                  <div className="onu-report-linkstatus__meta">
                    <InfoRow icon={<Compass size={15} />} label="Distância" value={distance} />
                    <InfoRow icon={<Clock size={15} />} label="Tempo Online" value={uptime} />
                  </div>
                </div>
              </Card>
            ) : null}

            <div className="onu-report-grid">
              <div className="onu-report-col">
                <div className="onu-report-col__title">
                  <Settings size={14} /> Configuração &amp; Rede
                </div>
                {hasNetworkCard ? (
                  <Card icon={<Network size={16} />} title="VLAN & Interface">
                    <KV label="VLAN" value={vlan} />
                    <KV label="Interface" value={iface} />
                  </Card>
                ) : null}
                {hasDeviceStateCard ? (
                  <Card icon={<CheckCircle2 size={16} />} title="Estado do Dispositivo">
                    <KV label="Admin" value={admin} />
                    <KV label="Estado" value={estado} />
                    <KV label="Status ONU" value={statusOnu} />
                  </Card>
                ) : null}
                {config ? (
                  <Card icon={<AlertTriangle size={16} />} title="Configuração" tone={configFail ? "err" : configOk ? "ok" : undefined}>
                    {configFail ? (
                      <p className="onu-report-alertmsg">
                        FALHA DE CONFIGURAÇÃO ({config}) — Verifique as credenciais da OLT.
                      </p>
                    ) : (
                      <KV label="Estado" value={config} />
                    )}
                  </Card>
                ) : null}
              </div>

              <div className="onu-report-col">
                <div className="onu-report-col__title">
                  <Router size={14} /> Hardware &amp; Software
                </div>
                {hasHardwareCard ? (
                  <Card icon={<Router size={16} />} title="Modelo & Hardware">
                    <KV label="Modelo" value={modelo} />
                    <KV label="Modelo reportado" value={modeloReportado} />
                    <KV label="HW" value={hw} />
                    <KV label="SW" value={sw} />
                    {serial ? (
                      <div className="onu-report-kv">
                        <span className="onu-report-kv__label" style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
                          <Fingerprint size={12} /> Serial
                        </span>
                        <span className="onu-report-kv__value mono">{serial}</span>
                      </div>
                    ) : null}
                  </Card>
                ) : null}
                {hasParamsCard ? (
                  <Card icon={<SlidersHorizontal size={16} />} title="Parâmetros Técnicos">
                    <div className="onu-report-params-grid">
                      {auth ? (
                        <div className="onu-report-param">
                          <ShieldCheck size={14} />
                          <div>
                            <div className="onu-report-param__label">Autenticação</div>
                            <div className="onu-report-param__value">{auth}</div>
                          </div>
                        </div>
                      ) : null}
                      {speed ? (
                        <div className="onu-report-param">
                          <Gauge size={14} />
                          <div>
                            <div className="onu-report-param__label">Velocidade</div>
                            <div className="onu-report-param__value">{speed}</div>
                          </div>
                        </div>
                      ) : null}
                      {fec ? (
                        <div className="onu-report-param">
                          <ShieldCheck size={14} style={{ color: /disable/i.test(fec) ? "var(--muted)" : "var(--ok)" }} />
                          <div>
                            <div className="onu-report-param__label">FEC</div>
                            <div className="onu-report-param__value">{fec}</div>
                          </div>
                        </div>
                      ) : null}
                      {dba ? (
                        <div className="onu-report-param">
                          <Repeat2 size={14} />
                          <div>
                            <div className="onu-report-param__label">DBA Mode</div>
                            <div className="onu-report-param__value">{dba}</div>
                          </div>
                        </div>
                      ) : null}
                    </div>
                  </Card>
                ) : null}
              </div>
            </div>

            {showRaw && sections.length > 0 ? (
              <details open style={{ marginTop: 16 }}>
                <summary style={{ cursor: "pointer", fontSize: 12, color: "var(--muted)", marginBottom: 8 }}>
                  Saída telnet original
                </summary>
                {sections.map((sec) => (
                  <div key={sec.id} style={{ marginBottom: 12 }}>
                    <div className="mono" style={{ fontSize: 10, color: "var(--muted)", marginBottom: 4 }}>
                      {sec.command}
                    </div>
                    <pre
                      style={{
                        margin: 0,
                        fontSize: 10,
                        whiteSpace: "pre-wrap",
                        wordBreak: "break-word",
                        background: "var(--surface-2, rgba(0,0,0,0.04))",
                        padding: 10,
                        borderRadius: 6,
                        border: "1px solid var(--border)",
                      }}
                    >
                      {sec.rawClean || EM_DASH}
                    </pre>
                  </div>
                ))}
              </details>
            ) : null}
          </div>
        )}

        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginTop: 14, flexShrink: 0 }}>
          <button type="button" className="btn-link" disabled={loading} onClick={() => setShowRaw((v) => !v)}>
            {showRaw ? "Ocultar saída bruta" : "Ver Saída Bruta (JSON/Txt)"}
          </button>
          {onRefresh ? (
            <button type="button" className="btn btn--primary" disabled={loading || refreshing} onClick={onRefresh}>
              <RefreshCw size={14} style={{ marginRight: 6, verticalAlign: -2 }} className={refreshing ? "map-refresh-spin" : undefined} />
              {refreshing ? "A atualizar…" : "Atualizar Dados"}
            </button>
          ) : (
            <button type="button" className="btn btn--primary" disabled={loading} onClick={onClose}>
              Fechar
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
