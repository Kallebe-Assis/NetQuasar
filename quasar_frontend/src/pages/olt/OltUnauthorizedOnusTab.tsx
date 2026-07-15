import { useMutation } from "@tanstack/react-query";
import { createPortal } from "react-dom";
import { useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { errorMessageFromUnknown, parseApiErrorForModal } from "../../lib/apiErrors";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";

type OltOption = {
  id: string;
  description?: string | null;
  ip?: string | null;
  pon_descriptions?: Record<string, string> | null;
};

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
  pon_descriptions?: Record<string, string>;
};

type AuthorizePreview = {
  ok?: boolean;
  pon?: number;
  onu?: number;
  serial?: string;
  gpon_onu?: string;
  vlan?: string;
  vlan_source?: string;
  allocated_onu?: boolean;
  onu_type?: string;
  name?: string;
  pon_description?: string;
  error?: string;
};

type AuthorizeResult = {
  ok: boolean;
  error?: string;
  onu?: number;
  pon?: number;
  vlan?: string;
  vlan_source?: string;
  allocated_onu?: boolean;
  commands?: string[];
  output?: string;
};

type ConfirmState = {
  entry: UnauthorizedEntry;
  preview: AuthorizePreview | null;
  previewError: string | null;
  actionError: string | null;
};

export function OltUnauthorizedOnusTab({ canMutate, olts }: { canMutate: boolean; olts: OltOption[] }) {
  const { push: pushToast } = useAppToast();
  const [oltId, setOltId] = useState("");
  const [showRaw, setShowRaw] = useState(false);
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set());
  const [authorizingKey, setAuthorizingKey] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);

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
      setHiddenKeys(new Set());
      if (data.ok) toastOk(pushToast, `${data.total ?? 0} ONU(s) não autorizada(s) encontrada(s).`);
      else toastErr(pushToast, new Error(data.error || "Consulta telnet falhou."), "Falha na consulta.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha na consulta telnet."),
  });

  const ponDescriptions = useMemo(() => {
    const fromQuery = query.data?.pon_descriptions;
    if (fromQuery && typeof fromQuery === "object") return fromQuery;
    const fromOlt = selectedOlt?.pon_descriptions;
    if (fromOlt && typeof fromOlt === "object") return fromOlt;
    return {} as Record<string, string>;
  }, [query.data?.pon_descriptions, selectedOlt?.pon_descriptions]);

  function ponDesc(pon?: number | null): string {
    if (pon == null || pon <= 0) return "";
    return String(ponDescriptions[String(pon)] ?? "").trim();
  }

  const preview = useMutation({
    mutationFn: (entry: UnauthorizedEntry) => {
      const pon = Number(entry.pon ?? 0);
      const serial = String(entry.serial ?? "").trim();
      if (pon <= 0) throw new Error("PON em falta nesta entrada.");
      if (!serial) throw new Error("Serial em falta.");
      return apiFetch<AuthorizePreview>(`/api/v1/olt/devices/${effectiveId}/onu-authorize-preview`, {
        method: "POST",
        json: {
          pon,
          onu: 0,
          serial,
          if_name: gponOnuIfName(entry),
          onu_type: String(entry.model ?? "").trim() || undefined,
        },
        timeoutMs: 120_000,
      });
    },
    onSuccess: (data) => {
      setConfirm((prev) =>
        prev
          ? {
              ...prev,
              preview: data,
              previewError: null,
              actionError: null,
            }
          : prev,
      );
    },
    onError: (err) => {
      const parsed = parseApiErrorForModal(err, "Não foi possível preparar a autorização");
      const msg = parsed.message || errorMessageFromUnknown(err);
      setConfirm((prev) => (prev ? { ...prev, preview: null, previewError: msg, actionError: null } : prev));
      toastErr(pushToast, err, "Falha ao preparar autorização.");
    },
  });

  const authorize = useMutation({
    mutationFn: ({ entry, onu }: { entry: UnauthorizedEntry; onu: number }) => {
      const pon = Number(entry.pon ?? 0);
      const serial = String(entry.serial ?? "").trim();
      if (pon <= 0) throw new Error("PON em falta nesta entrada.");
      if (!serial) throw new Error("Serial em falta.");
      if (onu <= 0) throw new Error("ID da ONU inválido.");
      return apiFetch<AuthorizeResult>(`/api/v1/olt/devices/${effectiveId}/onu-authorize`, {
        method: "POST",
        json: {
          pon,
          onu,
          serial,
          if_name: gponOnuIfName(entry),
          onu_type: String(entry.model ?? "").trim() || undefined,
        },
        timeoutMs: 300_000,
      });
    },
    onMutate: ({ entry }) => {
      setAuthorizingKey(entryKey(entry));
      setConfirm((prev) => (prev ? { ...prev, actionError: null } : prev));
    },
    onSuccess: (data, { entry }) => {
      setAuthorizingKey(null);
      if (data.ok) {
        toastOk(
          pushToast,
          `ONU autorizada: PON ${data.pon ?? entry.pon} / ONU ${data.onu ?? "?"}${data.vlan ? ` · VLAN ${data.vlan}` : ""}${entry.serial ? ` (${entry.serial})` : ""}.`,
        );
        setHiddenKeys((prev) => new Set(prev).add(entryKey(entry)));
        setConfirm(null);
      } else {
        const detail =
          [data.error, data.output ? truncate(data.output, 180) : ""].filter(Boolean).join(" — ") ||
          "O comando telnet de autorização falhou na OLT.";
        setConfirm((prev) => (prev ? { ...prev, actionError: detail } : prev));
        toastErr(pushToast, new Error(detail), "Falha ao autorizar ONU.");
      }
    },
    onError: (err) => {
      setAuthorizingKey(null);
      const parsed = parseApiErrorForModal(err, "Falha ao autorizar ONU");
      const msg = parsed.message || errorMessageFromUnknown(err);
      setConfirm((prev) => (prev ? { ...prev, actionError: msg } : prev));
      toastErr(pushToast, err, "Falha ao autorizar ONU.");
    },
  });

  const entries = useMemo(() => {
    const list = query.data?.entries ?? [];
    return list.filter((e) => !hiddenKeys.has(entryKey(e)));
  }, [query.data?.entries, hiddenKeys]);

  const busyModal = preview.isPending || authorize.isPending;
  const canConfirm = !!confirm?.preview?.onu && !!confirm.preview.vlan && !busyModal && !confirm.previewError;

  function openConfirm(entry: UnauthorizedEntry) {
    setConfirm({ entry, preview: null, previewError: null, actionError: null });
    preview.mutate(entry);
  }

  function retryPreview() {
    if (!confirm?.entry || preview.isPending) return;
    const entry = confirm.entry;
    setConfirm({ entry, preview: null, previewError: null, actionError: null });
    preview.mutate(entry);
  }

  function closeConfirm() {
    if (busyModal) return;
    setConfirm(null);
  }

  return (
    <div style={{ position: "relative", minHeight: 220 }}>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Consulta ONUs não autorizadas via telnet. Ao autorizar, o sistema escolhe o menor ID livre na PON (preenche
        buracos, ex.: 7 se 1–6 e 8–10 já existem). Requer credenciais em <strong>Rede e SNMP</strong> e o comando de{" "}
        <strong>Autorizar ONU</strong> no perfil.
      </p>
      <div className="row stack-mobile" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
        <div className="field" style={{ margin: 0, minWidth: 200, flex: "1 1 200px" }}>
          <label>OLT</label>
          <select className="input" value={effectiveId} onChange={(e) => setOltId(e.target.value)} disabled={query.isPending}>
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
          <RefreshCw size={14} style={{ marginRight: 6, verticalAlign: -2 }} className={query.isPending ? "map-refresh-spin" : undefined} />
          {query.isPending ? "Consultando…" : "Consultar ONUs não autorizadas"}
        </button>
      </div>

      {query.isPending ? (
        <div
          style={{
            position: "absolute",
            inset: 0,
            display: "grid",
            placeItems: "center",
            background: "color-mix(in srgb, var(--bg, #fff) 55%, transparent)",
            zIndex: 2,
            pointerEvents: "none",
          }}
          aria-busy="true"
          aria-label="A consultar ONUs não autorizadas"
        >
          <div style={{ textAlign: "center" }}>
            <RefreshCw size={28} className="map-refresh-spin" style={{ color: "var(--muted)", opacity: 0.85 }} />
            <p style={{ margin: "10px 0 0", fontSize: 12, color: "var(--muted)" }}>A consultar via telnet…</p>
          </div>
        </div>
      ) : null}

      {query.data?.command && showRaw ? (
        <p className="mono" style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
          Comando: {query.data.command}
        </p>
      ) : null}

      {query.isError && <div className="msg msg--err">{(query.error as Error).message}</div>}

      {query.data && (
        <>
          <div
            className="row"
            style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, flexWrap: "wrap", gap: 8 }}
          >
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
                  <th>Descrição</th>
                  <th>Modelo</th>
                  {canMutate ? <th style={{ width: 110 }} /> : null}
                </tr>
              </thead>
              <tbody>
                {entries.length === 0 ? (
                  <tr>
                    <td colSpan={canMutate ? 7 : 6} style={{ color: "var(--muted)" }}>
                      Nenhuma ONU parseada — veja a saída bruta ou ajuste o comando no perfil OLT.
                    </td>
                  </tr>
                ) : (
                  entries.map((e, i) => {
                    const key = entryKey(e);
                    const busy = authorize.isPending && authorizingKey === key;
                    return (
                      <tr key={`${e.serial ?? i}-${e.pon ?? 0}-${e.onu ?? 0}-${e.gpon_onu ?? ""}`}>
                        <td className="mono">{e.serial ?? "—"}</td>
                        <td>{e.pon ?? "—"}</td>
                        <td>{e.onu ?? "—"}</td>
                        <td className="mono" style={{ fontSize: 10 }}>
                          {e.gpon_onu ?? "—"}
                        </td>
                        <td>{ponDesc(e.pon) || "—"}</td>
                        <td>{e.model ?? "—"}</td>
                        {canMutate ? (
                          <td>
                            <button
                              type="button"
                              className="btn btn--sm btn--primary"
                              disabled={authorize.isPending || preview.isPending || !e.serial || !e.pon}
                              title={!e.serial ? "Serial em falta" : !e.pon ? "PON em falta" : "Rever dados e autorizar"}
                              onClick={() => openConfirm(e)}
                            >
                              {busy ? "…" : "Autorizar"}
                            </button>
                          </td>
                        ) : null}
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      {confirm
        ? createPortal(
            <div className="modal-backdrop" role="presentation" onMouseDown={closeConfirm}>
              <div
                className="modal"
                role="dialog"
                aria-modal="true"
                aria-labelledby="authorize-confirm-title"
                onMouseDown={(e) => e.stopPropagation()}
                style={{ maxWidth: 460 }}
              >
                <h3 id="authorize-confirm-title">Confirmar autorização</h3>
                <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
                  Confira os dados abaixo antes de autorizar esta ONU na OLT.
                </p>

                {preview.isPending ? (
                  <div style={{ textAlign: "center", padding: "28px 8px" }} aria-busy="true">
                    <RefreshCw size={28} className="map-refresh-spin" style={{ color: "var(--muted)" }} />
                    <p style={{ margin: "10px 0 0", fontSize: 12, color: "var(--muted)" }}>
                      A obter ID livre e VLAN…
                    </p>
                  </div>
                ) : null}

                {confirm.previewError ? (
                  <div className="msg msg--err" style={{ marginBottom: 12 }}>
                    <strong style={{ display: "block", marginBottom: 4 }}>Erro ao preparar autorização</strong>
                    {confirm.previewError}
                  </div>
                ) : null}

                {!preview.isPending && confirm.preview ? (
                  <dl
                    style={{
                      display: "grid",
                      gridTemplateColumns: "140px 1fr",
                      gap: "8px 12px",
                      margin: "0 0 12px",
                      fontSize: 13,
                    }}
                  >
                    <dt style={{ color: "var(--muted)" }}>Série</dt>
                    <dd className="mono" style={{ margin: 0 }}>
                      {confirm.entry.serial ?? confirm.preview.serial ?? "—"}
                    </dd>
                    <dt style={{ color: "var(--muted)" }}>PON</dt>
                    <dd style={{ margin: 0 }}>{confirm.preview.pon ?? confirm.entry.pon ?? "—"}</dd>
                    <dt style={{ color: "var(--muted)" }}>Descrição</dt>
                    <dd style={{ margin: 0 }}>
                      {confirm.preview.pon_description?.trim() ||
                        ponDesc(confirm.preview.pon ?? confirm.entry.pon) ||
                        "—"}
                    </dd>
                    <dt style={{ color: "var(--muted)" }}>Modelo / tipo</dt>
                    <dd style={{ margin: 0 }}>
                      {confirm.preview.onu_type || confirm.entry.model || "—"}
                      {confirm.preview.onu_type &&
                      confirm.entry.model &&
                      confirm.preview.onu_type.trim().toUpperCase() !== confirm.entry.model.trim().toUpperCase() ? (
                        <span style={{ color: "var(--muted)", fontSize: 11, marginLeft: 6 }}>
                          (lista: {confirm.entry.model})
                        </span>
                      ) : null}
                    </dd>
                    <dt style={{ color: "var(--muted)" }}>Número da ONU</dt>
                    <dd className="mono" style={{ margin: 0 }}>
                      {confirm.preview.gpon_onu || confirm.entry.gpon_onu || "—"}
                    </dd>
                    <dt style={{ color: "var(--muted)" }}>Nome</dt>
                    <dd className="mono" style={{ margin: 0 }}>
                      {confirm.preview.name || "—"}
                    </dd>
                    <dt style={{ color: "var(--muted)" }}>VLAN</dt>
                    <dd style={{ margin: 0 }}>
                      {confirm.preview.vlan ?? "—"}
                      {confirm.preview.vlan_source ? (
                        <span style={{ color: "var(--muted)", fontSize: 11, marginLeft: 6 }}>
                          ({vlanSourceLabel(confirm.preview.vlan_source)})
                        </span>
                      ) : null}
                    </dd>
                    <dt style={{ color: "var(--muted)" }}>ID da ONU</dt>
                    <dd style={{ margin: 0 }}>
                      {confirm.preview.onu ?? "—"}
                      {confirm.preview.allocated_onu ? (
                        <span style={{ color: "var(--muted)", fontSize: 11, marginLeft: 6 }}>(menor ID livre)</span>
                      ) : null}
                    </dd>
                  </dl>
                ) : null}

                {confirm.actionError ? (
                  <div className="msg msg--err" style={{ marginBottom: 12 }}>
                    <strong style={{ display: "block", marginBottom: 4 }}>Erro ao autorizar</strong>
                    {confirm.actionError}
                  </div>
                ) : null}

                {authorize.isPending ? (
                  <div
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 10,
                      marginBottom: 12,
                      padding: "10px 12px",
                      background: "var(--panel2)",
                      borderRadius: 8,
                      fontSize: 12,
                      color: "var(--muted)",
                    }}
                    aria-busy="true"
                  >
                    <RefreshCw size={16} className="map-refresh-spin" />
                    A autorizar via telnet… isto pode demorar alguns minutos.
                  </div>
                ) : null}

                <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 10 }}>
                  <button type="button" className="btn" disabled={busyModal} onClick={closeConfirm}>
                    Cancelar
                  </button>
                  {confirm.previewError ? (
                    <button
                      type="button"
                      className="btn btn--primary"
                      disabled={preview.isPending}
                      onClick={retryPreview}
                    >
                      Tentar novamente
                    </button>
                  ) : (
                    <button
                      type="button"
                      className="btn btn--primary"
                      disabled={!canConfirm}
                      onClick={() => {
                        if (!confirm.entry || !confirm.preview?.onu) return;
                        authorize.mutate({ entry: confirm.entry, onu: confirm.preview.onu });
                      }}
                    >
                      {authorize.isPending ? "A autorizar…" : "Confirmar autorização"}
                    </button>
                  )}
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </div>
  );
}

function entryKey(e: { pon?: number; onu?: number; serial?: string }): string {
  const serial = String(e.serial ?? "").trim().toLowerCase();
  if (serial) return `s:${serial}`;
  return `p:${e.pon ?? 0}:o:${e.onu ?? 0}`;
}

function vlanSourceLabel(src: string): string {
  if (src.startsWith("zte_vlan_snmp")) return "SNMP (ZTE)";
  switch (src) {
    case "device_pon_vlan":
      return "cadastro da OLT";
    case "profile_vlan_catalog":
      return "catálogo do perfil";
    case "snmp":
    case "snmp_live":
      return "SNMP";
    case "profile_default":
      return "padrão do perfil";
    default:
      return src;
  }
}

function gponOnuIfName(entry: UnauthorizedEntry): string {
  const raw = String(entry.gpon_onu ?? "").trim();
  const lower = raw.toLowerCase();
  if (lower.includes("gpon_onu") || lower.includes("gpon-onu")) return raw;
  return "";
}

function truncate(s: string, max: number): string {
  const t = s.trim().replace(/\s+/g, " ");
  if (t.length <= max) return t;
  return `${t.slice(0, max - 1)}…`;
}
