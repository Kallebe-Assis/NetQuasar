import { Navigate, Outlet, useLocation } from "react-router-dom";
import { getAuthToken } from "../lib/auth";

/** Sem JWT no browser, a aplicação protegida redireciona sempre para o login. */
export function ProtectedLayout() {
  const loc = useLocation();
  if (!getAuthToken()) {
    return <Navigate to="/login" replace state={{ from: loc.pathname }} />;
  }
  return <Outlet />;
}
