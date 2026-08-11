import { buildOltPonChoices, matchOltByTransmitter, oltDisplayName, type OltPonCatalog } from "../lib/oltPonInterfaces";

type Props = {
  olts: OltPonCatalog[];
  oltDeviceId: string;
  transmitter?: string;
  pon: string;
  onChange: (next: { olt_device_id: string; transmitter: string; pon: string }) => void;
  disabled?: boolean;
  oltLabel?: string;
  ponLabel?: string;
  fieldClassName?: string;
  labelClassName?: string;
};

export function OltInterfaceSelects({
  olts,
  oltDeviceId,
  transmitter,
  pon,
  onChange,
  disabled,
  oltLabel = "Transmissor (OLT)",
  ponLabel = "Interface (PON)",
  fieldClassName = "conn-form-modal__field",
  labelClassName = "conn-form-modal__field-label",
}: Props) {
  const selected = matchOltByTransmitter(olts, transmitter, oltDeviceId || null);
  const effectiveOltId = oltDeviceId || selected?.id || "";
  const choices = buildOltPonChoices(selected);

  return (
    <>
      <div className={fieldClassName}>
        <span className={labelClassName}>{oltLabel}</span>
        <select
          className="input"
          disabled={disabled}
          value={effectiveOltId}
          onChange={(e) => {
            const id = e.target.value;
            const olt = olts.find((o) => o.id === id);
            onChange({
              olt_device_id: id,
              transmitter: olt ? oltDisplayName(olt) : "",
              pon: "",
            });
          }}
        >
          <option value="">—</option>
          {olts.map((o) => (
            <option key={o.id} value={o.id}>
              {oltDisplayName(o)}
            </option>
          ))}
        </select>
      </div>
      <div className={fieldClassName}>
        <span className={labelClassName}>{ponLabel}</span>
        <select
          className="input"
          disabled={disabled || !effectiveOltId || choices.length === 0}
          value={pon}
          onChange={(e) =>
            onChange({
              olt_device_id: effectiveOltId,
              transmitter: selected ? oltDisplayName(selected) : transmitter ?? "",
              pon: e.target.value,
            })
          }
        >
          <option value="">{effectiveOltId ? (choices.length ? "Seleccione a interface…" : "Sem PONs cadastradas nesta OLT") : "Escolha o transmissor"}</option>
          {choices.map((c) => (
            <option key={c.pon} value={String(c.pon)}>
              {c.label}
            </option>
          ))}
        </select>
      </div>
    </>
  );
}
