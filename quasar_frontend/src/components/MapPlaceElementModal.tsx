import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { CABLE_FIBER_COUNTS } from "../lib/fiberSplitter";
import { FiberColorSelect, LocalitySelect, ProjectSelect } from "../pages/connections/ConnectionsFormFields";
import { INFRA_MAP_KIND_LABELS, type InfraMapKind } from "../lib/mapInfrastructureIcons";
import { queryKeys } from "../lib/queryKeys";
import type { CommercialLocality, NetworkProject } from "../lib/networkInfrastructure";

export type PlaceableKind = InfraMapKind;

export type MapPlaceSession =
  | { mode: "create"; kind: PlaceableKind; lat: number; lng: number }
  | { mode: "create-cable-path"; path: { lat: number; lng: number }[] }
  | { mode: "edit-cable"; cableId: string; lat: number; lng: number; initialDescription?: string };

type Props = {
  session: MapPlaceSession | null;
  onClose: () => void;
  onSaved: (info: { kind: PlaceableKind; id: string; lat: number; lng: number }) => void;
};

const API: Record<PlaceableKind, string> = {
  cto: "/api/v1/commercial/network/ctos",
  cable: "/api/v1/commercial/network/cables",
  splice_box: "/api/v1/commercial/network/splice-boxes",
  pole: "/api/v1/commercial/network/poles",
  project: "/api/v1/commercial/network/projects",
  pop: "/api/v1/pops",
};

const SPLITTERS = ["1x2", "1x4", "1x8", "1x16", "1x32", "1x64"];

function sessionKind(session: MapPlaceSession): PlaceableKind {
  if (session.mode === "edit-cable" || session.mode === "create-cable-path") return "cable";
  return session.kind;
}

function sessionCoords(session: MapPlaceSession): { lat: number; lng: number } {
  if (session.mode === "create-cable-path") {
    return { lat: session.path[0].lat, lng: session.path[0].lng };
  }
  return { lat: session.lat, lng: session.lng };
}

export function MapPlaceElementModal({ session, onClose, onSaved }: Props) {
  const qc = useQueryClient();
  const kind = session ? sessionKind(session) : null;
  const coords = session ? sessionCoords(session) : null;
  const needsProject = kind != null && kind !== "project" && kind !== "pop";
  const needsLocality = kind === "pop";
  const projectRequired = (session?.mode === "create" || session?.mode === "create-cable-path") && needsProject;

  const [description, setDescription] = useState("");
  const [projectId, setProjectId] = useState("");
  const [localityId, setLocalityId] = useState("");
  const [splitter, setSplitter] = useState("1x8");
  const [fiberColor, setFiberColor] = useState("Desconhecido");
  const [fiberCount, setFiberCount] = useState("12");
  const [cableType, setCableType] = useState("");
  const [boxModel, setBoxModel] = useState("emenda");
  const [projectColor, setProjectColor] = useState("#2563eb");
  const [err, setErr] = useState<string | null>(null);

  const projectsQ = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: () => apiFetch<{ projects: NetworkProject[] }>("/api/v1/commercial/network/projects"),
    enabled: !!session && needsProject,
  });
  const projects = (projectsQ.data?.projects ?? []).filter((p) => p.status !== "inativo");

  const localitiesQ = useQuery({
    queryKey: queryKeys.commercialLocalities,
    queryFn: () => apiFetch<{ localities: CommercialLocality[] }>("/api/v1/commercial/localities"),
    enabled: !!session && needsLocality,
  });
  const localities = localitiesQ.data?.localities ?? [];

  useEffect(() => {
    if (!session) return;
    setDescription(session.mode === "edit-cable" ? session.initialDescription ?? "" : "");
    setProjectId("");
    setLocalityId("");
    setSplitter("1x8");
    setFiberColor("Desconhecido");
    setFiberCount("12");
    setCableType("");
    setBoxModel("emenda");
    setProjectColor("#2563eb");
    setErr(null);
  }, [session]);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (!session || !kind || !coords) throw new Error("Sessão inválida.");
      const desc = description.trim();
      if (!desc) throw new Error("Informe o nome / descrição.");
      if (projectRequired && !projectId.trim()) throw new Error("Seleccione o projeto.");

      if (session.mode === "edit-cable") {
        await apiFetch(`${API.cable}/${session.cableId}`, {
          method: "PATCH",
          json: {
            description: desc,
            cable_type: cableType.trim() || null,
            fiber_count: Number(fiberCount) || null,
            status: "ativo",
            ...(projectId.trim() ? { project_id: projectId.trim() } : {}),
          },
        });
        return { kind: "cable" as const, id: session.cableId, lat: coords.lat, lng: coords.lng };
      }

      const payload: Record<string, unknown> = {
        description: desc,
        latitude: coords.lat,
        longitude: coords.lng,
      };
      if (needsProject) payload.project_id = projectId.trim();
      if (kind === "pop" && localityId.trim()) payload.locality_id = localityId.trim();

      if (kind === "cto") {
        payload.fiber_color = fiberColor.trim() || "Desconhecido";
        payload.splitter = splitter.trim() || null;
      }
      if (kind === "splice_box") {
        payload.box_model = boxModel;
        payload.fiber_count = Number(fiberCount) || null;
        if (boxModel === "distribuicao") {
          payload.splitter = splitter.trim() || null;
          payload.fiber_color = fiberColor.trim() || "Desconhecido";
        }
      }
      if (kind === "cable") {
        payload.cable_type = cableType.trim() || null;
        payload.fiber_count = Number(fiberCount) || null;
        payload.status = "ativo";
        if (session.mode === "create-cable-path") {
          payload.path = session.path.map((p) => ({ lat: p.lat, lng: p.lng }));
        }
      }
      if (kind === "project") {
        payload.color = projectColor.trim() || null;
        payload.status = "planejamento";
      }

      const created = await apiFetch<{ id: string }>(API[kind], { method: "POST", json: payload });
      return { kind, id: created.id, lat: coords.lat, lng: coords.lng };
    },
    onSuccess: async (res) => {
      await qc.invalidateQueries({ queryKey: ["map-infrastructure-points"] });
      if (res.kind === "pop") await qc.invalidateQueries({ queryKey: queryKeys.pops });
      onSaved(res);
    },
    onError: (e) => setErr(e instanceof Error ? e.message : "Falha ao guardar."),
  });

  if (!session || !kind || !coords) return null;

  const title =
    session.mode === "edit-cable"
      ? "Editar cabo"
      : session.mode === "create-cable-path"
        ? "Novo cabo"
        : `Nova ${INFRA_MAP_KIND_LABELS[kind] ?? kind}`;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div className="modal map-place-modal" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <h3 style={{ marginTop: 0 }}>{title}</h3>
        <p style={{ margin: "0 0 10px", fontSize: 12, color: "var(--muted)" }}>
          Posição:{" "}
          <span className="mono">
            {coords.lat.toFixed(6)}, {coords.lng.toFixed(6)}
          </span>
          {session.mode === "create-cable-path" ? (
            <>
              {" "}
              · {session.path.length} pontos no trajeto
            </>
          ) : null}
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          <label className="splitter-modal__field">
            <span>Nome / descrição</span>
            <input
              className="input"
              autoFocus
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={
                kind === "cable"
                  ? "Ex.: Cabo backbone setor A"
                  : kind === "pop"
                    ? "Ex.: POP Miracema"
                    : "Ex.: CTO Rua das Flores"
              }
            />
          </label>
          {needsProject ? (
            <ProjectSelect value={projectId} projects={projects} onChange={setProjectId} />
          ) : null}
          {needsLocality ? (
            <LocalitySelect value={localityId} localities={localities} onChange={setLocalityId} />
          ) : null}
          {kind === "cto" || (kind === "splice_box" && boxModel === "distribuicao") ? (
            <>
              <label className="splitter-modal__field">
                <span>Splitter</span>
                <select className="select" value={splitter} onChange={(e) => setSplitter(e.target.value)}>
                  {SPLITTERS.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
              </label>
              <FiberColorSelect value={fiberColor} onChange={setFiberColor} />
            </>
          ) : null}
          {kind === "splice_box" ? (
            <>
              <label className="splitter-modal__field">
                <span>Modelo</span>
                <select className="select" value={boxModel} onChange={(e) => setBoxModel(e.target.value)}>
                  <option value="emenda">Emenda</option>
                  <option value="distribuicao">Distribuição</option>
                </select>
              </label>
              {boxModel === "emenda" ? (
                <label className="splitter-modal__field">
                  <span>Quantidade de fibras</span>
                  <select className="select" value={fiberCount} onChange={(e) => setFiberCount(e.target.value)}>
                    {CABLE_FIBER_COUNTS.map((n) => (
                      <option key={n} value={String(n)}>
                        {n} fibras
                      </option>
                    ))}
                  </select>
                </label>
              ) : null}
            </>
          ) : null}
          {kind === "cable" ? (
            <>
              <label className="splitter-modal__field">
                <span>Tipo do cabo</span>
                <input
                  className="input"
                  value={cableType}
                  onChange={(e) => setCableType(e.target.value)}
                  placeholder="Ex.: AS80, drop, backbone…"
                />
              </label>
              <label className="splitter-modal__field">
                <span>Quantidade de fibras</span>
                <select className="select" value={fiberCount} onChange={(e) => setFiberCount(e.target.value)}>
                  {CABLE_FIBER_COUNTS.map((n) => (
                    <option key={n} value={String(n)}>
                      {n} fibras
                    </option>
                  ))}
                </select>
              </label>
            </>
          ) : null}
          {kind === "project" ? (
            <label className="splitter-modal__field">
              <span>Cor no mapa</span>
              <input className="input" type="color" value={projectColor} onChange={(e) => setProjectColor(e.target.value)} />
            </label>
          ) : null}
          {err ? <div className="msg msg--err">{err}</div> : null}
        </div>
        <div className="row" style={{ justifyContent: "flex-end", marginTop: 14, gap: 8 }}>
          <button type="button" className="btn" onClick={onClose} disabled={saveMut.isPending}>
            Cancelar
          </button>
          <button type="button" className="btn btn--primary" disabled={saveMut.isPending} onClick={() => saveMut.mutate()}>
            {saveMut.isPending ? "A guardar…" : "Guardar"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
