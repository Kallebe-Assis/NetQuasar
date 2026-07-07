import { useMutation } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";

type OltOption = { id: string; description?: string | null; ip?: string | null };

type UnauthorizedEntry = {
  serial?: string;
  model?: string;
  pon?: number;
  onu?: number;
  gpon_onu?: string;
  state?: string;
  mode?: string;
  raw_line?: string;
};

type UnauthorizedResponse = {
  ok?: boolean;
  olt_description?: string;
  command?: string;
  output?: string;
  entries?: UnauthorizedEntry[];
  total?: number;
  error?: string;
};

export function OltUnauthorizedOnusTab({ canMutate, olts }: { canMutate: boolean; olts: OltOption[] }) {
  const { push: pushToast } = useAppToast();
  const [oltId, setOltId] = useState("");
  const [showRaw, setShowRaw] = useState(false);

  const selectedOlt = useMemo(() => olts.find((o) => o.id === oltId) ?? olts[0], [olts, oltId]);
  const effectiveId = oltId || selectedOlt?.id || "";

  const query = useMutation({
    mutationFn: () =>
      apiFetch<UnauthorizedResponse>(`/api/v1/olt/devices/${effectiveId}/unauthorized-onus`, {
        method: "POST",
        json: {},
        timeoutMs: 240_000,
      }),
    onSuccess: (data) => {
      if (data.ok) toastOk(pushToast, `${data.total ?? 0} ONU(s) não autorizada(s) encontrada(s).`);
      else toastErr(pushToast, new Error(data.error || "Consulta telnet falhou."), "Falha na consulta.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha na consulta telnet."),
  });

  const entries = query.data?.entries ?? [];

  return (
    <div>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Consulta ONUs não autorizadas via telnet usando o comando configurado no perfil OLT (marca/modelo). Requer credenciais
        em <strong>Rede e SNMP</strong>.
      </p>
      <div className="row stack-mobile" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
        <div className="field" style={{ margin: 0, minWidth: 200, flex: "1 1 200px" }}>
          <label>OLT</label>
          <select className="input" value={effectiveId} onChange={(e) => setOltId(e.target.value)}>
            {olts.map((o) => (
              <option key={o.id} value={o.id}>
                {o.description ?? o.id} {o.ip ? `(${o.ip})` : ""}
              </option>
            ))}
          </select>
        </div>
        <button
          type="button"
          className="btn btn--primary full-width-mobile"
          disabled={!canMutate || !effectiveId || query.isPending}
          onClick={() => query.mutate()}
        >
          <RefreshCw size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
          {query.isPending ? "Consultando…" : "Consultar ONUs não autorizadas"}
        </button>
      </div>

      {query.data?.command && (
        <p className="mono" style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
          Comando: {query.data.command}
        </p>
      )}

      {query.isError && <div className="msg msg--err">{(query.error as Error).message}</div>}

      {query.data && (
        <>
          <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, flexWrap: "wrap", gap: 8 }}>
            <strong style={{ fontSize: 13 }}>
              {query.data.olt_description ?? "OLT"} — {entries.length} entrada(s)
            </strong>
            <label className="row" style={{ gap: 6, fontSize: 12 }}>
              <input type="checkbox" checked={showRaw} onChange={(e) => setShowRaw(e.target.checked)} />
              Mostrar saída bruta
            </label>
          </div>

          {showRaw && query.data.output && (
            <pre
              className="mono"
              style={{
                fontSize: 10,
                maxHeight: 280,
                overflow: "auto",
                padding: 10,
                background: "var(--panel2)",
                borderRadius: 8,
                marginBottom: 12,
                whiteSpace: "pre-wrap",
              }}
            >
              {query.data.output}
            </pre>
          )}

          <div className="table-wrap" style={{ maxHeight: 420, overflow: "auto" }}>
            <table style={{ fontSize: 11, width: "100%" }}>
              <thead>
                <tr>
                  <th>Serial</th>
                  <th>PON</th>
                  <th>ONU</th>
                  <th>GPON ONU</th>
                  <th>Estado</th>
                  <th>Modelo</th>
                </tr>
              </thead>
              <tbody>
                {entries.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ color: "var(--muted)" }}>
                      Nenhuma ONU parseada — veja a saída bruta ou ajuste o comando no perfil OLT.
                    </td>
                  </tr>
                ) : (
                  entries.map((e, i) => (
                    <tr key={`${e.serial ?? i}-${e.pon ?? 0}-${e.onu ?? 0}`}>
                      <td className="mono">{e.serial ?? "—"}</td>
                      <td>{e.pon ?? "—"}</td>
                      <td>{e.onu ?? "—"}</td>
                      <td className="mono" style={{ fontSize: 10 }}>
                        {e.gpon_onu ?? "—"}
                      </td>
                      <td className="mono">{e.state ?? e.mode ?? "—"}</td>
                      <td>{e.model ?? "—"}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
