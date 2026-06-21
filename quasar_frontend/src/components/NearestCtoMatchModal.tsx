import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { apiFetch, downloadBlob } from "../lib/api";
import { errorMessageFromUnknown } from "../lib/apiErrors";
import { useAppToast } from "../lib/appToast";
import { buildExcelCsvBlob } from "../lib/excelCsv";
import { toastErr, toastOk } from "../lib/operationToast";
import {
  ctoLabel,
  formatDistanceMeters,
  isValidGeoPoint,
  matchLoginsToNearestCtos,
  nearestCtoMatchesToCsvRows,
  type NearestCtoMatch,
} from "../lib/nearestCtoMatch";
import type { NetworkCto } from "../lib/networkInfrastructure";
import { NETWORK_INFRA_GC_MS, NETWORK_INFRA_STALE_MS } from "../lib/networkInfraCache";
import { queryKeys } from "../lib/queryKeys";
import type { ClientConnection } from "../pages/commercial/CommercialConnectionsTab";

type Props = {
  open: boolean;
  onClose: () => void;
  canMutate: boolean;
};

export function NearestCtoMatchModal({ open, onClose, canMutate }: Props) {
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [overwriteExisting, setOverwriteExisting] = useState(false);
  const [applying, setApplying] = useState(false);

  const loginsQ = useQuery({
    queryKey: [...queryKeys.clientConnections, "nearest-cto-match"],
    queryFn: () => apiFetch<{ connections: ClientConnection[] }>("/api/v1/commercial/connections"),
    enabled: open,
  });

  const ctosQ = useQuery({
    queryKey: queryKeys.networkCtos,
    queryFn: () => apiFetch<{ ctos: NetworkCto[] }>("/api/v1/commercial/network/ctos"),
    enabled: open,
    staleTime: NETWORK_INFRA_STALE_MS,
    gcTime: NETWORK_INFRA_GC_MS,
    refetchOnWindowFocus: false,
  });

  const matches = useMemo(() => {
    const logins = loginsQ.data?.connections ?? [];
    const ctos = ctosQ.data?.ctos ?? [];
    const loginRows = logins.map((c) => ({
      id: c.id,
      display_number: c.display_number,
      login: c.login,
      client_name: c.client_name,
      cto: c.cto,
      latitude: c.latitude ?? undefined,
      longitude: c.longitude ?? undefined,
    }));
    const ctoRows = ctos.map((c) => ({
      id: c.id,
      display_number: c.display_number,
      description: c.description,
      latitude: c.latitude ?? undefined,
      longitude: c.longitude ?? undefined,
    }));
    return matchLoginsToNearestCtos(loginRows, ctoRows);
  }, [loginsQ.data?.connections, ctosQ.data?.ctos]);

  const withCoords = matches.filter((m) => isValidGeoPoint(m.login));
  const withMatch = matches.filter((m) => m.nearestCto != null);
  const ctosWithCoords = (ctosQ.data?.ctos ?? []).filter((c) =>
    isValidGeoPoint({ latitude: c.latitude ?? undefined, longitude: c.longitude ?? undefined }),
  );

  const applicable = useMemo(
    () =>
      withMatch.filter((m) => {
        if (!m.login.id || !m.nearestCto) return false;
        const suggested = ctoLabel(m.nearestCto);
        if (!suggested || suggested === "—") return false;
        if (overwriteExisting) return true;
        return !(m.login.cto ?? "").trim();
      }),
    [withMatch, overwriteExisting],
  );

  function exportCsv() {
    const stamp = new Date().toISOString().slice(0, 10);
    downloadBlob(`comparacao_login_cto_${stamp}.csv`, buildExcelCsvBlob(nearestCtoMatchesToCsvRows(matches)));
    toastOk(pushToast, "Comparação exportada.");
  }

  async function applySuggestions() {
    if (!canMutate || applicable.length === 0) return;
    setApplying(true);
    let ok = 0;
    let fail = 0;
    try {
      for (const m of applicable) {
        const id = m.login.id!;
        const cto = ctoLabel(m.nearestCto);
        try {
          await apiFetch(`/api/v1/commercial/connections/${id}`, { method: "PATCH", json: { cto } });
          ok++;
        } catch {
          fail++;
        }
      }
      await qc.invalidateQueries({ queryKey: queryKeys.clientConnections });
      if (fail === 0) {
        toastOk(pushToast, `${ok} login(s) actualizado(s) com a CTO sugerida.`);
      } else {
        pushToast({ text: `${ok} actualizado(s), ${fail} falha(s).`, tone: fail > ok ? "err" : "info" });
      }
    } catch (e) {
      toastErr(pushToast, e);
    } finally {
      setApplying(false);
    }
  }

  if (!open) return null;

  const loading = loginsQ.isPending || ctosQ.isPending;
  const error = loginsQ.error ?? ctosQ.error;

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !applying && onClose()}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        aria-labelledby="nearest-cto-title"
        style={{ maxWidth: 960, width: "min(960px, 96vw)" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 id="nearest-cto-title" style={{ marginTop: 0 }}>
          Correlacionar logins com CTO mais próxima
        </h2>
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
          Compara as coordenadas de cada login com todas as CTOs cadastradas e sugere a mais próxima (distância em linha recta).
        </p>

        {loading ? <p>A carregar logins e CTOs…</p> : null}
        {error ? <div className="msg msg--err">{errorMessageFromUnknown(error)}</div> : null}

        {!loading && !error ? (
          <>
            <div className="row" style={{ gap: 12, flexWrap: "wrap", marginBottom: 12, fontSize: 12 }}>
              <span>
                <strong>{withCoords.length}</strong> logins com coordenadas
              </span>
              <span>
                <strong>{ctosWithCoords.length}</strong> CTOs com coordenadas
              </span>
              <span>
                <strong>{withMatch.length}</strong> com CTO sugerida
              </span>
              <span>
                <strong>{matches.length - withCoords.length}</strong> logins sem coordenadas (ignorados)
              </span>
            </div>

            <div className="table-wrap" style={{ maxHeight: 420, overflow: "auto" }}>
              <table className="conn-table" style={{ fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>Login</th>
                    <th>Cliente</th>
                    <th>CTO actual</th>
                    <th>CTO sugerida</th>
                    <th>Distância</th>
                  </tr>
                </thead>
                <tbody>
                  {matches.map((m) => (
                    <MatchRow key={m.login.id ?? m.login.login ?? m.login.display_number} match={m} />
                  ))}
                </tbody>
              </table>
            </div>

            {canMutate ? (
              <label className="row" style={{ gap: 8, alignItems: "center", marginTop: 12, fontSize: 12 }}>
                <input
                  type="checkbox"
                  checked={overwriteExisting}
                  onChange={(e) => setOverwriteExisting(e.target.checked)}
                  disabled={applying}
                />
                Sobrescrever CTO já preenchida ({applicable.length} elegível{applicable.length === 1 ? "" : "eis"})
              </label>
            ) : null}
          </>
        ) : null}

        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 16, flexWrap: "wrap" }}>
          <button type="button" className="btn" onClick={onClose} disabled={applying}>
            Fechar
          </button>
          <button type="button" className="btn" disabled={loading || !!error || matches.length === 0} onClick={exportCsv}>
            Exportar comparação CSV
          </button>
          {canMutate ? (
            <button
              type="button"
              className="btn btn--primary"
              disabled={loading || !!error || applying || applicable.length === 0}
              onClick={() => void applySuggestions()}
            >
              {applying ? "A aplicar…" : `Aplicar CTO sugerida (${applicable.length})`}
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function MatchRow({ match }: { match: NearestCtoMatch }) {
  const hasCoords = isValidGeoPoint(match.login);
  const suggested = ctoLabel(match.nearestCto);
  const current = (match.login.cto ?? "").trim() || "—";
  const changed = hasCoords && suggested !== "—" && current !== "—" && current !== suggested;

  return (
    <tr style={!hasCoords ? { opacity: 0.55 } : undefined}>
      <td className="mono">{match.login.login ?? "—"}</td>
      <td>{match.login.client_name ?? "—"}</td>
      <td>{current}</td>
      <td style={changed ? { color: "var(--warning, #b45309)", fontWeight: 600 } : undefined}>
        {hasCoords ? suggested : "—"}
        {!hasCoords ? <span style={{ color: "var(--muted)", fontWeight: 400 }}> (sem coords)</span> : null}
      </td>
      <td>{formatDistanceMeters(match.distanceMeters)}</td>
    </tr>
  );
}
