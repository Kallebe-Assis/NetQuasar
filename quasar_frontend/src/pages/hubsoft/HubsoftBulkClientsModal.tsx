import { useState } from "react";
import { createPortal } from "react-dom";
import { Download, X } from "lucide-react";
import { apiFetch } from "../../lib/api";
import { ConsultaLoading } from "./ConsultaLoading";
import { useConsultaToast } from "./hubsoftConsulta";

const SLUG = "hubsoft";
const CHUNK = 20;
const MAX_NAMES = 500;

type Svc = { plan: string; status?: string; login?: string; city?: string; sold_at?: string };
type Match = { id: string; code?: string; name: string; phone?: string; city?: string; services: Svc[] };
type NameResult = { query: string; status: "found" | "approx" | "not_found" | "error"; message?: string; matches: Match[] };
type Resp = { ok: boolean; message?: string; results: NameResult[] };

function csvEsc(v: string): string {
  return /[";,\n\r]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** "Consulta em massa": cola uma lista de nomes (um por linha) e recebe ID, nome, telefone, serviços/planos, cidade e data da venda. */
export function HubsoftBulkClientsModal({ onClose }: { onClose: () => void }) {
  const { notify, missing } = useConsultaToast();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState({ done: 0, total: 0 });
  const [results, setResults] = useState<NameResult[] | null>(null);

  async function run() {
    const names = Array.from(new Set(text.split(/\r?\n/).map((l) => l.trim()).filter(Boolean)));
    if (names.length === 0) return missing("cole ao menos um nome de cliente (um por linha).");
    if (names.length > MAX_NAMES) return missing(`no máximo ${MAX_NAMES} nomes por consulta (você colou ${names.length}).`);
    setBusy(true);
    setResults(null);
    setProgress({ done: 0, total: names.length });
    const acc: NameResult[] = [];
    try {
      for (let i = 0; i < names.length; i += CHUNK) {
        const r = await apiFetch<Resp>(`/api/v1/integrations/${SLUG}/hubsoft/report/clients/bulk`, {
          method: "POST",
          json: { names: names.slice(i, i + CHUNK) },
          timeoutMs: 3 * 60_000,
        });
        if (!r.ok) throw new Error(r.message || "Falha na consulta.");
        acc.push(...r.results);
        setProgress({ done: Math.min(i + CHUNK, names.length), total: names.length });
      }
      setResults(acc);
      notify({ ok: true }, null);
    } catch (e) {
      setResults(acc.length > 0 ? acc : null);
      notify(null, e);
    } finally {
      setBusy(false);
    }
  }

  const found = results?.filter((r) => r.status === "found").length ?? 0;
  const approx = results?.filter((r) => r.status === "approx").length ?? 0;
  const notFound = results?.filter((r) => r.status === "not_found" || r.status === "error").length ?? 0;

  function exportCsv() {
    if (!results) return;
    const head = ["Nome pesquisado", "Situação", "ID", "Código", "Nome", "Telefone", "Serviços (planos)", "Cidade", "Data da venda"];
    const lines: string[][] = [head];
    for (const r of results) {
      const label = r.status === "found" ? "Encontrado" : r.status === "approx" ? "Aproximado" : r.status === "not_found" ? "Não encontrado" : `Erro: ${r.message ?? ""}`;
      if (r.matches.length === 0) lines.push([r.query, label, "", "", "", "", "", "", ""]);
      for (const m of r.matches) {
        lines.push([
          r.query, label, m.id, m.code ?? "", m.name, m.phone ?? "",
          m.services.map((s) => s.plan).join(" | "),
          m.city ?? "",
          m.services.map((s) => s.sold_at || "—").join(" | "),
        ]);
      }
    }
    const csv = lines.map((l) => l.map(csvEsc).join(";")).join("\r\n");
    const url = URL.createObjectURL(new Blob([`﻿${csv}`], { type: "text/csv;charset=utf-8;" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `consulta-em-massa-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 1100, width: "100%", maxHeight: "90vh", overflow: "auto" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", marginBottom: 10 }}>
          <div>
            <h3 style={{ margin: 0 }}>Consulta em massa</h3>
            <p style={{ margin: "2px 0 0", fontSize: 12, color: "var(--muted)" }}>
              Cole os nomes dos clientes (um por linha) para ver ID, nome, telefone, serviços/planos, cidade e data da venda.
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <textarea
          className="input"
          rows={7}
          style={{ width: "100%", fontFamily: "inherit", resize: "vertical" }}
          placeholder={"Maria da Silva\nJoão Pereira\n…"}
          value={text}
          onChange={(e) => setText(e.target.value)}
          disabled={busy}
        />
        <div className="row" style={{ gap: 10, alignItems: "center", margin: "10px 0", flexWrap: "wrap" }}>
          <button type="button" className="btn btn--primary" disabled={busy} onClick={() => void run()}>
            {busy ? "A consultar…" : "Consultar"}
          </button>
          <span style={{ fontSize: 11, color: "var(--muted)" }}>
            {text.split(/\r?\n/).filter((l) => l.trim()).length} nome(s) · máx. {MAX_NAMES} · nome idêntico (sem acento/maiúscula) tem prioridade
          </span>
          {results ? (
            <button type="button" className="btn btn--sm" style={{ marginLeft: "auto" }} onClick={exportCsv}>
              <Download size={12} style={{ marginRight: 4, verticalAlign: -2 }} aria-hidden />
              Exportar CSV
            </button>
          ) : null}
        </div>

        {busy ? <ConsultaLoading text={`Consultando… ${progress.done} de ${progress.total} nome(s)`} /> : null}

        {results ? (
          <>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>
              {found} encontrado(s) · {approx} aproximado(s) (conferir) · {notFound} não encontrado(s)
            </p>
            <div className="table-wrap integration-support-table" style={{ maxHeight: "50vh", overflow: "auto" }}>
              <table className="integration-support-table__grid">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Nome</th>
                    <th>Telefone</th>
                    <th>Serviços (planos)</th>
                    <th>Cidade</th>
                    <th>Data da venda</th>
                  </tr>
                </thead>
                <tbody>
                  {results.flatMap((r) =>
                    r.matches.length === 0
                      ? [
                          <tr key={`${r.query}-nf`}>
                            <td className="integration-support-table__cell">—</td>
                            <td className="integration-support-table__cell" colSpan={5} style={{ color: "var(--err)" }}>
                              {r.query} — {r.status === "error" ? `erro: ${r.message ?? "falha na consulta"}` : "não encontrado"}
                            </td>
                          </tr>,
                        ]
                      : r.matches.map((m) => (
                          <tr key={`${r.query}-${m.id}`}>
                            <td className="mono integration-support-table__cell">
                              {m.id}
                              {m.code && m.code !== m.id ? <div className="integration-support-table__meta">cód. {m.code}</div> : null}
                            </td>
                            <td className="integration-support-table__cell">
                              {m.name}
                              {r.status === "approx" ? <div style={{ fontSize: 10, color: "var(--warn)" }}>aproximado — pesquisado: {r.query}</div> : null}
                            </td>
                            <td className="mono integration-support-table__cell">{m.phone || "—"}</td>
                            <td className="integration-support-table__cell">
                              {m.services.length === 0 ? "—" : m.services.map((s, i) => <div key={i}>{s.plan || "—"}</div>)}
                            </td>
                            <td className="integration-support-table__cell">{m.city || "—"}</td>
                            <td className="mono integration-support-table__cell">
                              {m.services.length === 0 ? "—" : m.services.map((s, i) => <div key={i}>{s.sold_at || "—"}</div>)}
                            </td>
                          </tr>
                        )),
                  )}
                </tbody>
              </table>
            </div>
          </>
        ) : null}
      </div>
    </div>,
    document.body,
  );
}
