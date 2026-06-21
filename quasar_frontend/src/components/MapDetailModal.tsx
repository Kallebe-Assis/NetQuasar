import { createPortal } from "react-dom";
import { Link } from "react-router-dom";

type PointSummary = {
  id: string;
  description: string;
  category: string;
  lat: number;
  lng: number;
  login?: string;
};

type PointDetail = PointSummary & {
  network_status?: string;
  brand?: string;
  model?: string;
  ip?: string | null;
  mac?: string;
  serial_number?: string;
  software_version?: string;
  hardware_version?: string;
  ping_enabled?: boolean;
  telemetry_enabled?: boolean;
  operational_mode?: string;
  status: string;
  last_check_at?: string | null;
  updated_at?: string | null;
};

function fmtIso(s: string | null | undefined): string {
  if (!s?.trim()) return "—";
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return d.toLocaleString("pt-PT");
}

type Props = {
  open: boolean;
  onClose: () => void;
  selId: string | null;
  selPoint: PointSummary | null;
  isConnPoint: boolean;
  isInfraPoint: boolean;
  detailLoading: boolean;
  detailError: Error | null;
  detail: PointDetail | null | undefined;
};

export function MapDetailModal(props: Props) {
  const { open, onClose, selId, selPoint, isConnPoint, isInfraPoint, detailLoading, detailError, detail } = props;
  if (!open || !selId) return null;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="map-detail-title"
        style={{ maxWidth: 520, width: "min(96vw, 520px)" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, marginBottom: 8 }}>
          <h3 id="map-detail-title" style={{ margin: 0 }}>
            Detalhe
          </h3>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>

        {isInfraPoint && selPoint && (
          <div style={{ fontSize: 13 }}>
            <p style={{ marginTop: 0 }}>
              <strong>{selPoint.description}</strong>
            </p>
            <table style={{ width: "100%", fontSize: 12, borderCollapse: "collapse" }}>
              <tbody>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Tipo</td>
                  <td>{selPoint.category}</td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Coordenadas</td>
                  <td className="mono">
                    {selPoint.lat.toFixed(5)}, {selPoint.lng.toFixed(5)}
                  </td>
                </tr>
              </tbody>
            </table>
            <p style={{ marginTop: 12, fontSize: 11, color: "var(--muted)" }}>
              <Link to="/connections">Ver em Conexões</Link>
            </p>
          </div>
        )}

        {isConnPoint && selPoint && (
          <div style={{ fontSize: 13 }}>
            <p style={{ marginTop: 0 }}>
              <strong>{selPoint.description}</strong>
            </p>
            <table style={{ width: "100%", fontSize: 12, borderCollapse: "collapse" }}>
              <tbody>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Login</td>
                  <td className="mono">{selPoint.login ?? "—"}</td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Tipo</td>
                  <td>{selPoint.category}</td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Coordenadas</td>
                  <td className="mono">
                    {selPoint.lat.toFixed(5)}, {selPoint.lng.toFixed(5)}
                  </td>
                </tr>
              </tbody>
            </table>
            <p style={{ marginTop: 12, fontSize: 11, color: "var(--muted)" }}>
              <Link to="/connections">Ver em Conexões</Link>
            </p>
          </div>
        )}

        {!isConnPoint && !isInfraPoint && detailLoading && <p>A carregar…</p>}
        {!isConnPoint && !isInfraPoint && detailError && <div className="msg msg--err">{detailError.message}</div>}
        {!isConnPoint && !isInfraPoint && detail && (
          <div style={{ fontSize: 13 }}>
            <p style={{ marginTop: 0 }}>
              <strong>{detail.description}</strong>
            </p>
            <table style={{ width: "100%", fontSize: 12, borderCollapse: "collapse" }}>
              <tbody>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0", verticalAlign: "top" }}>Categoria</td>
                  <td className="mono" style={{ padding: "4px 0" }}>
                    {detail.category}
                  </td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Marca / Modelo</td>
                  <td style={{ padding: "4px 0" }}>{[detail.brand, detail.model].filter(Boolean).join(" · ") || "—"}</td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>IP</td>
                  <td className="mono" style={{ padding: "4px 0" }}>
                    {detail.ip ?? "—"}
                  </td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Rede</td>
                  <td style={{ padding: "4px 0" }}>{detail.network_status ?? "—"}</td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Modo operação</td>
                  <td style={{ padding: "4px 0" }}>{detail.operational_mode ?? "—"}</td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Ping / Telemetria</td>
                  <td style={{ padding: "4px 0" }}>
                    {detail.ping_enabled ? "sim" : "não"} / {detail.telemetry_enabled ? "sim" : "não"}
                  </td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>MAC</td>
                  <td className="mono" style={{ padding: "4px 0" }}>
                    {detail.mac || "—"}
                  </td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Coordenadas</td>
                  <td className="mono" style={{ padding: "4px 0" }}>
                    {detail.lat}, {detail.lng}
                  </td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Estado (probe)</td>
                  <td style={{ padding: "4px 0" }}>
                    <span className={`badge ${detail.status === "online" ? "badge--ok" : detail.status === "offline" ? "badge--err" : "badge--off"}`}>
                      {detail.status}
                    </span>
                  </td>
                </tr>
                <tr>
                  <td style={{ color: "var(--muted)", padding: "4px 8px 4px 0" }}>Última verificação</td>
                  <td style={{ padding: "4px 0", fontSize: 11 }}>{fmtIso(detail.last_check_at)}</td>
                </tr>
              </tbody>
            </table>
            <div className="row" style={{ marginTop: 12, flexWrap: "wrap", gap: 8 }}>
              <Link to={`/devices?focus=${encodeURIComponent(String(detail.id))}`} className="btn btn--primary">
                Editar equipamento
              </Link>
              <Link to="/devices" className="btn">
                Lista de equipamentos
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>,
    document.body,
  );
}
