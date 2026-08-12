import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createPortal } from "react-dom";
import { FileUp } from "lucide-react";
import { apiFetch } from "../lib/api";
import { queryKeys } from "../lib/queryKeys";
import {
  kmlReviewSummary,
  parseKmlOrKmzFile,
  reviewItemsToImportElements,
  type KmlReviewItem,
} from "../lib/parseKmlProject";
import type { NetworkProject } from "../lib/networkInfrastructure";
import { parseCoordInput } from "../lib/networkInfrastructure";
import { KmlImportReviewModal } from "../pages/connections/KmlImportReviewModal";
import { ConfirmModal } from "./ConfirmModal";

type ProjectOpt = { id: string; display_number: number; description: string };

type Props = {
  open: boolean;
  defaultProjectId?: string;
  projects: ProjectOpt[];
  onClose: () => void;
  onImported: (info: { replaced: boolean; projectId: string; imported: Record<string, number> }) => void;
};

function centroidOf(items: KmlReviewItem[]): { latitude: number; longitude: number } | null {
  const pts = items.filter((i) => i.include);
  if (pts.length === 0) return null;
  return {
    latitude: pts.reduce((s, p) => s + p.latitude, 0) / pts.length,
    longitude: pts.reduce((s, p) => s + p.longitude, 0) / pts.length,
  };
}

export function MapProjectKmlImport({ open, defaultProjectId, projects, onClose, onImported }: Props) {
  const qc = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const [mode, setMode] = useState<"replace" | "create">(defaultProjectId ? "replace" : "create");
  const [projectId, setProjectId] = useState(defaultProjectId ?? "");
  const [err, setErr] = useState<string | null>(null);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [confirmReplace, setConfirmReplace] = useState(false);
  const [pendingItems, setPendingItems] = useState<KmlReviewItem[] | null>(null);
  const [draft, setDraft] = useState<{
    projectName: string;
    items: KmlReviewItem[];
    skipped: number;
    fileName: string;
  } | null>(null);

  const selected = projects.find((p) => p.id === projectId);

  useEffect(() => {
    if (!open) return;
    setMode(defaultProjectId ? "replace" : "create");
    setProjectId(defaultProjectId ?? "");
    setErr(null);
  }, [open, defaultProjectId]);

  const importMut = useMutation({
    mutationFn: async (items: KmlReviewItem[]) => {
      const elements = reviewItemsToImportElements(items);
      const total =
        elements.ctos.length + elements.splice_boxes.length + elements.poles.length + elements.cables.length;
      if (total === 0) throw new Error("Nenhum elemento seleccionado para importar.");
      const center = centroidOf(items);
      if (mode === "replace") {
        if (!projectId) throw new Error("Seleccione o projeto a substituir.");
        const existing = await apiFetch<NetworkProject>(`/api/v1/commercial/network/projects/${projectId}`);
        const lat = center?.latitude ?? parseCoordInput(String(existing.latitude ?? "")) ?? existing.latitude ?? null;
        const lng = center?.longitude ?? parseCoordInput(String(existing.longitude ?? "")) ?? existing.longitude ?? null;
        return apiFetch<{ id: string; display_number: number; imported: Record<string, number>; replaced?: boolean }>(
          "/api/v1/commercial/network/projects/import/kml",
          {
            method: "POST",
            json: {
              replace_project_id: projectId,
              description: (draft?.projectName ?? "").trim() || existing.description,
              locality_id: existing.locality_id ?? null,
              color: existing.color ?? null,
              status: existing.status,
              latitude: lat,
              longitude: lng,
              elements,
            },
          },
        );
      }
      if (!center) throw new Error("O ficheiro não tem coordenadas válidas.");
      return apiFetch<{ id: string; display_number: number; imported: Record<string, number>; replaced?: boolean }>(
        "/api/v1/commercial/network/projects/import/kml",
        {
          method: "POST",
          json: {
            description: (draft?.projectName ?? "").trim() || "Projecto importado",
            status: "planejamento",
            latitude: center.latitude,
            longitude: center.longitude,
            elements,
          },
        },
      );
    },
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: queryKeys.networkProjects });
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      const replaced = mode === "replace";
      onImported({ replaced, projectId: data.id, imported: data.imported ?? {} });
      resetAndClose();
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao importar."),
  });

  function resetAndClose() {
    setErr(null);
    setReviewOpen(false);
    setConfirmReplace(false);
    setPendingItems(null);
    setDraft(null);
    onClose();
  }

  async function onFile(file: File | null) {
    if (!file) return;
    setErr(null);
    try {
      const parsed = await parseKmlOrKmzFile(file);
      if (parsed.items.length === 0) {
        throw new Error("O KML/KMZ não contém elementos reconhecidos.");
      }
      const projectName =
        parsed.projectName ||
        (mode === "replace" && selected ? selected.description : "Projecto importado");
      setDraft({
        projectName,
        items: parsed.items,
        skipped: parsed.skipped,
        fileName: file.name,
      });
      setReviewOpen(true);
    } catch (e) {
      setDraft(null);
      setReviewOpen(false);
      setErr(e instanceof Error ? e.message : "Falha ao ler o ficheiro.");
    }
  }

  if (!open) return null;

  return (
    <>
      {createPortal(
        <div className="modal-backdrop" role="presentation" onMouseDown={() => !importMut.isPending && resetAndClose()}>
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="map-kml-import-title"
            style={{ maxWidth: 480, width: "min(96vw, 480px)" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
              <h3 id="map-kml-import-title" style={{ margin: 0 }}>
                Importar KML / KMZ
              </h3>
              <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={resetAndClose} disabled={importMut.isPending}>
                ×
              </button>
            </div>
            <p style={{ margin: "0 0 12px", fontSize: 12, color: "var(--muted)" }}>
              Importe um ficheiro de projeto e, se quiser, substitua um projeto já existente. Na revisão pode alterar o
              tipo e os atributos em massa.
            </p>
            <div style={{ display: "grid", gap: 10 }}>
              <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
                <input
                  type="radio"
                  name="map-kml-mode"
                  checked={mode === "replace"}
                  onChange={() => setMode("replace")}
                  disabled={importMut.isPending}
                />
                Substituir projeto existente
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
                <input
                  type="radio"
                  name="map-kml-mode"
                  checked={mode === "create"}
                  onChange={() => setMode("create")}
                  disabled={importMut.isPending}
                />
                Criar novo projeto
              </label>
              {mode === "replace" ? (
                <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  <span style={{ fontSize: 12, color: "var(--muted)" }}>Projeto</span>
                  <select
                    className="select"
                    value={projectId}
                    onChange={(e) => setProjectId(e.target.value)}
                    disabled={importMut.isPending}
                  >
                    <option value="">— seleccionar —</option>
                    {projects.map((p) => (
                      <option key={p.id} value={p.id}>
                        Projeto {p.display_number} — {p.description}
                      </option>
                    ))}
                  </select>
                </label>
              ) : null}
              <input
                ref={fileRef}
                type="file"
                accept=".kml,.kmz,application/vnd.google-earth.kml+xml,application/vnd.google-earth.kmz,application/xml,text/xml"
                style={{ display: "none" }}
                onChange={(e) => {
                  const f = e.target.files?.[0] ?? null;
                  e.target.value = "";
                  void onFile(f);
                }}
              />
              <button
                type="button"
                className="btn btn--primary"
                disabled={importMut.isPending || (mode === "replace" && !projectId)}
                onClick={() => fileRef.current?.click()}
              >
                <FileUp size={15} style={{ marginRight: 6 }} />
                Escolher KML / KMZ
              </button>
              {draft ? (
                <div style={{ fontSize: 12, color: "var(--muted)" }}>
                  {draft.fileName} · {kmlReviewSummary(draft.items)}
                </div>
              ) : null}
              {err ? <div className="msg msg--err">{err}</div> : null}
            </div>
          </div>
        </div>,
        document.body,
      )}

      {draft ? (
        <KmlImportReviewModal
          open={reviewOpen}
          fileName={draft.fileName}
          projectName={draft.projectName}
          skipped={draft.skipped}
          initialItems={draft.items}
          confirmLabel={mode === "replace" ? "Continuar para substituir" : "Confirmar e importar"}
          warning={
            mode === "replace" && selected
              ? `Os elementos actuais de «${selected.description}» serão apagados e substituídos pelos do ficheiro.`
              : null
          }
          onCancel={() => setReviewOpen(false)}
          onConfirm={(items) => {
            setDraft({ ...draft, items });
            setReviewOpen(false);
            if (mode === "replace") {
              setPendingItems(items);
              setConfirmReplace(true);
              return;
            }
            importMut.mutate(items);
          }}
        />
      ) : null}

      {confirmReplace && pendingItems ? (
        <ConfirmModal
          open
          danger
          title="Substituir projeto"
          message={
            selected
              ? `Isto apaga CTOs, emendas, cabos e postes do projeto «${selected.description}» e importa os do ficheiro. O número do projeto mantém-se.`
              : "Isto apaga os elementos actuais do projeto e importa os do ficheiro."
          }
          confirmLabel="Substituir"
          busy={importMut.isPending}
          onCancel={() => {
            setConfirmReplace(false);
            setPendingItems(null);
          }}
          onConfirm={() => importMut.mutate(pendingItems)}
        />
      ) : null}
    </>
  );
}
