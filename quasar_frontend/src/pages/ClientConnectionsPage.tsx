import { InfoHint } from "../components/InfoHint";
import { isAdminUser } from "../lib/auth";
import { CommercialConnectionsTab } from "./commercial/CommercialConnectionsTab";

export function ClientConnectionsPage() {
  const canMutate = isAdminUser();

  return (
    <>
      <div className="page-heading">
        <h1>
          Conexões
          <InfoHint label="Sobre conexões de clientes">
            <p>Logins PPPoE e DHCP com dados comerciais e coordenadas para o mapa. Importe via CSV ou cadastre individualmente.</p>
          </InfoHint>
        </h1>
      </div>
      <CommercialConnectionsTab canMutate={canMutate} />
    </>
  );
}
