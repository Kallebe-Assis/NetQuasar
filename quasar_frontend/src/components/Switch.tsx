import { useId, type ReactNode } from "react";

type Props = {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: ReactNode;
  hint?: ReactNode;
  disabled?: boolean;
  id?: string;
};

/** Interruptor ON/OFF reutilizável — usa o mesmo visual do `.toggle` do resto da app. */
export function Switch({ checked, onChange, label, hint, disabled, id }: Props) {
  const autoId = useId();
  const inputId = id ?? autoId;
  return (
    <label className="toggle" htmlFor={inputId} style={{ alignItems: "flex-start", opacity: disabled ? 0.55 : 1 }}>
      <span className="toggle__track" style={{ marginTop: 1 }}>
        <input
          id={inputId}
          type="checkbox"
          role="switch"
          className="toggle__input"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="toggle__thumb" aria-hidden />
      </span>
      <span style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
        <span className="toggle__label" style={{ color: "var(--text)", fontSize: 13 }}>
          {label}
        </span>
        {hint ? (
          <span style={{ color: "var(--muted)", fontSize: 11 }}>{hint}</span>
        ) : null}
      </span>
    </label>
  );
}
