import { Loader2 } from "lucide-react";

/** Indicador de carregamento padrão de toda consulta à HubSoft (spinner + texto + barra animada). */
export function ConsultaLoading({ text = "Consultando a HubSoft…" }: { text?: string }) {
  return (
    <div className="hubsoft-consulta-loading" role="status" aria-live="polite">
      <Loader2 size={20} className="map-refresh-spin" aria-hidden />
      <span>{text}</span>
      <div className="hubsoft-consulta-loading__bar" aria-hidden />
    </div>
  );
}
