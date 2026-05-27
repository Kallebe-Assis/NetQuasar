import { Link, useLocation } from "react-router-dom";
import { ArrowLeft, Search, Settings } from "lucide-react";
import { isAdminUser } from "../lib/auth";
import { APP_ROUTES } from "../app/routes";

export function IntegrationNav({
  slug,
  name,
  consultaEnabled,
}: {
  slug: string;
  name: string;
  consultaEnabled?: boolean;
}) {
  const loc = useLocation();
  const admin = isAdminUser();
  const onConsulta = loc.pathname.endsWith("/consulta");
  const onConfig = loc.pathname.endsWith("/config");

  return (
    <div className="integration-nav" style={{ marginBottom: 16 }}>
      <Link to={APP_ROUTES.integrations} className="btn" style={{ textDecoration: "none", marginBottom: 10, display: "inline-flex" }}>
        <ArrowLeft size={14} style={{ marginRight: 4 }} /> Integrações
      </Link>
      <h1 style={{ margin: "0 0 10px", fontSize: 20 }}>{name}</h1>
      <div className="tabs integration-nav__tabs">
        {consultaEnabled !== false ? (
          <Link
            to={APP_ROUTES.integrationConsulta(slug)}
            className={onConsulta ? "active" : ""}
            style={{ textDecoration: "none", display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            <Search size={14} /> Consulta
          </Link>
        ) : null}
        {admin ? (
          <Link
            to={APP_ROUTES.integrationConfig(slug)}
            className={onConfig ? "active" : ""}
            style={{ textDecoration: "none", display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            <Settings size={14} /> Configuração API
          </Link>
        ) : null}
      </div>
    </div>
  );
}
