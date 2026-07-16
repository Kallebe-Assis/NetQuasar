import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useRef, useState } from "react";
import { Copy, Pencil, Plus } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import {
  MikrotikCollectionPanel,
  type MikrotikCollectionHandle,
  MIKROTIK_SNMP_SECTION_ORDER,
} from "./MikrotikCollectionPanel";
import {
  MikrotikTelnetProfilesPanel,
  type MikrotikTelnetProfilesHandle,
  type TelnetProfile,
  MIKROTIK_TELNET_SECTION_ORDER,
} from "./MikrotikTelnetProfilesPanel";

type SnmpCollectionResponse = {
  metrics: Record<string, { enabled?: boolean; oid?: string }>;
  catalog: Array<{ key: string; section: string }>;
  sections: Record<string, string>;
};

type TelnetProfilesResponse = {
  profiles: TelnetProfile[];
  catalog: Array<{ key: string; section: string; default_command: string }>;
  sections: Record<string, string>;
};

type EditTarget =
  | { kind: "snmp" }
  | { kind: "telnet"; profileId: string };

export function MikrotikSettingsPanel() {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const snmpRef = useRef<MikrotikCollectionHandle>(null);
  const telnetRef = useRef<MikrotikTelnetProfilesHandle>(null);

  const [editTarget, setEditTarget] = useState<EditTarget | null>(null);
  const [editSection, setEditSection] = useState("geral");
  const [savePending, setSavePending] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [copyOpen, setCopyOpen] = useState(false);
  const [copyName, setCopyName] = useState("");

  const snmp = useQuery({
    queryKey: ["mikrotik-collection"],
    queryFn: () => apiFetch<SnmpCollectionResponse>("/api/v1/settings/mikrotik-collection"),
  });

  const telnet = useQuery({
    queryKey: ["mikrotik-telnet-profiles"],
    queryFn: () => apiFetch<TelnetProfilesResponse>("/api/v1/settings/mikrotik-telnet-profiles"),
  });

  const snmpEnabled = useMemo(() => {
    const catalog = snmp.data?.catalog ?? [];
    const metrics = snmp.data?.metrics ?? {};
    let n = 0;
    for (const e of catalog) {
      if (metrics[e.key]?.enabled) n++;
    }
    return n;
  }, [snmp.data]);

  const profiles = telnet.data?.profiles ?? [];
  const editingTelnet = useMemo(() => {
    if (editTarget?.kind !== "telnet") return null;
    return profiles.find((p) => p.id === editTarget.profileId) ?? null;
  }, [editTarget, profiles]);

  const snmpNav = useMemo(() => {
    const sections = snmp.data?.sections ?? {};
    const catalog = snmp.data?.catalog ?? [];
    const present = MIKROTIK_SNMP_SECTION_ORDER.filter((s) => catalog.some((c) => c.section === s)).map(
      (s) => ({ id: s, label: sections[s] || s }),
    );
    return [{ id: "geral", label: "Geral" }, ...present, { id: "advanced", label: "Avançado" }];
  }, [snmp.data]);

  const telnetNav = useMemo(() => {
    const sections = telnet.data?.sections ?? {};
    const catalog = telnet.data?.catalog ?? [];
    const present = MIKROTIK_TELNET_SECTION_ORDER.filter((s) => catalog.some((c) => c.section === s)).map(
      (s) => ({ id: s, label: sections[s] || s }),
    );
    return [{ id: "geral", label: "Geral" }, ...present];
  }, [telnet.data]);

  const create = useMutation({
    mutationFn: (name: string) => {
      const catalog = telnet.data?.catalog ?? [];
      const metrics: Record<string, { enabled: boolean; command: string }> = {};
      for (const e of catalog) {
        metrics[e.key] = { enabled: false, command: e.default_command };
      }
      return apiFetch<TelnetProfile>("/api/v1/settings/mikrotik-telnet-profiles", {
        method: "POST",
        json: { name, metrics, pre_commands: [] },
      });
    },
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["mikrotik-telnet-profiles"] });
      setCreateOpen(false);
      setCreateName("");
      toastOk(pushToast, `Perfil «${p.name}» criado.`);
      setEditSection("geral");
      setEditTarget({ kind: "telnet", profileId: p.id });
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao criar perfil."),
  });

  function openSnmpEdit() {
    setSavePending(false);
    setEditSection("geral");
    setEditTarget({ kind: "snmp" });
  }

  function openTelnetEdit(profileId: string) {
    setSavePending(false);
    setCopyOpen(false);
    setEditSection("geral");
    setEditTarget({ kind: "telnet", profileId });
  }

  function closeEdit() {
    if (editTarget?.kind === "snmp") snmpRef.current?.reloadFromServer();
    else if (editTarget?.kind === "telnet") telnetRef.current?.reloadFromServer();
    setEditTarget(null);
    setCopyOpen(false);
    setSavePending(false);
  }

  if (snmp.isLoading || telnet.isLoading) return <p>A carregar perfis MikroTik…</p>;
  if (snmp.isError) return <div className="msg msg--err">{(snmp.error as Error).message}</div>;
  if (telnet.isError) return <div className="msg msg--err">{(telnet.error as Error).message}</div>;

  const snmpStatus =
    snmpEnabled > 0
      ? `${snmpEnabled} métrica${snmpEnabled === 1 ? "" : "s"} activa${snmpEnabled === 1 ? "" : "s"}`
      : "Inactivo";

  return (
    <>
      <div className="olt-profiles-layout">
        <div className="card">
          <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8 }}>
            <div>
              <h2 style={{ margin: 0 }}>Perfis MikroTik</h2>
              <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 4, marginBottom: 0 }}>
                Coleta SNMP global e perfis Telnet nomeados. Credenciais em <strong>Rede e SNMP</strong>.
              </p>
            </div>
            <button type="button" className="btn btn--primary" onClick={() => setCreateOpen(true)}>
              <Plus size={16} aria-hidden /> Novo Perfil
            </button>
          </div>

          <div className="table-wrap" style={{ marginTop: 12 }}>
            <table className="olt-profiles-table">
              <thead>
                <tr>
                  <th>Descrição</th>
                  <th>Marca/Tipo</th>
                  <th>Modelo</th>
                  <th>Status</th>
                  <th style={{ width: 110 }}>Ações</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>Coleta SNMP global</td>
                  <td>SNMP</td>
                  <td className="mono">RouterOS</td>
                  <td>
                    <span className={snmpEnabled > 0 ? "badge badge--ok" : "badge"}>{snmpStatus}</span>
                  </td>
                  <td>
                    <div className="olt-profiles-table__actions">
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Editar"
                        aria-label="Editar coleta SNMP MikroTik"
                        onClick={openSnmpEdit}
                      >
                        <Pencil size={14} aria-hidden />
                      </button>
                    </div>
                  </td>
                </tr>
                {profiles.map((p) => {
                  const enabled = Object.values(p.metrics ?? {}).filter((m) => m?.enabled).length;
                  return (
                    <tr key={p.id}>
                      <td>
                        {p.name}
                        {p.is_default ? " (padrão)" : ""}
                      </td>
                      <td>Telnet</td>
                      <td className="mono">RouterOS</td>
                      <td>
                        <span className={enabled > 0 ? "badge badge--ok" : "badge"}>
                          {enabled > 0
                            ? `${enabled} métrica${enabled === 1 ? "" : "s"}`
                            : "Inactivo"}
                        </span>
                      </td>
                      <td>
                        <div className="olt-profiles-table__actions">
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Editar"
                            aria-label={"Editar " + p.name}
                            onClick={() => openTelnetEdit(p.id)}
                          >
                            <Pencil size={14} aria-hidden />
                          </button>
                          <button
                            type="button"
                            className="btn btn--icon"
                            title="Copiar perfil"
                            aria-label={"Copiar " + p.name}
                            onClick={() => {
                              openTelnetEdit(p.id);
                              setCopyName(`${p.name} (cópia)`);
                              setCopyOpen(true);
                            }}
                          >
                            <Copy size={14} aria-hidden />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
                {profiles.length === 0 && (
                  <tr>
                    <td colSpan={5} style={{ color: "var(--muted)" }}>
                      Nenhum perfil telnet — use «Novo Perfil» para criar.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {createOpen && (
        <div
          className="modal-backdrop"
          role="presentation"
          onClick={() => {
            setCreateOpen(false);
            setCreateName("");
          }}
        >
          <div className="modal" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
            <h3 style={{ margin: "0 0 8px", fontSize: 16 }}>Novo perfil telnet</h3>
            <div className="field" style={{ margin: 0 }}>
              <label style={{ fontSize: 11 }}>Nome</label>
              <input
                className="input"
                autoFocus
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && createName.trim()) create.mutate(createName.trim());
                }}
              />
            </div>
            <div className="row" style={{ gap: 8, justifyContent: "flex-end", marginTop: 12 }}>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  setCreateOpen(false);
                  setCreateName("");
                }}
              >
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                disabled={!createName.trim() || create.isPending}
                onClick={() => create.mutate(createName.trim())}
              >
                {create.isPending ? "A criar…" : "Criar"}
              </button>
            </div>
          </div>
        </div>
      )}

      {editTarget?.kind === "snmp" && (
        <div
          className="modal-backdrop olt-profile-modal-backdrop"
          role="presentation"
          onClick={closeEdit}
        >
          <div
            className="olt-profile-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="mk-snmp-edit-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="olt-profile-modal__header">
              <div>
                <h2 id="mk-snmp-edit-title" style={{ margin: 0 }}>
                  Editar coleta SNMP
                </h2>
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "4px 0 0" }}>
                  MikroTik / RouterOS
                </p>
              </div>
              <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
                <button type="button" className="btn" onClick={closeEdit}>
                  Cancelar
                </button>
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={savePending}
                  onClick={() => snmpRef.current?.save()}
                >
                  {savePending ? "A guardar…" : "Guardar"}
                </button>
              </div>
            </div>

            <div className="olt-profile-modal__body" style={{ gridTemplateColumns: "200px minmax(0, 1fr)" }}>
              <nav className="olt-profile-modal__nav" aria-label="Secções do perfil">
                <div className="olt-profile-modal__nav-list">
                  {snmpNav.map((sec) => (
                    <button
                      key={sec.id}
                      type="button"
                      className={
                        "olt-profile-modal__nav-btn" +
                        (editSection === sec.id ? " olt-profile-modal__nav-btn--active" : "")
                      }
                      onClick={() => setEditSection(sec.id)}
                    >
                      {sec.label}
                    </button>
                  ))}
                </div>
              </nav>
              <div className="olt-profile-modal__main">
                <MikrotikCollectionPanel
                  ref={snmpRef}
                  variant="modal"
                  activeSection={editSection}
                  embedded
                  onPendingChange={setSavePending}
                  onSaved={() => qc.invalidateQueries({ queryKey: ["mikrotik-collection"] })}
                />
              </div>
            </div>
          </div>
        </div>
      )}

      {editTarget?.kind === "telnet" && (
        <div
          className="modal-backdrop olt-profile-modal-backdrop"
          role="presentation"
          onClick={closeEdit}
        >
          <div
            className="olt-profile-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="mk-telnet-edit-title"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="olt-profile-modal__header">
              <div>
                <h2 id="mk-telnet-edit-title" style={{ margin: 0 }}>
                  Editar perfil Telnet
                </h2>
                <p style={{ fontSize: 12, color: "var(--muted)", margin: "4px 0 0" }}>
                  {editingTelnet?.name ?? "…"}
                  {editingTelnet?.is_default ? " (padrão)" : ""}
                </p>
              </div>
              <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
                <button
                  type="button"
                  className="btn btn--ghost"
                  title="Copiar perfil"
                  onClick={() => {
                    setCopyName(`${editingTelnet?.name ?? "Perfil"} (cópia)`);
                    setCopyOpen(true);
                  }}
                >
                  <Copy size={14} aria-hidden /> Copiar
                </button>
                <button type="button" className="btn" onClick={closeEdit}>
                  Cancelar
                </button>
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={savePending}
                  onClick={() => telnetRef.current?.save()}
                >
                  {savePending ? "A guardar…" : "Guardar"}
                </button>
              </div>
            </div>

            {copyOpen && (
              <div className="card" style={{ padding: 12, flexShrink: 0 }}>
                <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>Copiar perfil</h3>
                <div className="field" style={{ margin: 0, maxWidth: 320 }}>
                  <label style={{ fontSize: 11 }}>Nome da cópia</label>
                  <input className="input" value={copyName} onChange={(e) => setCopyName(e.target.value)} />
                </div>
                <div style={{ marginTop: 8, display: "flex", gap: 8 }}>
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={!copyName.trim()}
                    onClick={() => {
                      telnetRef.current?.copy(copyName.trim());
                      setCopyOpen(false);
                    }}
                  >
                    Copiar
                  </button>
                  <button type="button" className="btn btn--ghost" onClick={() => setCopyOpen(false)}>
                    Cancelar
                  </button>
                </div>
              </div>
            )}

            <div className="olt-profile-modal__body" style={{ gridTemplateColumns: "200px minmax(0, 1fr)" }}>
              <nav className="olt-profile-modal__nav" aria-label="Secções do perfil">
                <div className="olt-profile-modal__nav-list">
                  {telnetNav.map((sec) => (
                    <button
                      key={sec.id}
                      type="button"
                      className={
                        "olt-profile-modal__nav-btn" +
                        (editSection === sec.id ? " olt-profile-modal__nav-btn--active" : "")
                      }
                      onClick={() => setEditSection(sec.id)}
                    >
                      {sec.label}
                    </button>
                  ))}
                </div>
                {editingTelnet && !editingTelnet.is_default && (
                  <button
                    type="button"
                    className="btn btn--danger olt-profile-modal__nav-delete"
                    onClick={() => telnetRef.current?.remove()}
                  >
                    Excluir perfil
                  </button>
                )}
              </nav>
              <div className="olt-profile-modal__main">
                <MikrotikTelnetProfilesPanel
                  ref={telnetRef}
                  variant="modal"
                  profileId={editTarget.profileId}
                  activeSection={editSection}
                  onPendingChange={setSavePending}
                  onSaved={(copied) => {
                    qc.invalidateQueries({ queryKey: ["mikrotik-telnet-profiles"] });
                    if (copied?.id) {
                      setEditTarget({ kind: "telnet", profileId: copied.id });
                    }
                  }}
                  onDeleted={() => {
                    setEditTarget(null);
                    qc.invalidateQueries({ queryKey: ["mikrotik-telnet-profiles"] });
                  }}
                />
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
