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

export function AutomationRecurrenceFields({ value, allowed, disabled, onChange }: Props) {
  const kinds = allowed.length ? allowed : (["daily"] as RecurrenceKind[]);
  const kind = kinds.includes(value.kind) ? value.kind : kinds[0];

  function setKind(next: RecurrenceKind) {
    onChange({
      ...value,
      kind: next,
      weekdays:
        next === "weekly" ? (value.weekdays.length === 1 ? value.weekdays : [1]) : next === "custom" ? (value.weekdays.length ? value.weekdays : [1, 2, 3, 4, 5]) : [],
    });
  }

  function toggleDay(d: number) {
    const has = value.weekdays.includes(d);
    const weekdays = has ? value.weekdays.filter((x) => x !== d) : [...value.weekdays, d];
    onChange({ ...value, weekdays: weekdays.length ? weekdays : [d] });
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

        {kind === "weekly" ? (
          <SettingsField label="Dia da semana">
            <select
              className="input"
              value={String(value.weekdays[0] ?? 1)}
              disabled={disabled}
              onChange={(e) => onChange({ ...value, weekdays: [Number(e.target.value)] })}
            >
              {WEEKDAY_LABELS.map((l, i) => (
                <option key={l} value={String(i)}>
                  {l}
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

      {kind === "custom" ? (
        <>
          <div className="automation-weekdays" role="group" aria-label="Dias da semana">
            {WEEKDAY_LABELS.map((l, i) => {
              const on = value.weekdays.includes(i);
              return (
                <button
                  key={l}
                  type="button"
                  className={`automation-weekdays__day${on ? " is-on" : ""}`}
                  disabled={disabled}
                  aria-pressed={on}
                  onClick={() => toggleDay(i)}
                >
                  {l}
                </button>
              );
            })}
          </div>
          {value.weekdays.length > 0 && value.weekdays.length < 7 ? (
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
              O agendamento corre à hora escolhida em cada dia marcado. Se o motor ainda só aceitar um dia, usa o primeiro
              seleccionado.
            </p>
          ) : null}
        </>
      ) : null}
    </div>
  );
}
