import { useRef, useState } from "react";
import { createPortal } from "react-dom";
import { FolderPlus, Pencil, RotateCcw, Trash2 } from "lucide-react";
import { downloadBlob } from "../../lib/api";
import { TOPOLOGY_CONNECTION_TYPES, type TopologyConnectionType } from "../../lib/topologyConnectionTypes";
import type { TopologyDocument } from "./types";

export type TopologyProjectSummary = { id: string; name: string; updated_at: string };

type Tab = "cores" | "projetos" | "arquivo" | "risco";

type Props = {
  open: boolean;
  onClose: () => void;
  canMutate: boolean;
  colorOverrides: Partial<Record<TopologyConnectionType, string>>;
  onColorsChange: (next: Partial<Record<TopologyConnectionType, string>>) => void;
  projects: TopologyProjectSummary[];
  projectsLoading: boolean;
  activeProjectId: string | null;
  onSwitchProject: (id: string) => void;
  onCreateProject: (name: string) => Promise<void>;
  onRenameProject: (id: string, name: string) => Promise<void>;
  onDeleteProject: (id: string) => Promise<void>;
  buildExportDoc: () => TopologyDocument;
  onImportDoc: (doc: TopologyDocument) => void;
  onClearAll: () => void;
};

const CLEAR_CONFIRM_WORD = "LIMPAR";

export function TopologySettingsModal({
  open,
  onClose,
  canMutate,
  colorOverrides,
  onColorsChange,
  projects,
  projectsLoading,
  activeProjectId,
  onSwitchProject,
  onCreateProject,
  onRenameProject,
  onDeleteProject,
  buildExportDoc,
  onImportDoc,
  onClearAll,
}: Props) {
  const [tab, setTab] = useState<Tab>("cores");
  const [newProjectName, setNewProjectName] = useState("");
  const [creating, setCreating] = useState(false);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const [importError, setImportError] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  // "2 confirmações" para Limpar tudo: passo 1 é o aviso inicial (botão "Sim, quero limpar"),
  // passo 2 exige digitar a palavra de confirmação — só então o botão final fica activo.
  const [clearStep, setClearStep] = useState<"idle" | "confirming">("idle");
  const [clearWord, setClearWord] = useState("");

  if (!open) return null;

  function resetClear() {
    setClearStep("idle");
    setClearWord("");
  }

  function handleClose() {
    resetClear();
    setDeleteTargetId(null);
    setRenamingId(null);
    setImportError("");
    onClose();
  }

  async function handleCreateProject() {
    const name = newProjectName.trim();
    if (!name) return;
    setCreating(true);
    try {
      await onCreateProject(name);
      setNewProjectName("");
    } finally {
      setCreating(false);
    }
  }

  function handleExport() {
    const doc = buildExportDoc();
    doc.settings = { ...(doc.settings ?? {}), connection_colors: colorOverrides };
    const projectName = projects.find((p) => p.id === activeProjectId)?.name || "topologia";
    const safeName = projectName.normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^a-zA-Z0-9-_]+/g, "-");
    const blob = new Blob([JSON.stringify(doc, null, 2)], { type: "application/json" });
    downloadBlob(`${safeName || "topologia"}.json`, blob);
  }

  function handleImportFile(file: File) {
    setImportError("");
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const text = typeof reader.result === "string" ? reader.result : "";
        const parsed = JSON.parse(text) as TopologyDocument;
        if (!parsed || !Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) {
          throw new Error("formato inesperado");
        }
        if (Array.isArray((parsed as { groups?: unknown }).groups) === false) parsed.groups = [];
        onImportDoc(parsed);
        if (parsed.settings?.connection_colors) onColorsChange(parsed.settings.connection_colors);
      } catch {
        setImportError("Ficheiro inválido — precisa ser um .json exportado desta tela.");
      }
    };
    reader.onerror = () => setImportError("Falha ao ler o ficheiro.");
    reader.readAsText(file);
  }

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={handleClose}>
      <div className="modal modal--wide topo-settings-modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>Configurações da Topologia</h3>

        <div className="topo-settings-modal__tabs">
          <button type="button" className={`btn btn--sm${tab === "cores" ? " is-active" : ""}`} onClick={() => setTab("cores")}>
            Cores das conexões
          </button>
          <button type="button" className={`btn btn--sm${tab === "projetos" ? " is-active" : ""}`} onClick={() => setTab("projetos")}>
            Projetos
          </button>
          <button type="button" className={`btn btn--sm${tab === "arquivo" ? " is-active" : ""}`} onClick={() => setTab("arquivo")}>
            Arquivo
          </button>
          {canMutate ? (
            <button type="button" className={`btn btn--sm${tab === "risco" ? " is-active" : ""}`} onClick={() => setTab("risco")}>
              Zona de risco
            </button>
          ) : null}
        </div>

        {tab === "cores" ? (
          <div className="topo-settings-modal__section">
            <p className="muted" style={{ fontSize: 12, marginTop: 0 }}>
              Personalize a cor de cada tipo de conexão neste projeto. Fica guardado junto com o diagrama — clique em
              «Salvar» na tela principal depois de ajustar.
            </p>
            <div className="topo-settings-modal__colors">
              {TOPOLOGY_CONNECTION_TYPES.map((t) => {
                const value = colorOverrides[t.id] ?? t.color;
                return (
                  <label key={t.id} className="topo-settings-modal__color-row">
                    <span className="topo-toolbar__legend-dot" style={{ background: value }} />
                    <span style={{ flex: 1 }}>{t.label}</span>
                    <input
                      type="color"
                      value={value}
                      disabled={!canMutate}
                      onChange={(e) => onColorsChange({ ...colorOverrides, [t.id]: e.target.value })}
                    />
                    {colorOverrides[t.id] ? (
                      <button
                        type="button"
                        className="btn btn--icon"
                        title="Restaurar cor padrão"
                        disabled={!canMutate}
                        onClick={() => {
                          const next = { ...colorOverrides };
                          delete next[t.id];
                          onColorsChange(next);
                        }}
                      >
                        <RotateCcw size={13} />
                      </button>
                    ) : null}
                  </label>
                );
              })}
            </div>
          </div>
        ) : null}

        {tab === "projetos" ? (
          <div className="topo-settings-modal__section">
            <p className="muted" style={{ fontSize: 12, marginTop: 0 }}>
              Cada projeto é um diagrama independente (equipamentos, conexões e POPs próprios). Troque de projeto a
              qualquer momento — mudanças não salvas no projeto atual são perdidas ao trocar.
            </p>
            {canMutate ? (
              <div className="row" style={{ gap: 6, marginBottom: 10 }}>
                <input
                  className="input"
                  style={{ flex: 1 }}
                  placeholder="Nome do novo projeto…"
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && void handleCreateProject()}
                />
                <button type="button" className="btn btn--primary" disabled={!newProjectName.trim() || creating} onClick={() => void handleCreateProject()}>
                  <FolderPlus size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                  Criar
                </button>
              </div>
            ) : null}
            <div className="topo-settings-modal__projects">
              {projectsLoading ? <p className="muted">A carregar…</p> : null}
              {!projectsLoading && projects.length === 0 ? <p className="muted">Nenhum projeto.</p> : null}
              {projects.map((p) => (
                <div key={p.id} className={`topo-settings-modal__project-row${p.id === activeProjectId ? " is-active" : ""}`}>
                  {renamingId === p.id ? (
                    <input
                      className="input"
                      style={{ flex: 1 }}
                      autoFocus
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={async (e) => {
                        if (e.key === "Enter" && renameValue.trim()) {
                          await onRenameProject(p.id, renameValue.trim());
                          setRenamingId(null);
                        }
                        if (e.key === "Escape") setRenamingId(null);
                      }}
                    />
                  ) : (
                    <button type="button" className="topo-settings-modal__project-name" onClick={() => onSwitchProject(p.id)}>
                      {p.name}
                    </button>
                  )}
                  {canMutate && renamingId !== p.id ? (
                    <button
                      type="button"
                      className="btn btn--icon"
                      title="Renomear"
                      onClick={() => {
                        setRenamingId(p.id);
                        setRenameValue(p.name);
                      }}
                    >
                      <Pencil size={13} />
                    </button>
                  ) : null}
                  {renamingId === p.id ? (
                    <button
                      type="button"
                      className="btn btn--sm btn--primary"
                      disabled={!renameValue.trim()}
                      onClick={async () => {
                        await onRenameProject(p.id, renameValue.trim());
                        setRenamingId(null);
                      }}
                    >
                      Guardar
                    </button>
                  ) : null}
                  {canMutate && projects.length > 1 && renamingId !== p.id ? (
                    <button
                      type="button"
                      className="btn btn--icon"
                      title="Remover projeto"
                      onClick={() => setDeleteTargetId(p.id)}
                    >
                      <Trash2 size={13} />
                    </button>
                  ) : null}
                </div>
              ))}
            </div>
            {deleteTargetId ? (
              <div className="topo-settings-modal__danger-inline">
                <p style={{ margin: "0 0 8px", fontSize: 12 }}>
                  Remover «{projects.find((p) => p.id === deleteTargetId)?.name}»? Isto apaga o diagrama desse projeto
                  permanentemente.
                </p>
                <div className="row" style={{ gap: 6 }}>
                  <button type="button" className="btn btn--sm" onClick={() => setDeleteTargetId(null)}>
                    Cancelar
                  </button>
                  <button
                    type="button"
                    className="btn btn--sm btn--danger"
                    onClick={async () => {
                      await onDeleteProject(deleteTargetId);
                      setDeleteTargetId(null);
                    }}
                  >
                    Remover
                  </button>
                </div>
              </div>
            ) : null}
          </div>
        ) : null}

        {tab === "arquivo" ? (
          <div className="topo-settings-modal__section">
            <p className="muted" style={{ fontSize: 12, marginTop: 0 }}>
              Além dos projetos guardados no sistema, dá para exportar o projeto atual como um ficheiro .json (backup,
              partilhar com outra instalação…) e importar um ficheiro assim de volta.
            </p>
            <div className="row" style={{ gap: 8 }}>
              <button type="button" className="btn" onClick={handleExport}>
                Baixar projeto atual como arquivo
              </button>
              {canMutate ? (
                <button type="button" className="btn" onClick={() => fileInputRef.current?.click()}>
                  Importar de arquivo…
                </button>
              ) : null}
              <input
                ref={fileInputRef}
                type="file"
                accept=".json,application/json"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) handleImportFile(f);
                }}
              />
            </div>
            {importError ? <p className="msg msg--err" style={{ marginTop: 10, fontSize: 12 }}>{importError}</p> : null}
            <p className="muted" style={{ fontSize: 11, marginTop: 10 }}>
              Importar substitui todo o diagrama do projeto atual (equipamentos, conexões, POPs e cores) — só fica
              permanente depois de clicar em «Salvar» na tela principal.
            </p>
          </div>
        ) : null}

        {tab === "risco" && canMutate ? (
          <div className="topo-settings-modal__section">
            <div className="fleet-import-danger fleet-import-danger--compact">
              <h3 style={{ display: "flex", alignItems: "center", gap: 6 }}>Limpar tudo</h3>
              <p style={{ fontSize: 12, margin: "4px 0 8px" }}>
                Remove todos os equipamentos, conexões e POPs do projeto <strong>atual</strong>. Só fica permanente
                depois de clicar em «Salvar» — mas dentro deste projeto, não tem como desfazer depois de salvar.
              </p>
              {clearStep === "idle" ? (
                <button type="button" className="btn btn--danger btn--sm" onClick={() => setClearStep("confirming")}>
                  Limpar tudo
                </button>
              ) : (
                <div>
                  <p style={{ margin: "0 0 6px", fontSize: 12, color: "var(--err, #c44)" }}>
                    Digite <strong>{CLEAR_CONFIRM_WORD}</strong> para confirmar.
                  </p>
                  <input
                    className="input"
                    style={{ maxWidth: 160 }}
                    value={clearWord}
                    onChange={(e) => setClearWord(e.target.value)}
                    placeholder={CLEAR_CONFIRM_WORD}
                    autoComplete="off"
                  />
                  <div className="fleet-import-actions" style={{ marginTop: 6 }}>
                    <button type="button" className="btn btn--sm" onClick={resetClear}>
                      Cancelar
                    </button>
                    <button
                      type="button"
                      className="btn btn--danger btn--sm"
                      disabled={clearWord.trim().toUpperCase() !== CLEAR_CONFIRM_WORD}
                      onClick={() => {
                        onClearAll();
                        resetClear();
                        handleClose();
                      }}
                    >
                      Confirmar e limpar
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        ) : null}

        <div className="row" style={{ justifyContent: "flex-end", marginTop: 16 }}>
          <button type="button" className="btn" onClick={handleClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
