import { Link } from "react-router-dom";
import { APP_ROUTES } from "../app/routes";

export function NotFoundPage() {
  return (
    <div className="card" style={{ maxWidth: 480 }}>
      <h1 style={{ marginTop: 0 }}>Página não encontrada</h1>
      <p style={{ color: "var(--muted)" }}>
        O endereço que abriu não corresponde a nenhum ecrã desta aplicação.
      </p>
      <Link to={APP_ROUTES.dashboard} className="btn btn--primary" style={{ textDecoration: "none", display: "inline-block" }}>
        Ir para o dashboard
      </Link>
    </div>
  );
}
