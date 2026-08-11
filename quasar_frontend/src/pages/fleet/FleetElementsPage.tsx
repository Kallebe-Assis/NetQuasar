import { useSearchParams } from "react-router-dom";
import { FleetCostCentersPage } from "./FleetCostCentersPage";
import { FleetDriversPage } from "./FleetDriversPage";
import { FleetExpenseTypesPage } from "./FleetExpenseTypesPage";
import { FleetFuelsPage } from "./FleetFuelsPage";
import { FleetStationsPage } from "./FleetStationsPage";

type Tab = "motoristas" | "postos" | "combustiveis" | "centros" | "tipos";

function parseTab(raw: string | null): Tab {
  if (raw === "postos" || raw === "stations") return "postos";
  if (raw === "combustiveis" || raw === "fuels" || raw === "combustíveis") return "combustiveis";
  if (raw === "centros" || raw === "cost-centers" || raw === "centros-de-custo") return "centros";
  if (raw === "tipos" || raw === "tipos-despesa" || raw === "expense-types") return "tipos";
  return "motoristas";
}

export function FleetElementsPage() {
  const [params, setParams] = useSearchParams();
  const tab = parseTab(params.get("tab"));

  function setTab(next: Tab) {
    const nextParams = new URLSearchParams(params);
    nextParams.set("tab", next);
    setParams(nextParams, { replace: true });
  }

  return (
    <div className="fleet-page">
      <h1 style={{ marginTop: 0 }}>Frota — Elementos</h1>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Cadastro de motoristas, postos, combustíveis, centros de custo e tipos de despesa.
      </p>
      <div className="tabs" style={{ flexWrap: "wrap", marginBottom: 12 }}>
        <button type="button" className={tab === "motoristas" ? "active" : ""} onClick={() => setTab("motoristas")}>
          Motoristas
        </button>
        <button type="button" className={tab === "postos" ? "active" : ""} onClick={() => setTab("postos")}>
          Postos de combustível
        </button>
        <button type="button" className={tab === "combustiveis" ? "active" : ""} onClick={() => setTab("combustiveis")}>
          Combustíveis
        </button>
        <button type="button" className={tab === "centros" ? "active" : ""} onClick={() => setTab("centros")}>
          Centros de custo
        </button>
        <button type="button" className={tab === "tipos" ? "active" : ""} onClick={() => setTab("tipos")}>
          Tipos de despesa
        </button>
      </div>
      {tab === "motoristas" ? <FleetDriversPage embedded /> : null}
      {tab === "postos" ? <FleetStationsPage embedded /> : null}
      {tab === "combustiveis" ? <FleetFuelsPage embedded /> : null}
      {tab === "centros" ? <FleetCostCentersPage embedded /> : null}
      {tab === "tipos" ? <FleetExpenseTypesPage embedded /> : null}
    </div>
  );
}
