import { useSearchParams } from "react-router-dom";
import { Segmented } from "./hubsoftAdminKit";
import { HubsoftStockProducts } from "./HubsoftStockProducts";
import { HubsoftStockItems } from "./HubsoftStockItems";
import { HubsoftStockComodato } from "./HubsoftStockComodato";
import { HubsoftStockComodatoBatch } from "./HubsoftStockComodatoBatch";

/**
 * Aba «Estoque» da configuração da HubSoft: cadastro em massa de PRODUTOS e de PATRIMÔNIOS de estoque e comodato de um patrimônio para o
 * cliente (passos 1, 2 e 3 da migração do IXC), em sub-abas. A conferência dos dois fica na aba «Conferência».
 */

type Sub = "produtos" | "patrimonios" | "comodato" | "comodato-lote";

export function HubsoftStock() {
  const [params, setParams] = useSearchParams();
  // ?estoque= escolhe a sub-aba; links antigos (?aba=patrimonios) também abrem os patrimônios
  const raw = params.get("estoque") ?? (params.get("aba") === "patrimonios" ? "patrimonios" : "produtos");
  const sub: Sub = raw === "patrimonios" || raw === "comodato" || raw === "comodato-lote" ? raw : "produtos";

  function choose(s: Sub) {
    const next = new URLSearchParams(params);
    next.set("estoque", s);
    setParams(next, { replace: true });
  }

  return (
    <div style={{ display: "grid", gap: 12 }}>
      <div className="card" style={{ padding: 12 }}>
        <Segmented
          label="Estoque"
          value={sub}
          onChange={choose}
          options={[
            { value: "produtos", label: "Produtos de estoque" },
            { value: "patrimonios", label: "Patrimônios de estoque" },
            { value: "comodato", label: "Comodato para o cliente" },
            { value: "comodato-lote", label: "Comodato em lote (CSV)" },
          ]}
        />
      </div>
      {sub === "produtos" ? <HubsoftStockProducts key="produtos" /> : null}
      {sub === "patrimonios" ? <HubsoftStockItems key="patrimonios" /> : null}
      {sub === "comodato" ? <HubsoftStockComodato key="comodato" /> : null}
      {sub === "comodato-lote" ? <HubsoftStockComodatoBatch key="comodato-lote" /> : null}
    </div>
  );
}
