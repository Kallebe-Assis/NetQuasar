import { useId, useState } from "react";

/** Texto numa linha com opção de ver o conteúdo completo. */
export function TableCellExpandableText({
  text,
  maxLength = 80,
}: {
  text?: string | null;
  maxLength?: number;
}) {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const value = (text ?? "").trim();
  if (!value) return <>—</>;

  const needsToggle = value.length > maxLength;
  const preview = needsToggle ? `${value.slice(0, maxLength).trimEnd()}…` : value;

  if (!needsToggle) {
    return <span className="integration-cell-text">{value}</span>;
  }

  return (
    <div className="integration-cell-text-wrap">
      <span className="integration-cell-text">{open ? value : preview}</span>
      <button
        type="button"
        className="integration-cell-text__toggle"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((v) => !v)}
      >
        {open ? "menos" : "mais"}
      </button>
      {open ? (
        <div id={panelId} className="integration-cell-text__full" role="region">
          {value}
        </div>
      ) : null}
    </div>
  );
}
