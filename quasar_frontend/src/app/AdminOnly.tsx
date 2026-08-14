import type { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { can, isAdminUser } from "../lib/auth";
import { APP_ROUTES } from "./routes";

/** Redireciona quem não tem acesso a configurações; admin e perfis com settings.* passam. */
export function AdminOnly({ children }: { children: ReactNode }) {
  if (isAdminUser() || can("settings.view") || can("settings.users") || can("settings.permissions") || can("integrations.manage")) {
    return <>{children}</>;
  }
  return <Navigate to={APP_ROUTES.dashboard} replace />;
}

/** Apenas usuários admin (perfil * / role admin). */
export function StrictAdminOnly({ children }: { children: ReactNode }) {
  if (isAdminUser()) {
    return <>{children}</>;
  }
  return <Navigate to={APP_ROUTES.dashboard} replace />;
}
