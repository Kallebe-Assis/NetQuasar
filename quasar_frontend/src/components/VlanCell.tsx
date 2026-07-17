import { EM_DASH } from "../lib/formatDisplay";

type Props = {
  mode?: string | null;
  vlans?: number[] | null;
  vlanLabel?: string | null;
  /** Máximo de VLANs visíveis antes de «…» (default 5). */
  maxVisible?: number;
};

/**
 * Coluna VLAN: uma VLAN por linha; no máximo N + reticências com title (hover) da lista completa.
 */
export function VlanCell({ mode, vlans, vlanLabel, maxVisible = 5 }: Props) {
  const ids = Array.isArray(vlans) ? [...vlans].filter((n) => Number.isFinite(n) && n > 0).sort((a, b) => a - b) : [];
  const names = parseNamesFromLabel(vlanLabel);

  if (ids.length === 0 && !vlanLabel) {
    return <span className="mono">{EM_DASH}</span>;
  }

  // Sem lista estruturada — fallback ao label legado, ainda com hover completo.
  if (ids.length === 0) {
    const text = String(vlanLabel ?? "").trim() || EM_DASH;
    return (
      <span className="mono" style={{ fontSize: 11, whiteSpace: "normal", lineHeight: 1.35 }} title={text}>
        {text}
      </span>
    );
  }

  const prefix = String(mode ?? "").toLowerCase() === "trunk" ? "trunk" : "";
  const lines = ids.map((id) => {
    const name = names.get(id);
    return name ? `${id} (${name})` : String(id);
  });
  const full = prefix ? `${prefix}:\n${lines.join("\n")}` : lines.join("\n");
  const visible = lines.slice(0, maxVisible);
  const hidden = lines.length - visible.length;

  return (
    <span className="mono" style={{ fontSize: 11, whiteSpace: "pre-line", lineHeight: 1.35, display: "inline-block", maxWidth: 260 }} title={full}>
      {prefix ? (
        <>
          {prefix}
          {"\n"}
        </>
      ) : null}
      {visible.join("\n")}
      {hidden > 0 ? (
        <>
          {"\n"}
          <span style={{ color: "var(--muted)", cursor: "help", textDecoration: "underline dotted" }} title={full}>
            … (+{hidden})
          </span>
        </>
      ) : null}
    </span>
  );
}

function parseNamesFromLabel(label: string | null | undefined): Map<number, string> {
  const out = new Map<number, string>();
  if (!label) return out;
  // Ex.: "trunk: 9 (VLAN-9-…), 10 (VLAN-10-…)" ou "9 (nome)"
  const re = /(\d+)\s*\(([^)]+)\)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(label)) != null) {
    const id = Number(m[1]);
    if (Number.isFinite(id)) out.set(id, m[2].trim());
  }
  return out;
}
