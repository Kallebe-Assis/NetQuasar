import { fmtCoord } from "./networkInfrastructure";

export function formatLatLng(lat: number, lng: number): string {
  return `${fmtCoord(lat)}, ${fmtCoord(lng)}`;
}

export function googleMapsUrl(lat: number, lng: number): string {
  return `https://www.google.com/maps?q=${lat},${lng}`;
}

/** @deprecated Use googleMapsUrl — mantido por compatibilidade. */
export function formatLatLngWithGoogleMaps(lat: number, lng: number): string {
  return googleMapsUrl(lat, lng);
}

export async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.left = "-9999px";
  document.body.appendChild(ta);
  ta.select();
  document.execCommand("copy");
  document.body.removeChild(ta);
}
