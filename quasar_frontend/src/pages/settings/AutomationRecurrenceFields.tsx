import { SettingsField } from "../../components/SettingsField";
import {
  WEEKDAY_LABELS,
  recurrenceLabel,
  type RecurrenceDraft,
  type RecurrenceKind,
} from "../../lib/automationJobs";

type Props = {
  value: RecurrenceDraft;
  allowed: RecurrenceKind[];
  disabled?: boolean;
  onChange: (next: RecurrenceDraft) => void;
};

export function AutomationWeekdayChecks({
  days,
  disabled,
  onChange,
}: {
  days: number[];
  disabled?: boolean;
  onChange: (days: number[]) => void;
}) {
  function toggle(d: number) {
    const has = days.includes(d);
    const next = has ? days.filter((x) => x !== d) : [...days, d];
    onChange((next.length ? next : [d]).sort((a, b) => a - b));
  }
  return (
    <div className="automation-weekdays" role="group" aria-label="Dias da semana">
      {WEEKDAY_LABELS.map((l, i) => {
        const on = days.includes(i);
        return (
          <label key={l} className={`automation-weekdays__day${on ? " is-on" : ""}`}>
            <input type="checkbox" checked={on} disabled={disabled} onChange={() => toggle(i)} />
            {l}
          </label>
        );
      })}
    </div>
  );
}

export function AutomationRecurrenceFields({ value, allowed, disabled, onChange }: Props) {
  const kinds = allowed.length ? allowed : (["daily"] as RecurrenceKind[]);
  const kind = kinds.includes(value.kind) ? value.kind : kinds[0];

  function setKind(next: RecurrenceKind) {
    onChange({
      ...value,
      kind: next,
      weekdays:
        next === "weekly"
          ? value.weekdays.length
            ? value.weekdays
            : [1]
          : next === "custom"
            ? value.weekdays.length
              ? value.weekdays
              : [1, 2, 3, 4, 5]
            : [],
    });
  }

  return (
    <div className="automation-recurrence">
      <div className="automation-recurrence__kinds" role="group" aria-label="Recorrência">
        {kinds.map((k) => (
          <button
            key={k}
            type="button"
            className={`automation-recurrence__kind${kind === k ? " is-active" : ""}`}
            disabled={disabled}
            onClick={() => setKind(k)}
          >
            {recurrenceLabel(k)}
          </button>
        ))}
      </div>

      <div className="settings-fields-grid" style={{ marginTop: 12 }}>
        {kind === "monthly" ? (
          <SettingsField label="Dia do mês">
            <select
              className="input"
              value={String(value.dayOfMonth)}
              disabled={disabled}
              onChange={(e) => onChange({ ...value, dayOfMonth: Number(e.target.value) || 1 })}
            >
              {Array.from({ length: 31 }, (_, i) => i + 1).map((d) => (
                <option key={d} value={String(d)}>
                  {d}
                </option>
              ))}
            </select>
          </SettingsField>
        ) : null}

        <SettingsField label="Hora">
          <input
            className="input"
            type="time"
            value={value.time}
            disabled={disabled}
            onChange={(e) => onChange({ ...value, time: e.target.value })}
          />
        </SettingsField>
        <SettingsField label="Fuso horário">
          <input
            className="input mono"
            value={value.timezone}
            disabled={disabled}
            onChange={(e) => onChange({ ...value, timezone: e.target.value })}
          />
        </SettingsField>
      </div>

      {kind === "weekly" || kind === "custom" ? (
        <>
          <p style={{ fontSize: 12, color: "var(--muted)", margin: "12px 0 0" }}>Dias da semana</p>
          <AutomationWeekdayChecks
            days={value.weekdays.length ? value.weekdays : [1]}
            disabled={disabled}
            onChange={(weekdays) => onChange({ ...value, weekdays })}
          />
        </>
      ) : null}
    </div>
  );
}
