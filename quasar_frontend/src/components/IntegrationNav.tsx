import type { ReactNode } from "react";
import { Link, useLocation } from "react-router-dom";
import { ArrowLeft, Search, Settings } from "lucide-react";
import { isAdminUser } from "../lib/auth";
import { APP_ROUTES } from "../app/routes";

export function IntegrationNav({
  slug,
  name,
  consultaEnabled,
  badges,
}: {
  slug: string;
  name: string;
  consultaEnabled?: boolean;
  /** Tags de estado (Ativa, Sessão ativa…): aparecem no canto direito, na mesma linha do nome. */
  badges?: ReactNode;
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
      <div className="integration-nav__title">
        <h1 style={{ margin: 0, fontSize: 20 }}>{name}</h1>
        {badges ? <div className="integration-nav__badges">{badges}</div> : null}
      </div>
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
            style={{ textDecoration: "none", display: "inline-flex", alignItems: "center" }}
            title="Configuração API"
            aria-label="Configuração API"
          >
            <Settings size={14} />
          </Link>
        ) : null}
      </div>
    </div>
  );
}
