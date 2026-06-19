import { PAGE_SIZE_OPTIONS, type ConnectionsViewPrefs, resetConnectionsPrefs } from "../../lib/connectionsPreferences";
import { SideDrawer } from "../../components/SideDrawer";

type Props = {
  open: boolean;
  tab: string;
  prefs: ConnectionsViewPrefs;
  columnOptions: Array<{ id: string; label: string }>;
  onChange: (p: ConnectionsViewPrefs) => void;
  onClose: () => void;
  onSave: () => void;
};

export function ConnectionsSettingsDrawer({ open, tab, prefs, columnOptions, onChange, onClose, onSave }: Props) {
  function toggleColumn(id: string) {
    const hidden = prefs.hiddenColumns.includes(id)
      ? prefs.hiddenColumns.filter((c) => c !== id)
      : [...prefs.hiddenColumns, id];
    onChange({ ...prefs, hiddenColumns: hidden });
  }

  return (
    <SideDrawer
      open={open}
      title="Configurações"
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="btn"
            onClick={() => onChange(resetConnectionsPrefs(tab))}
          >
            Restaurar padrão
          </button>
          <button type="button" className="btn btn--primary" onClick={onSave}>
            Guardar
          </button>
        </>
      }
    >
      <label className="field" style={{ display: "block", marginBottom: 14 }}>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Registos por página</span>
        <select
          className="input"
          value={prefs.pageSize}
          onChange={(e) => onChange({ ...prefs, pageSize: Number(e.target.value) })}
        >
          {PAGE_SIZE_OPTIONS.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
      </label>

      <label className="field" style={{ display: "block", marginBottom: 14 }}>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Ordenação padrão</span>
        <select
          className="input"
          value={prefs.sortDir}
          onChange={(e) => onChange({ ...prefs, sortDir: e.target.value as "asc" | "desc" })}
        >
          <option value="asc">Crescente</option>
          <option value="desc">Decrescente</option>
        </select>
      </label>

      <label className="conn-switch" style={{ display: "flex", marginBottom: 16 }}>
        <input
          type="checkbox"
          checked={prefs.showSecondaryInfo}
          onChange={(e) => onChange({ ...prefs, showSecondaryInfo: e.target.checked })}
        />
        Exibir informações secundárias
      </label>

      {columnOptions.length > 0 ? (
        <fieldset style={{ border: "1px solid var(--border)", borderRadius: 8, padding: 10 }}>
          <legend style={{ fontSize: 12, padding: "0 4px" }}>Colunas visíveis</legend>
          {columnOptions.map((col) => (
            <label key={col.id} className="conn-switch" style={{ display: "flex", marginBottom: 6 }}>
              <input
                type="checkbox"
                checked={!prefs.hiddenColumns.includes(col.id)}
                onChange={() => toggleColumn(col.id)}
              />
              {col.label}
            </label>
          ))}
        </fieldset>
      ) : null}

      <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 14 }}>
        As preferências são guardadas neste navegador para o seu usuário.
      </p>
    </SideDrawer>
  );
}
