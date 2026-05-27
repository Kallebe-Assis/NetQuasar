import type { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { isAdminUser } from "../lib/auth";
import { APP_ROUTES } from "./routes";

/** Redireciona visitantes (viewer) para o dashboard; só renderiza `children` para administradores. */
export function AdminOnly({ children }: { children: ReactNode }) {
  if (!isAdminUser()) {
    return <Navigate to={APP_ROUTES.dashboard} replace />;
  }
  return <>{children}</>;
}
