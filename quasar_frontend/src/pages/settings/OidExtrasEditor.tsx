import { InfoHint } from "../../components/InfoHint";
import {
  OID_KIND_GROUPS,
  OID_KIND_BY_VALUE,
  type ExtraOidRow,
  type OidExtraKind,
  newOidRowId,
} from "../../lib/oidExtrasConfig";

type Props = {
  title: string;
  rows: ExtraOidRow[];
  onChange: (rows: ExtraOidRow[]) => void;
};

export function OidExtrasEditor({ title, rows, onChange }: Props) {
  function updateRow(id: string, patch: Partial<ExtraOidRow>) {
    onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  }

  function removeRow(id: string) {
    onChange(rows.filter((r) => r.id !== id));
  }

  function addRow() {
    onChange([...rows, { id: newOidRowId(), kind: "custom", oid: "", label: "" }]);
  }

  return (
    <div className="card" style={{ padding: 14, marginTop: 16 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 15, display: "flex", alignItems: "center", gap: 6 }}>
        {title}
        <InfoHint label="Leituras SNMP extra">
          <p>
            OIDs adicionais incluídos na telemetria de equipamentos BNG (inventário SNMP por categoria). Um identificador
            por linha; a descrição aparece nos relatórios.
          </p>
        </InfoHint>
      </h3>
      {rows.length === 0 ? (
        <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Nenhum extra — use «Adicionar» para incluir mais leituras.</p>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {rows.map((r) => (
            <div key={r.id} className="row" style={{ flexWrap: "wrap", gap: 6, alignItems: "flex-end" }}>
              <select
                title="Tipo de métrica"
                aria-label="Tipo de métrica SNMP"
                className="select"
                style={{ minWidth: 220, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                value={r.kind}
                onChange={(e) => {
                  const kind = e.target.value as OidExtraKind;
                  const meta = OID_KIND_BY_VALUE[kind];
                  updateRow(r.id, {
                    kind,
                    label: r.label || meta?.defaultLabel || "",
                  });
                }}
              >
                {OID_KIND_GROUPS.map((group) => (
                  <optgroup key={group.label} label={group.label}>
                    {group.kinds.map((o) => (
                      <option key={o.value} value={o.value}>
                        {o.label}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
              <input
                title="Identificador SNMP"
                aria-label="Identificador SNMP"
                className="input mono"
                style={{ flex: "1 1 160px", minWidth: 140, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                value={r.oid}
                onChange={(e) => updateRow(r.id, { oid: e.target.value })}
              />
              <input
                title="Descrição no relatório"
                aria-label="Descrição da leitura SNMP extra"
                className="input"
                placeholder="Descrição (relatório)"
                style={{ flex: "1 1 140px", minWidth: 120, fontSize: 11, padding: "4px 6px", minHeight: 32 }}
                value={r.label}
                onChange={(e) => updateRow(r.id, { label: e.target.value })}
              />
              <button type="button" className="btn" style={{ padding: "4px 8px", fontSize: 11 }} onClick={() => removeRow(r.id)}>
                −
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="row" style={{ marginTop: 8, gap: 6 }}>
        <button type="button" className="btn btn--primary" style={{ padding: "4px 10px", fontSize: 11 }} onClick={addRow}>
          Adicionar OID extra
        </button>
      </div>
    </div>
  );
}
