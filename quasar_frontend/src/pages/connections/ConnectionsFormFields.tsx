import { FIBER_COLORS } from "../../lib/networkInfrastructure";
import type { CommercialLocality, NetworkProject } from "../../lib/networkInfrastructure";

type CoordProps = {
  latitude: string;
  longitude: string;
  onChange: (lat: string, lon: string) => void;
};

export function CoordFields({ latitude, longitude, onChange }: CoordProps) {
  return (
    <div className="row" style={{ gap: 8 }}>
      <label className="field" style={{ flex: 1 }}>
        Latitude
        <input className="input mono" value={latitude} onChange={(e) => onChange(e.target.value, longitude)} />
      </label>
      <label className="field" style={{ flex: 1 }}>
        Longitude
        <input className="input mono" value={longitude} onChange={(e) => onChange(latitude, e.target.value)} />
      </label>
    </div>
  );
}

type ProjectSelectProps = {
  value: string;
  projects: NetworkProject[];
  onChange: (id: string) => void;
};

export function ProjectSelect({ value, projects, onChange }: ProjectSelectProps) {
  return (
    <label className="field">
      Projeto
      <select className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {projects.map((p) => (
          <option key={p.id} value={p.id}>
            #{p.display_number} — {p.description}
          </option>
        ))}
      </select>
    </label>
  );
}

type LocalitySelectProps = {
  value: string;
  localities: CommercialLocality[];
  onChange: (id: string) => void;
};

export function LocalitySelect({ value, localities, onChange }: LocalitySelectProps) {
  return (
    <label className="field">
      Localidade
      <select className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {localities.map((l) => (
          <option key={l.id} value={l.id}>
            {l.name}
          </option>
        ))}
      </select>
    </label>
  );
}

type MaintenanceSwitchProps = {
  checked: boolean;
  onChange: (v: boolean) => void;
};

export function MaintenanceSwitch({ checked, onChange }: MaintenanceSwitchProps) {
  return (
    <label className="conn-switch">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      Necessita manutenção
    </label>
  );
}

type FiberColorSelectProps = {
  value: string;
  onChange: (v: string) => void;
};

export function FiberColorSelect({ value, onChange }: FiberColorSelectProps) {
  return (
    <label className="field">
      Cor da fibra
      <select className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {FIBER_COLORS.map((c) => (
          <option key={c} value={c}>
            {c}
          </option>
        ))}
      </select>
    </label>
  );
}
