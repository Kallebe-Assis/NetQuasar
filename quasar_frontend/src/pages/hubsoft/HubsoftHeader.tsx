import { Link, useLocation } from "react-router-dom";
import { ArrowLeft, BarChart3, ClipboardList, LifeBuoy, Receipt, Search, Settings } from "lucide-react";
import { isAdminUser } from "../../lib/auth";
import { APP_ROUTES } from "../../app/routes";
import { useHubsoftLogo } from "../../lib/hubsoftLogo";

/**
 * Cabeçalho dedicado às telas da HubSoft (Consulta/Atendimentos/Ordens/Financeiro/Dashboard/
 * Configuração) — não usa o <IntegrationNav> partilhado (esse continua a servir o IXC e as
 * integrações genéricas sem alterações). Título centrado, logo opcional ao lado do nome,
 * abas reais (não links soltos).
 */
export function HubsoftHeader() {
  const loc = useLocation();
  const admin = isAdminUser();
  const logo = useHubsoftLogo();

  const tabs = [
    { to: APP_ROUTES.integrationConsulta("hubsoft"), label: "Consulta", icon: <Search size={14} /> },
    { to: APP_ROUTES.hubsoftAttendance, label: "Atendimentos", icon: <LifeBuoy size={14} /> },
    { to: APP_ROUTES.hubsoftWorkOrders, label: "Ordens de serviço", icon: <ClipboardList size={14} /> },
    { to: APP_ROUTES.hubsoftFinancial, label: "Financeiro", icon: <Receipt size={14} /> },
    { to: APP_ROUTES.hubsoftDashboard, label: "Dashboard", icon: <BarChart3 size={14} /> },
  ];

  return (
    <div className="hubsoft-header">
      <Link to={APP_ROUTES.integrations} className="hubsoft-header__back">
        <ArrowLeft size={14} /> Integrações
      </Link>
      <div className="hubsoft-header__title-row">
        {logo ? <img src={logo} alt="" className="hubsoft-header__logo" /> : null}
        <h1 className="hubsoft-header__title">Hubsoft</h1>
      </div>
      <div className="hubsoft-header__tabs">
        {tabs.map((t) => (
          <Link key={t.to} to={t.to} className={loc.pathname === t.to ? "active" : ""}>
            {t.icon} {t.label}
          </Link>
        ))}
        {admin ? (
          <Link to={APP_ROUTES.integrationConfig("hubsoft")} className={loc.pathname.endsWith("/config") ? "active" : ""}>
            <Settings size={14} /> Configuração API
          </Link>
        ) : null}
      </div>
    </div>
  );
}
