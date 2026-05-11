import L from "leaflet";
import { useEffect, useMemo } from "react";
import { MapContainer, Marker, TileLayer, useMap, useMapEvents } from "react-leaflet";
import markerIcon2x from "leaflet/dist/images/marker-icon-2x.png";
import markerIcon from "leaflet/dist/images/marker-icon.png";
import markerShadow from "leaflet/dist/images/marker-shadow.png";
import "leaflet/dist/leaflet.css";

const DEFAULT_CENTER: [number, number] = [-14.235, -51.9253];

function fixLeafletIcons() {
  const def = L.Icon.Default.prototype as unknown as { _getIconUrl?: unknown };
  delete def._getIconUrl;
  L.Icon.Default.mergeOptions({
    iconUrl: markerIcon,
    iconRetinaUrl: markerIcon2x,
    shadowUrl: markerShadow,
  });
}

fixLeafletIcons();

function MapClickLayer({ onPick }: { onPick: (lat: number, lng: number) => void }) {
  useMapEvents({
    click(e) {
      onPick(e.latlng.lat, e.latlng.lng);
    },
  });
  return null;
}

function MapRecenter({ latitude, longitude, zoom }: { latitude: number | null; longitude: number | null; zoom: number }) {
  const map = useMap();
  useEffect(() => {
    if (latitude == null || longitude == null || !Number.isFinite(latitude) || !Number.isFinite(longitude)) return;
    map.setView([latitude, longitude], zoom, { animate: true });
  }, [map, latitude, longitude, zoom]);
  return null;
}

type Props = {
  latitude: number | null;
  longitude: number | null;
  onChange: (lat: number, lng: number) => void;
};

/** Mapa simples: clique define o ponto; marcador arrastável para ajustar. */
export function PopLocationPicker({ latitude, longitude, onChange }: Props) {
  const defaultIcon = useMemo(() => new L.Icon.Default(), []);

  const has = latitude != null && longitude != null && Number.isFinite(latitude) && Number.isFinite(longitude);
  const center: [number, number] = has ? [latitude!, longitude!] : DEFAULT_CENTER;
  const zoom = has ? 14 : 5;

  return (
    <div>
      <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
        Clique no mapa para colocar o marcador; depois pode arrastá-lo para ajustar a posição.
      </p>
      <MapContainer center={center} zoom={zoom} style={{ height: 240, width: "100%", borderRadius: "var(--radius)", border: "1px solid var(--border)" }}>
        <MapRecenter latitude={latitude} longitude={longitude} zoom={zoom} />
        <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a>' url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
        <MapClickLayer onPick={onChange} />
        {has && (
          <Marker
            position={[latitude!, longitude!]}
            icon={defaultIcon}
            draggable
            eventHandlers={{
              dragend: (e) => {
                const ll = e.target.getLatLng();
                onChange(ll.lat, ll.lng);
              },
            }}
          />
        )}
      </MapContainer>
    </div>
  );
}
