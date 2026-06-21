import { useId } from "react";
import { FIBER_COLORS } from "../../lib/networkInfrastructure";
import type { CommercialLocality, NetworkProject } from "../../lib/networkInfrastructure";

type CoordProps = {
  latitude: string;
  longitude: string;
  onChange: (lat: string, lon: string) => void;
};

export function CoordFields({ latitude, longitude, onChange }: CoordProps) {
  const latId = useId();
  const lonId = useId();
  return (
    <>
      <div className="conn-form-modal__field">
        <label className="conn-form-modal__field-label" htmlFor={latId}>
          Latitude
        </label>
        <input id={latId} className="input mono" value={latitude} onChange={(e) => onChange(e.target.value, longitude)} />
      </div>
      <div className="conn-form-modal__field">
        <label className="conn-form-modal__field-label" htmlFor={lonId}>
          Longitude
        </label>
        <input id={lonId} className="input mono" value={longitude} onChange={(e) => onChange(latitude, e.target.value)} />
      </div>
    </>
  );
}

type ProjectSelectProps = {
  value: string;
  projects: NetworkProject[];
  onChange: (id: string) => void;
};

export function ProjectSelect({ value, projects, onChange }: ProjectSelectProps) {
  const id = useId();
  return (
    <div className="conn-form-modal__field">
      <label className="conn-form-modal__field-label" htmlFor={id}>
        Projeto
      </label>
      <select id={id} className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {projects.map((p) => (
          <option key={p.id} value={p.id}>
            #{p.display_number} — {p.description}
          </option>
        ))}
      </select>
    </div>
  );
}

type LocalitySelectProps = {
  value: string;
  localities: CommercialLocality[];
  onChange: (id: string) => void;
};

export function LocalitySelect({ value, localities, onChange }: LocalitySelectProps) {
  const id = useId();
  return (
    <div className="conn-form-modal__field">
      <label className="conn-form-modal__field-label" htmlFor={id}>
        Localidade
      </label>
      <select id={id} className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {localities.map((l) => (
          <option key={l.id} value={l.id}>
            {l.name}
          </option>
        ))}
      </select>
    </div>
  );
}

type MaintenanceSwitchProps = {
  checked: boolean;
  onChange: (v: boolean) => void;
};

export function MaintenanceSwitch({ checked, onChange }: MaintenanceSwitchProps) {
  const id = useId();
  return (
    <label className="conn-switch" htmlFor={id}>
      <input id={id} type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      Necessita manutenção
    </label>
  );
}

type FiberColorSelectProps = {
  value: string;
  onChange: (v: string) => void;
};

export function FiberColorSelect({ value, onChange }: FiberColorSelectProps) {
  const id = useId();
  return (
    <div className="conn-form-modal__field">
      <label className="conn-form-modal__field-label" htmlFor={id}>
        Cor da fibra
      </label>
      <select id={id} className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {FIBER_COLORS.map((c) => (
          <option key={c} value={c}>
            {c}
          </option>
        ))}
      </select>
    </div>
  );
}
