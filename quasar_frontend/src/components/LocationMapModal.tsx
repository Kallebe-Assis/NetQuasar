import { useEffect } from "react";
import { MapContainer, Marker, TileLayer, useMap } from "react-leaflet";
import "leaflet/dist/leaflet.css";
import { copyText, formatLatLng, googleMapsUrl } from "../lib/geoClipboard";
import { INFRA_MAP_KIND_LABELS, infrastructurePinIcon, type InfraMapKind } from "../lib/mapInfrastructureIcons";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";

function MapInvalidateSize() {
  const map = useMap();
  useEffect(() => {
    const run = () => map.invalidateSize();
    run();
    const t = window.setTimeout(run, 120);
    return () => window.clearTimeout(t);
  }, [map]);
  return null;
}

function MapFlyTo({ lat, lng }: { lat: number; lng: number }) {
  const map = useMap();
  useEffect(() => {
    map.setView([lat, lng], 16, { animate: false });
  }, [map, lat, lng]);
  return null;
}

export type LocationMapPreview = {
  title: string;
  subtitle?: string;
  lat: number;
  lng: number;
  kind: InfraMapKind;
  color?: string | null;
};

type Props = {
  preview: LocationMapPreview | null;
  onClose: () => void;
};

export function LocationMapModal({ preview, onClose }: Props) {
  const { push: pushToast } = useAppToast();

  if (!preview) return null;

  const { title, subtitle, lat, lng, kind, color } = preview;
  const icon = infrastructurePinIcon(kind, color);

  async function handleCopyCoords() {
    try {
      await copyText(formatLatLng(lat, lng));
      toastOk(pushToast, "Coordenadas copiadas.");
    } catch (e) {
      toastErr(pushToast, e);
    }
  }

  async function handleCopyMapsLink() {
    try {
      await copyText(googleMapsUrl(lat, lng));
      toastOk(pushToast, "Link do Google Maps copiado.");
    } catch (e) {
      toastErr(pushToast, e);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal conn-form-modal conn-form-modal--infra"
        role="dialog"
        aria-modal="true"
        aria-labelledby="location-map-title"
        style={{ maxWidth: 720, width: "min(720px, 96vw)" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="conn-form-modal__head">
          <h2 id="location-map-title">{title}</h2>
          <p>
            {INFRA_MAP_KIND_LABELS[kind]}
            {subtitle ? ` · ${subtitle}` : ""}
          </p>
        </div>
        <div className="conn-form-modal__body">
          <div style={{ borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)" }}>
            <MapContainer
              center={[lat, lng]}
              zoom={16}
              style={{ height: 360, width: "100%" }}
              scrollWheelZoom
            >
              <MapInvalidateSize />
              <MapFlyTo lat={lat} lng={lng} />
              <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a>' url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
              <Marker position={[lat, lng]} icon={icon} />
            </MapContainer>
          </div>
          <p className="mono" style={{ margin: "10px 0 0", fontSize: 12 }}>
            {formatLatLng(lat, lng)}
          </p>
        </div>
        <div className="conn-form-modal__foot row" style={{ gap: 8, flexWrap: "wrap" }}>
          <button type="button" className="btn" onClick={() => void handleCopyCoords()}>
            Copiar coordenadas
          </button>
          <button type="button" className="btn" onClick={() => void handleCopyMapsLink()}>
            Copiar link Google Maps
          </button>
          <button type="button" className="btn btn--primary" style={{ marginLeft: "auto" }} onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}

/** Converte variant da aba de infraestrutura para kind do mapa. */
export function infraVariantToMapKind(variant: "cto" | "splice" | "cable" | "pole"): InfraMapKind {
  if (variant === "splice") return "splice_box";
  return variant;
}
