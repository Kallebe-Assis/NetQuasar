import { createPortal } from "react-dom";
import type { CSSProperties, ReactNode, Ref } from "react";

type Props = {
  open: boolean;
  panelRef: Ref<HTMLDivElement>;
  panelStyle: CSSProperties;
  className?: string;
  children: ReactNode;
  role?: string;
  onMouseLeave?: () => void;
};

/** Painel de menu renderizado em document.body (evita corte por overflow em tabelas). */
export function FloatingMenuPanel({ open, panelRef, panelStyle, className, children, role = "menu", onMouseLeave }: Props) {
  if (!open) return null;
  return createPortal(
    <div ref={panelRef} className={className} style={panelStyle} role={role} onMouseLeave={onMouseLeave}>
      {children}
    </div>,
    document.body,
  );
}
