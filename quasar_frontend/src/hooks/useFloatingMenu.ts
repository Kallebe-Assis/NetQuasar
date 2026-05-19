import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";

export type FloatingAlign = "start" | "end";

const PANEL_Z = 10050;

export function useFloatingMenu(align: FloatingAlign = "end", minWidth = 220) {
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const [panelStyle, setPanelStyle] = useState<CSSProperties>({});

  const updatePosition = useCallback(() => {
    const anchor = anchorRef.current;
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    const panel = panelRef.current;
    const panelW = Math.max(minWidth, panel?.offsetWidth ?? minWidth);
    const panelH = panel?.offsetHeight ?? 180;
    let left = align === "end" ? rect.right - panelW : rect.left;
    left = Math.max(8, Math.min(left, window.innerWidth - panelW - 8));
    let top = rect.bottom + 4;
    if (top + panelH > window.innerHeight - 8) {
      top = Math.max(8, rect.top - panelH - 4);
    }
    setPanelStyle({ position: "fixed", top, left, minWidth, zIndex: PANEL_Z });
  }, [align, minWidth]);

  useLayoutEffect(() => {
    if (!open) return;
    updatePosition();
    const panel = panelRef.current;
    const ro = panel ? new ResizeObserver(() => updatePosition()) : null;
    if (panel && ro) ro.observe(panel);
    window.addEventListener("scroll", updatePosition, true);
    window.addEventListener("resize", updatePosition);
    return () => {
      ro?.disconnect();
      window.removeEventListener("scroll", updatePosition, true);
      window.removeEventListener("resize", updatePosition);
    };
  }, [open, updatePosition]);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      const t = e.target as Node;
      if (anchorRef.current?.contains(t) || panelRef.current?.contains(t)) return;
      setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const close = useCallback(() => setOpen(false), []);
  const toggle = useCallback(() => setOpen((v) => !v), []);

  return {
    open,
    setOpen,
    anchorRef,
    panelRef,
    panelStyle,
    toggle,
    close,
  };
}
