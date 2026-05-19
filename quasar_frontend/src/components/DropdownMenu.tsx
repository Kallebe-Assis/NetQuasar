import type { ReactNode } from "react";
import { FloatingMenuPanel } from "./FloatingMenuPanel";
import { useFloatingMenu, type FloatingAlign } from "../hooks/useFloatingMenu";

export type DropdownMenuApi = { close: () => void; open: boolean };

type Props = {
  align?: FloatingAlign;
  minWidth?: number;
  className?: string;
  panelClassName?: string;
  trigger: (api: { toggle: () => void; open: boolean }) => ReactNode;
  children: (api: DropdownMenuApi) => ReactNode;
};

function defaultPanelClass(align: FloatingAlign) {
  return `action-menu__panel action-menu__panel--portal${align === "start" ? " action-menu__panel--align-start" : ""}`;
}

/** Menu dropdown com portal (não é cortado por overflow em tabelas). */
export function DropdownMenu({ align = "end", minWidth, className, panelClassName, trigger, children }: Props) {
  const fm = useFloatingMenu(align, minWidth);
  return (
    <div ref={fm.anchorRef} className={className}>
      {trigger({ toggle: fm.toggle, open: fm.open })}
      <FloatingMenuPanel
        open={fm.open}
        panelRef={fm.panelRef}
        panelStyle={fm.panelStyle}
        className={panelClassName ?? defaultPanelClass(align)}
      >
        {children({ close: fm.close, open: fm.open })}
      </FloatingMenuPanel>
    </div>
  );
}
