import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import type { MikrotikIfRow } from "../../lib/mikrotikNocData";

type EditableMetadata = {
  description: string;
  type: "" | "ether" | "sfp";
};

type Props = {
  deviceId: string;
  canMutate: boolean;
  onClose: () => void;
};

function interfaceName(row: MikrotikIfRow): string {
  return String(row.if_name ?? row.display_name ?? row.descr ?? `ifIndex ${row.if_index}`).trim();
}

function normalizeType(value: string | undefined): EditableMetadata["type"] {
  const valueLower = String(value ?? "").trim().toLowerCase();
  return valueLower === "ether" || valueLower === "sfp" ? valueLower : "";
}

export function DeviceEditInterfacesTab({ deviceId, canMutate, onClose }: Props) {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [metadata, setMetadata] = useState<Record<number, EditableMetadata>>({});

  const interfaces = useQuery({
    queryKey: ["device-edit-interfaces", deviceId],
    enabled: !!deviceId,
    queryFn: () =>
      apiFetch<{ interface_table?: MikrotikIfRow[]; collected_at?: string; note?: string }>(
        `/api/v1/interfaces/devices/${deviceId}`,
      ),
  });

  useEffect(() => {
    const rows = interfaces.data?.interface_table;
    if (!rows) return;
    const next: Record<number, EditableMetadata> = {};
    for (const row of rows) {
      next[row.if_index] = {
        description: String(row.custom_description ?? ""),
        type: normalizeType(row.custom_type),
      };
    }
    setMetadata(next);
  }, [interfaces.data?.interface_table]);

  const rows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const source = [...(interfaces.data?.interface_table ?? [])].sort((a, b) => a.if_index - b.if_index);
    if (!query) return source;
    return source.filter((row) => {
      const current = metadata[row.if_index];
      return `${row.if_index} ${interfaceName(row)} ${current?.description ?? ""} ${current?.type ?? ""}`
        .toLowerCase()
        .includes(query);
    });
  }, [interfaces.data?.interface_table, metadata, search]);

  const save = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/interfaces/devices/${deviceId}/metadata`, {
        method: "PUT",
        json: {
          interfaces: (interfaces.data?.interface_table ?? []).map((row) => ({
            if_index: row.if_index,
            if_name: interfaceName(row),
            description: metadata[row.if_index]?.description.trim() ?? "",
            type: metadata[row.if_index]?.type ?? "",
          })),
        },
      }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["device-edit-interfaces", deviceId] }),
        queryClient.invalidateQueries({ queryKey: ["mikrotik-if", deviceId] }),
        queryClient.invalidateQueries({ queryKey: ["switch-if", deviceId] }),
        queryClient.invalidateQueries({ queryKey: ["bng-if", deviceId] }),
      ]);
    },
  });

  const refresh = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/interfaces/devices/${deviceId}/refresh`, {
        method: "POST",
        json: {},
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["device-edit-interfaces", deviceId] }),
  });

  if (interfaces.isLoading) return <p style={{ color: "var(--muted)" }}>A carregar interfaces coletadas…</p>;
  if (interfaces.isError) return <div className="msg msg--err">{(interfaces.error as Error).message}</div>;

  const total = interfaces.data?.interface_table?.length ?? 0;
  return (
    <>
      <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
        As interfaces vêm da última coleta SNMP. A descrição e o tipo abaixo são personalizados no NetQuasar e não
        alteram a configuração do equipamento.
      </p>

      <div className="row" style={{ gap: 8, marginBottom: 10, alignItems: "center", flexWrap: "wrap" }}>
        <input
          className="input"
          style={{ flex: "1 1 260px" }}
          placeholder="Pesquisar interface ou descrição"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <span style={{ color: "var(--muted)", fontSize: 12 }}>{total} interfaces</span>
        <button
          type="button"
          className="btn"
          disabled={!canMutate || refresh.isPending}
          onClick={() => refresh.mutate()}
        >
          {refresh.isPending ? "Coletando…" : "Atualizar coleta"}
        </button>
      </div>

      {refresh.isError && <div className="msg msg--err">{(refresh.error as Error).message}</div>}
      {save.isError && <div className="msg msg--err">{(save.error as Error).message}</div>}
      {save.isSuccess && <div className="msg msg--ok">Descrições e tipos salvos.</div>}

      {total === 0 ? (
        <p style={{ color: "var(--muted)", fontSize: 13 }}>
          Ainda não há interfaces coletadas. Clique em <strong>Atualizar coleta</strong>.
        </p>
      ) : (
        <div className="table-wrap" style={{ maxHeight: 440, overflow: "auto" }}>
          <table style={{ width: "100%", fontSize: 12 }}>
            <thead>
              <tr>
                <th style={{ width: 80 }}>Idx</th>
                <th>Interface coletada</th>
                <th>Descrição</th>
                <th style={{ width: 130 }}>Tipo</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const current = metadata[row.if_index] ?? { description: "", type: "" };
                return (
                  <tr key={row.if_index}>
                    <td className="mono">{row.if_index}</td>
                    <td className="mono">{interfaceName(row)}</td>
                    <td>
                      <input
                        className="input"
                        style={{ width: "100%" }}
                        maxLength={120}
                        placeholder="Ex.: Link principal"
                        value={current.description}
                        onChange={(event) =>
                          setMetadata((previous) => ({
                            ...previous,
                            [row.if_index]: { ...current, description: event.target.value.slice(0, 120) },
                          }))
                        }
                      />
                    </td>
                    <td>
                      <select
                        className="select"
                        style={{ width: "100%" }}
                        value={current.type}
                        onChange={(event) =>
                          setMetadata((previous) => ({
                            ...previous,
                            [row.if_index]: {
                              ...current,
                              type: normalizeType(event.target.value),
                            },
                          }))
                        }
                      >
                        <option value="">Automático</option>
                        <option value="ether">Ether</option>
                        <option value="sfp">SFP</option>
                      </select>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <div className="row" style={{ marginTop: 12 }}>
        <button type="button" className="btn" onClick={onClose}>
          Cancelar
        </button>
        <button
          type="button"
          className="btn btn--primary"
          disabled={!canMutate || save.isPending || total === 0}
          onClick={() => save.mutate()}
        >
          {save.isPending ? "Salvando…" : "Salvar interfaces"}
        </button>
      </div>
    </>
  );
}
