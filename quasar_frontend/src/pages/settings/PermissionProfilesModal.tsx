import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Plus, Shield } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import {
  DEFAULT_USER_PERMISSIONS,
  groupPermissionsByModule,
  type PermissionDefinition,
  type PermissionProfile,
} from "../../lib/permissions";
import { ActionMenu } from "../../components/ActionMenu";

type CatalogResponse = { permissions: PermissionDefinition[] };
type ProfilesResponse = { profiles: PermissionProfile[] };

type Props = {
  open: boolean;
  onClose: () => void;
};

function PermSwitch({
  id,
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  id: string;
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  hint?: string;
  disabled?: boolean;
}) {
  return (
    <label className="toggle" htmlFor={id} style={{ alignItems: "flex-start", opacity: disabled ? 0.55 : 1 }}>
      <span className="toggle__track" style={{ marginTop: 1 }}>
        <input
          id={id}
          type="checkbox"
          role="switch"
          className="toggle__input"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="toggle__thumb" aria-hidden />
      </span>
      <span style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
        <span className="toggle__label" style={{ color: "var(--text)", fontSize: 13 }}>
          {label}
        </span>
        {hint ? (
          <span className="mono" style={{ color: "var(--muted)", fontSize: 11 }}>
            {hint}
          </span>
        ) : null}
      </span>
    </label>
  );
}

export function PermissionProfilesModal({ open, onClose }: Props) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [editing, setEditing] = useState<PermissionProfile | null>(null);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmDelete, setConfirmDelete] = useState<PermissionProfile | null>(null);

  const catalogQ = useQuery({
    queryKey: ["permission-catalog"],
    enabled: open,
    queryFn: () => apiFetch<CatalogResponse>("/api/v1/settings/permissions"),
    staleTime: 60_000,
  });
  const profilesQ = useQuery({
    queryKey: ["permission-profiles"],
    enabled: open,
    queryFn: () => apiFetch<ProfilesResponse>("/api/v1/settings/permission-profiles"),
  });

  const catalog = catalogQ.data?.permissions?.length ? catalogQ.data.permissions : undefined;
  const modules = useMemo(() => groupPermissionsByModule(catalog), [catalog]);

  useEffect(() => {
    if (!open) {
      setEditing(null);
      setCreating(false);
      setConfirmDelete(null);
    }
  }, [open]);

  const startCreate = () => {
    setCreating(true);
    setEditing(null);
    setName("");
    setDescription("");
    setSelected(new Set(DEFAULT_USER_PERMISSIONS));
  };

  const startEdit = (p: PermissionProfile) => {
    setCreating(false);
    setEditing(p);
    setName(p.name);
    setDescription(p.description ?? "");
    setSelected(new Set(p.permissions.filter((x) => x !== "*")));
  };

  const cancelEditor = () => {
    setCreating(false);
    setEditing(null);
  };

  const setPerm = (key: string, on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });
  };

  const setModule = (keys: string[], on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      for (const k of keys) {
        if (on) next.add(k);
        else next.delete(k);
      }
      return next;
    });
  };

  const save = useMutation({
    mutationFn: async () => {
      const body = {
        name: name.trim(),
        description: description.trim() || null,
        permissions: [...selected],
      };
      if (creating) {
        return apiFetch<PermissionProfile>("/api/v1/settings/permission-profiles", {
          method: "POST",
          json: body,
        });
      }
      if (!editing) throw new Error("Nenhum perfil seleccionado");
      return apiFetch<PermissionProfile>(`/api/v1/settings/permission-profiles/${editing.id}`, {
        method: "PATCH",
        json: body,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["permission-profiles"] });
      void qc.invalidateQueries({ queryKey: ["settings-users"] });
      toastOk(pushToast, creating ? "Perfil criado." : "Perfil actualizado.");
      cancelEditor();
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao guardar perfil."),
  });

  const del = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/settings/permission-profiles/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["permission-profiles"] });
      setConfirmDelete(null);
      toastOk(pushToast, "Perfil removido.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao remover perfil."),
  });

  if (!open) return null;

  const editorOpen = creating || !!editing;
  const isAdminSystem = editing?.slug === "admin";
  const profiles = profilesQ.data?.profiles ?? [];

  return (
    <div className="modal-backdrop" style={{ zIndex: 80 }} onClick={() => !save.isPending && !del.isPending && onClose()}>
      <div
        className="card"
        style={{ width: "min(960px, 96vw)", maxHeight: "90vh", margin: "4vh auto", display: "flex", flexDirection: "column" }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "center", gap: 12, marginBottom: 8 }}>
          <div>
            <h2 style={{ margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
              <Shield size={18} aria-hidden /> Perfis de permissão
            </h2>
            <p style={{ margin: "6px 0 0", color: "var(--muted)", fontSize: 13 }}>
              Active ou desactive cada tela e cada função do perfil. Os perfis <strong>Administrador</strong> e{" "}
              <strong>Usuário</strong> vêm de base; pode criar outros.
            </p>
          </div>
          <div className="row" style={{ gap: 8 }}>
            {!editorOpen ? (
              <button type="button" className="btn btn--primary" onClick={startCreate}>
                <Plus size={16} aria-hidden /> Novo perfil
              </button>
            ) : null}
            <button type="button" className="btn" disabled={save.isPending || del.isPending} onClick={onClose}>
              Fechar
            </button>
          </div>
        </div>

        {profilesQ.isLoading || catalogQ.isLoading ? <p>A carregar…</p> : null}
        {profilesQ.isError ? <div className="msg msg--err">{(profilesQ.error as Error).message}</div> : null}

        {!editorOpen ? (
          <div className="table-wrap" style={{ overflow: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>Nome</th>
                  <th>Descrição</th>
                  <th>Permissões</th>
                  <th>Tipo</th>
                  <th style={{ width: 56 }} />
                </tr>
              </thead>
              <tbody>
                {profiles.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ color: "var(--muted)" }}>
                      Nenhum perfil encontrado.
                    </td>
                  </tr>
                ) : (
                  profiles.map((p) => (
                    <tr key={p.id}>
                      <td>{p.name}</td>
                      <td style={{ color: "var(--muted)", fontSize: 12 }}>{p.description || "—"}</td>
                      <td className="mono">{p.permissions.includes("*") ? "Todas" : p.permissions.length}</td>
                      <td>{p.is_system ? <span className="badge badge--ok">Sistema</span> : <span className="badge">Custom</span>}</td>
                      <td>
                        <ActionMenu
                          title={`Opções de ${p.name}`}
                          items={[
                            {
                              id: "edit",
                              label: p.slug === "admin" ? "Ver" : "Editar",
                              onClick: () => startEdit(p),
                            },
                            ...(!p.is_system
                              ? [
                                  {
                                    id: "delete",
                                    label: "Apagar",
                                    danger: true as const,
                                    onClick: () => setConfirmDelete(p),
                                  },
                                ]
                              : []),
                          ]}
                        />
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        ) : (
          <div style={{ display: "grid", gap: 12, overflow: "auto", paddingRight: 4 }}>
            <div className="row" style={{ gap: 10, flexWrap: "wrap" }}>
              <div className="field" style={{ margin: 0, flex: "1 1 240px" }}>
                <label>Nome do perfil</label>
                <input
                  className="input"
                  value={name}
                  disabled={!!editing?.is_system && editing.slug === "admin"}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Ex.: Operações OLT"
                />
              </div>
              <div className="field" style={{ margin: 0, flex: "2 1 320px" }}>
                <label>Descrição</label>
                <input
                  className="input"
                  value={description}
                  disabled={isAdminSystem}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Resumo do acesso deste perfil"
                />
              </div>
            </div>

            {isAdminSystem ? (
              <div className="msg" style={{ margin: 0 }}>
                O perfil Administrador tem acesso total a todos os módulos e acções e não pode ser alterado.
              </div>
            ) : (
              <div style={{ display: "grid", gap: 10 }}>
                {modules.map((mod) => {
                  const keys = mod.items.map((i) => i.key);
                  const onCount = keys.filter((k) => selected.has(k)).length;
                  const allOn = onCount === keys.length && keys.length > 0;
                  const partial = onCount > 0 && !allOn;
                  return (
                    <div key={mod.module} className="card" style={{ padding: 12, margin: 0, background: "var(--panel-2, transparent)" }}>
                      <div
                        className="row"
                        style={{
                          justifyContent: "space-between",
                          alignItems: "center",
                          marginBottom: 10,
                          gap: 12,
                          paddingBottom: 8,
                          borderBottom: "1px solid var(--border)",
                        }}
                      >
                        <div>
                          <strong style={{ display: "block" }}>{mod.module_label}</strong>
                          <span style={{ fontSize: 11, color: "var(--muted)" }}>
                            Tela · {onCount}/{keys.length} funções
                            {partial ? " (parcial)" : ""}
                          </span>
                        </div>
                        <PermSwitch
                          id={`perm-mod-${mod.module}`}
                          checked={allOn}
                          onChange={(on) => setModule(keys, on)}
                          label={allOn ? "ON" : "OFF"}
                        />
                      </div>
                      <div style={{ display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))" }}>
                        {mod.items.map((item) => (
                          <PermSwitch
                            key={item.key}
                            id={`perm-${item.key}`}
                            checked={selected.has(item.key)}
                            onChange={(on) => setPerm(item.key, on)}
                            label={item.label}
                            hint={item.key}
                          />
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            <div className="row" style={{ justifyContent: "flex-end", gap: 8, position: "sticky", bottom: 0, paddingTop: 8 }}>
              <button type="button" className="btn" disabled={save.isPending} onClick={cancelEditor}>
                Cancelar
              </button>
              {!isAdminSystem ? (
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={save.isPending || !name.trim()}
                  onClick={() => save.mutate()}
                >
                  {save.isPending ? "A guardar…" : creating ? "Criar perfil" : "Salvar perfil"}
                </button>
              ) : null}
            </div>
          </div>
        )}

        {confirmDelete ? (
          <div className="modal-backdrop" style={{ zIndex: 90 }} onClick={() => !del.isPending && setConfirmDelete(null)}>
            <div className="card" style={{ width: "min(420px, 92vw)", margin: "18vh auto" }} onClick={(e) => e.stopPropagation()}>
              <h3 style={{ marginTop: 0 }}>Apagar perfil</h3>
              <p style={{ fontSize: 13, color: "var(--muted)" }}>
                Remover o perfil <strong>{confirmDelete.name}</strong>? Utilizadores atribuídos a ele precisam ser reatribuídos antes.
              </p>
              <div className="row" style={{ justifyContent: "flex-end", gap: 8 }}>
                <button type="button" className="btn" disabled={del.isPending} onClick={() => setConfirmDelete(null)}>
                  Cancelar
                </button>
                <button type="button" className="btn btn--danger" disabled={del.isPending} onClick={() => del.mutate(confirmDelete.id)}>
                  Apagar
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}
