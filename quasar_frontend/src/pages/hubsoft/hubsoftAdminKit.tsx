import { useRef, useState, type DragEvent, type ReactNode } from "react";
import { FileSpreadsheet, Info, TriangleAlert, Upload, X } from "lucide-react";
import "./hubsoftAdmin.css";

/** Painel de uma ferramenta: cabeçalho (ícone + título + descrição + etiqueta) e corpo. */
export function ToolPanel({
  icon,
  title,
  subtitle,
  badge,
  badgeTone,
  children,
}: {
  icon: ReactNode;
  title: string;
  subtitle?: ReactNode;
  badge?: string;
  badgeTone?: "warn" | "ok" | "muted";
  children: ReactNode;
}) {
  return (
    <section className="hsa-panel">
      <header className="hsa-panel__head">
        <span className="hsa-panel__icon" aria-hidden>
          {icon}
        </span>
        <div style={{ minWidth: 0 }}>
          <h3 className="hsa-panel__title">
            {title}
            {badge ? (
              <span className={`hsa-badge${badgeTone === "warn" ? " hsa-badge--warn" : badgeTone === "ok" ? " hsa-badge--ok" : ""}`}>
                {badge}
              </span>
            ) : null}
          </h3>
          {subtitle ? <p className="hsa-panel__sub">{subtitle}</p> : null}
        </div>
      </header>
      <div className="hsa-panel__body">{children}</div>
    </section>
  );
}

/** Um passo numerado de um fluxo em sequência (enviar → validar → aplicar). */
export function Step({
  n,
  title,
  hint,
  done,
  children,
}: {
  n: number;
  title: string;
  hint?: ReactNode;
  done?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={`hsa-step${done ? " is-done" : ""}`}>
      <span className="hsa-step__num" aria-hidden>
        {done ? "✓" : n}
      </span>
      <div className="hsa-step__head">
        <h4 className="hsa-step__title">{title}</h4>
        {hint ? <p className="hsa-step__hint">{hint}</p> : null}
      </div>
      <div className="hsa-step__body">{children}</div>
    </div>
  );
}

export function Callout({ tone = "info", children }: { tone?: "info" | "warn" | "err"; children: ReactNode }) {
  return (
    <div className={`hsa-callout${tone === "warn" ? " hsa-callout--warn" : tone === "err" ? " hsa-callout--err" : ""}`} role={tone === "err" ? "alert" : undefined}>
      {tone === "info" ? <Info size={16} aria-hidden /> : <TriangleAlert size={16} aria-hidden />}
      <div style={{ minWidth: 0 }}>{children}</div>
    </div>
  );
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  return (
    <div className="hsa-seg" role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" className={value === o.value ? "is-active" : ""} aria-pressed={value === o.value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Stat({ label, value, tone }: { label: string; value: ReactNode; tone?: "ok" | "err" | "warn" | "muted" }) {
  return (
    <div className={`hsa-stat${tone ? ` hsa-stat--${tone}` : ""}`}>
      <div className="hsa-stat__k">{label}</div>
      <div className="hsa-stat__v">{value}</div>
    </div>
  );
}

export function Pill({ tone, children }: { tone?: "ok" | "err" | "warn"; children: ReactNode }) {
  return <span className={`hsa-pill${tone ? ` hsa-pill--${tone}` : ""}`}>{children}</span>;
}

export function ProgressBar({ done, total, label }: { done: number; total: number; label?: string }) {
  const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
  return (
    <div>
      <div className="hsa-progress" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={done}>
        <span style={{ width: `${pct}%` }} />
      </div>
      <div className="hsa-progress__label">{label ?? `${done} de ${total}`}</div>
    </div>
  );
}

/** Seletor de CSV estilizado (clique ou arrastar). Substitui o <input type="file"> cru. */
export function CsvDropzone({
  fileName,
  info,
  disabled,
  onFile,
  onClear,
  accept = ".csv,text/csv",
  prompt = "Arraste o CSV aqui ou clique para escolher",
}: {
  fileName: string;
  info?: string;
  disabled?: boolean;
  onFile: (f: File | undefined) => void;
  onClear?: () => void;
  accept?: string;
  prompt?: string;
}) {
  const ref = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const has = !!fileName;

  function drop(e: DragEvent<HTMLLabelElement>) {
    e.preventDefault();
    setOver(false);
    if (disabled) return;
    onFile(e.dataTransfer.files?.[0]);
  }

  return (
    <label
      className={`hsa-drop${over ? " is-over" : ""}${disabled ? " is-disabled" : ""}${has ? " has-file" : ""}`}
      onDragOver={(e) => {
        e.preventDefault();
        if (!disabled) setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={drop}
    >
      <input
        ref={ref}
        className="hsa-drop__input"
        type="file"
        accept={accept}
        disabled={disabled}
        onChange={(e) => {
          onFile(e.target.files?.[0]);
          e.target.value = ""; // permite escolher o mesmo arquivo de novo
        }}
      />
      <span className="hsa-drop__icon" aria-hidden>
        {has ? <FileSpreadsheet size={20} /> : <Upload size={20} />}
      </span>
      <span className="hsa-drop__text">
        <span className="hsa-drop__main">{has ? fileName : prompt}</span>
        <span className="hsa-drop__sub">{has ? info || "Arquivo carregado." : "Formato CSV (separado por vírgula ou ponto e vírgula)"}</span>
      </span>
      {has && onClear ? (
        <button
          type="button"
          className="btn btn--sm btn--icon"
          aria-label="Remover arquivo"
          title="Remover arquivo"
          disabled={disabled}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onClear();
          }}
        >
          <X size={14} />
        </button>
      ) : (
        <span className="btn btn--sm" aria-hidden>
          Escolher arquivo
        </span>
      )}
    </label>
  );
}
