import { useEffect, useRef, useState } from "react";

export type ActionMenuItem = {
  id: string;
  label: string;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
};

export function ActionMenu({
  items,
  title = "Opções",
  /** `start`: painel abre à direita do botão (evita sobrepor menu lateral). `end`: alinha à direita do botão (útil na última coluna da tabela). */
  align = "end",
}: {
  items: ActionMenuItem[];
  title?: string;
  align?: "start" | "end";
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  return (
    <div className="action-menu" ref={rootRef}>
      <button type="button" className="btn btn--icon btn--icon-menu" title={title} aria-label={title} onClick={() => setOpen((v) => !v)}>
        ⋮
      </button>
      {open && (
        <div className={`action-menu__panel ${align === "start" ? "action-menu__panel--align-start" : ""}`}>
          {items.map((it) => (
            <button
              key={it.id}
              type="button"
              className={`action-menu__item ${it.danger ? "action-menu__item--danger" : ""}`}
              disabled={it.disabled}
              onClick={() => {
                setOpen(false);
                it.onClick();
              }}
            >
              {it.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

