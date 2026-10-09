import { useSearchParams } from "react-router-dom";
import { Segmented } from "./hubsoftAdminKit";
import { HubsoftRegistrationCheck } from "./HubsoftRegistrationCheck";
import { HubsoftStockCheck } from "./HubsoftStockChecks";
import { HubsoftDataVendaCheck } from "./HubsoftDataVendaCheck";

/**
 * Aba «Conferência» da configuração da HubSoft: todas as conferências SOMENTE LEITURA num só lugar. O operador escolhe o que
 * conferir — cadastro de clientes, serviços, patrimônios, data de venda ou produtos — e envia o CSV correspondente.
 */

type What = "clientes" | "servicos" | "patrimonios" | "data-venda" | "produtos";
const OPTIONS: { value: What; label: string }[] = [
  { value: "clientes", label: "Cadastro de clientes" },
  { value: "servicos", label: "Serviços" },
  { value: "patrimonios", label: "Patrimônios" },
  { value: "data-venda", label: "Data de venda" },
  { value: "produtos", label: "Produtos" },
];

export function HubsoftConference() {
  const [params, setParams] = useSearchParams();
  const raw = params.get("conf");
  const what: What = OPTIONS.some((o) => o.value === raw) ? (raw as What) : "clientes";

  function choose(w: What) {
    const next = new URLSearchParams(params);
    next.set("conf", w);
    setParams(next, { replace: true });
  }

  return (
    <div style={{ display: "grid", gap: 12 }}>
      <div className="card" style={{ padding: 12 }}>
        <Segmented label="O que conferir" value={what} onChange={choose} options={OPTIONS} />
      </div>
      {what === "clientes" ? <HubsoftRegistrationCheck key="clientes" fixedKind="client" /> : null}
      {what === "servicos" ? <HubsoftRegistrationCheck key="servicos" fixedKind="service" /> : null}
      {what === "patrimonios" ? <HubsoftStockCheck key="patrimonios" kind="patrimonios" /> : null}
      {what === "data-venda" ? <HubsoftDataVendaCheck key="data-venda" /> : null}
      {what === "produtos" ? <HubsoftStockCheck key="produtos" kind="produtos" /> : null}
    </div>
  );
}
