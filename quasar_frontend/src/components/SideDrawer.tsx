import { X } from "lucide-react";
import type { ReactNode } from "react";

type Props = {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  width?: number;
};

export function SideDrawer({ open, title, onClose, children, footer, width = 380 }: Props) {
  if (!open) return null;
  return (
    <div className="side-drawer-backdrop" role="presentation" onMouseDown={onClose}>
      <aside
        className="side-drawer"
        style={{ width: `min(${width}px, 96vw)` }}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="side-drawer__head">
          <h3>{title}</h3>
          <button type="button" className="btn btn--icon" onClick={onClose} aria-label="Fechar">
            <X size={18} />
          </button>
        </header>
        <div className="side-drawer__body">{children}</div>
        {footer ? <footer className="side-drawer__foot">{footer}</footer> : null}
      </aside>
    </div>
  );
}
