import { Info } from "lucide-react";
import type { ReactNode } from "react";

type InfoHintProps = {
  children: ReactNode;
  /** Rótulo para leitores de ecrã */
  label?: string;
  className?: string;
};

/** Ícone discreto; o texto de ajuda aparece ao passar o rato ou focar. */
export function InfoHint({ children, label = "Informação adicional", className = "" }: InfoHintProps) {
  return (
    <span className={`info-hint${className ? ` ${className}` : ""}`}>
      <button type="button" className="info-hint__trigger" aria-label={label} title={label}>
        <Info size={15} strokeWidth={2} aria-hidden />
      </button>
      <span className="info-hint__popover" role="tooltip">
        <span className="info-hint__content">{children}</span>
      </span>
    </span>
  );
}
