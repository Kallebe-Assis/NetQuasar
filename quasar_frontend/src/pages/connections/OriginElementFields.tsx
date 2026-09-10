import { useId, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";

/** Tipos de elemento que podem ser a origem do sinal de uma CTO ou foguete. */
export type OriginKind = "" | "pop" | "foguete" | "cto" | "olt" | "switch" | "mikrotik" | "radio";

export type OriginValue = {
  origin_kind: OriginKind;
  origin_ref_id: string;
  origin_label: string;
};

export const ORIGIN_KIND_LABELS: Record<Exclude<OriginKind, "">, string> = {
  pop: "POP",
  foguete: "Foguete (caixa)",
  cto: "CTO",
  olt: "OLT",
  switch: "Switch",
  mikrotik: "MikroTik",
  radio: "Rádio",
};

type Choice = { id: string; label: string };

function useOriginChoices(kind: OriginKind, enabled: boolean): { choices: Choice[]; loading: boolean } {
  const isDevice = kind === "olt" || kind === "switch" || kind === "mikrotik" || kind === "radio";
  const pathByKind: Partial<Record<OriginKind, string>> = {
    pop: "/api/v1/pops",
    foguete: "/api/v1/commercial/network/splice-boxes",
    cto: "/api/v1/commercial/network/ctos",
  };
  const path = isDevice ? "/api/v1/devices" : pathByKind[kind];

  const q = useQuery({
    queryKey: ["origin-choices", isDevice ? "devices" : kind],
    enabled: enabled && !!path,
    staleTime: 60_000,
    queryFn: () => apiFetch<Record<string, unknown>>(path as string),
  });

  const choices = useMemo<Choice[]>(() => {
    const data = q.data;
    if (!data) return [];
    if (isDevice) {
      const rows = (data.devices as Array<Record<string, unknown>>) ?? [];
      const want = kind;
      const matches = rows.filter((r) => String(r.category ?? "").toLowerCase().includes(want));
      const list = matches.length > 0 ? matches : rows;
      return list.map((r) => ({ id: String(r.id), label: String(r.description ?? r.id) }));
    }
    if (kind === "pop") {
      const rows = (data.pops as Array<Record<string, unknown>>) ?? [];
      return rows.map((r) => ({ id: String(r.id), label: String(r.name ?? r.description ?? r.id) }));
    }
    if (kind === "foguete") {
      const rows = (data.splice_boxes as Array<Record<string, unknown>>) ?? [];
      return rows.map((r) => ({
        id: String(r.id),
        label: `#${r.display_number ?? "?"} — ${r.description ?? ""}`.trim(),
      }));
    }
    if (kind === "cto") {
      const rows = (data.ctos as Array<Record<string, unknown>>) ?? [];
      return rows.map((r) => ({
        id: String(r.id),
        label: `#${r.display_number ?? "?"} — ${r.description ?? ""}`.trim(),
      }));
    }
    return [];
  }, [q.data, kind, isDevice]);

  return { choices, loading: q.isLoading };
}

type Props = {
  value: OriginValue;
  onChange: (next: OriginValue) => void;
  disabled?: boolean;
  /** Excluir este id da lista de escolhas (o próprio elemento não pode ser a sua origem). */
  excludeId?: string;
};

/** Selector do elemento de origem (de onde vem a fibra que alimenta este elemento). */
export function OriginElementFields({ value, onChange, disabled, excludeId }: Props) {
  const kindId = useId();
  const refId = useId();
  const labelId = useId();
  const { choices, loading } = useOriginChoices(value.origin_kind, !!value.origin_kind);
  const visibleChoices = excludeId ? choices.filter((c) => c.id !== excludeId) : choices;

  return (
    <>
      <div className="conn-form-modal__field">
        <label className="conn-form-modal__field-label" htmlFor={kindId}>
          Origem do sinal
        </label>
        <select
          id={kindId}
          className="input"
          disabled={disabled}
          value={value.origin_kind}
          onChange={(e) => {
            const origin_kind = e.target.value as OriginKind;
            onChange(origin_kind ? { origin_kind, origin_ref_id: "", origin_label: "" } : { origin_kind: "", origin_ref_id: "", origin_label: "" });
          }}
        >
          <option value="">— Sem origem definida —</option>
          {(Object.keys(ORIGIN_KIND_LABELS) as Array<Exclude<OriginKind, "">>).map((k) => (
            <option key={k} value={k}>
              {ORIGIN_KIND_LABELS[k]}
            </option>
          ))}
        </select>
      </div>
      {value.origin_kind ? (
        <div className="conn-form-modal__field">
          <label className="conn-form-modal__field-label" htmlFor={refId}>
            {ORIGIN_KIND_LABELS[value.origin_kind]} de origem
          </label>
          <select
            id={refId}
            className="input"
            disabled={disabled || loading}
            value={value.origin_ref_id}
            onChange={(e) => {
              const id = e.target.value;
              const chosen = visibleChoices.find((c) => c.id === id);
              onChange({
                ...value,
                origin_ref_id: id,
                origin_label: chosen ? chosen.label : value.origin_label,
              });
            }}
          >
            <option value="">{loading ? "A carregar…" : "— Escolher / ou preencher só o nome —"}</option>
            {visibleChoices.map((c) => (
              <option key={c.id} value={c.id}>
                {c.label}
              </option>
            ))}
          </select>
        </div>
      ) : null}
      {value.origin_kind ? (
        <div className="conn-form-modal__field">
          <label className="conn-form-modal__field-label" htmlFor={labelId}>
            Nome da origem (livre)
          </label>
          <input
            id={labelId}
            className="input"
            disabled={disabled}
            placeholder="Ex.: POP Central, Rádio Torre Norte…"
            value={value.origin_label}
            onChange={(e) => onChange({ ...value, origin_label: e.target.value })}
          />
        </div>
      ) : null}
    </>
  );
}
