import { Navigate, useParams } from "react-router-dom";
import { APP_ROUTES } from "./routes";

/** Redireciona /integrations/:slug → /integrations/:slug/consulta */
export function IntegrationSlugRedirect() {
  const { slug } = useParams<{ slug: string }>();
  if (!slug?.trim()) {
    return <Navigate to={APP_ROUTES.integrations} replace />;
  }
  return <Navigate to={APP_ROUTES.integrationConsulta(slug)} replace />;
}
