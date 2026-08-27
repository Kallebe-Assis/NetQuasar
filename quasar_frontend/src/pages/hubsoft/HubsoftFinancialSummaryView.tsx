import type { HubsoftFinancialSummaryResponse } from "../../integrations/types";

function fmtInt(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR");
}
function fmtCurrency(n?: number): string {
  return (n ?? 0).toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

/** KPIs + maiores devedores — usado pela aba Financeiro e pelo modo "Financeiro" do Dashboard. */
export function HubsoftFinancialSummaryView({ d }: { d: HubsoftFinancialSummaryResponse }) {
  return (
    <>
      <div className="dashboard-kpi-row" style={{ gridTemplateColumns: "repeat(5, minmax(0, 1fr))" }}>
        <div className="stat">
          <div className="stat__k">Total a receber</div>
          <div className="stat__v" style={{ fontSize: 15 }}>
            {fmtCurrency(d.total_receivable)}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Vencido</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--err)" }}>
            {fmtCurrency(d.total_overdue)}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Pendente (não vencido)</div>
          <div className="stat__v" style={{ fontSize: 15 }}>
            {fmtCurrency(d.total_pending)}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Recebido</div>
          <div className="stat__v" style={{ fontSize: 15, color: "var(--ok)" }}>
            {fmtCurrency(d.total_paid)}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Clientes com pendência</div>
          <div className="stat__v">{fmtInt(d.clients_with_debt)}</div>
        </div>
      </div>

      <section className="integration-detail__section" style={{ marginTop: 16 }}>
        <h4 className="integration-detail__section-title">
          Maiores devedores (amostra) <span className="integration-detail__count">({d.top_debtors.length})</span>
        </h4>
        {d.top_debtors.length === 0 ? (
          <div className="msg">Nenhum cliente com pendência encontrado na amostra.</div>
        ) : (
          <div className="table-wrap integration-support-table">
            <table className="integration-support-table__grid">
              <thead>
                <tr>
                  <th>Cliente</th>
                  <th>Faturas em aberto</th>
                  <th>Vencido</th>
                  <th>Pendente</th>
                  <th>Total</th>
                </tr>
              </thead>
              <tbody>
                {d.top_debtors.map((c, i) => (
                  <tr key={c.client_code || i}>
                    <td className="integration-support-table__cell">
                      {c.client_name || "—"}
                      {c.client_code ? <span className="mono integration-support-table__meta"> · {c.client_code}</span> : null}
                    </td>
                    <td className="integration-support-table__cell">{fmtInt(c.invoice_count)}</td>
                    <td className="mono integration-support-table__cell" style={{ color: "var(--err)" }}>
                      {fmtCurrency(c.overdue_value)}
                    </td>
                    <td className="mono integration-support-table__cell">{fmtCurrency(c.pending_value)}</td>
                    <td className="mono integration-support-table__cell">{fmtCurrency(c.pending_value + c.overdue_value)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}
