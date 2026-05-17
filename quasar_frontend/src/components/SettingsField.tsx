import type { ReactNode } from "react";
import { InfoHint } from "./InfoHint";

type SettingsFieldProps = {
  label: string;
  hintLabel: string;
  hint: ReactNode;
  children: ReactNode;
};

/** Rótulo + InfoHint por cima do controlo (formulários em Configurações). */
export function SettingsField({ label, hintLabel, hint, children }: SettingsFieldProps) {
  return (
    <div className="settings-field">
      <div className="settings-field__head">
        <span className="settings-field__title">{label}</span>
        <InfoHint label={hintLabel}>{hint}</InfoHint>
      </div>
      {children}
    </div>
  );
}
