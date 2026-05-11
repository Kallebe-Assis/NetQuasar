import { useQuery } from "@tanstack/react-query";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { LoginCircuitBackdrop } from "../components/LoginCircuitBackdrop";
import { apiFetch } from "../lib/api";
import { FirstRunDatabaseSetup } from "../app/FirstRunDatabaseSetup";
import { isClientConfigured } from "../lib/auth";

type SetupStatus = { database_configured?: boolean };

/** Configuração inicial do Postgres (rota /config-setup). */
export function ConfigSetupPage() {
  const nav = useNavigate();
  const clientOk = isClientConfigured();
  const q = useQuery({
    queryKey: ["setup-status", "config-setup"],
    queryFn: () => apiFetch<SetupStatus>("/api/v1/setup/status"),
    enabled: clientOk,
  });

  if (!clientOk) {
    return <Navigate to="/client-setup" replace />;
  }

  if (q.isLoading) {
    return (
      <div className="login-page">
        <LoginCircuitBackdrop />
        <div className="login-box">
          <p style={{ color: "var(--muted)" }}>A carregar estado da base de dados…</p>
        </div>
      </div>
    );
  }
  if (q.isError) {
    return (
      <div className="login-page">
        <LoginCircuitBackdrop />
        <div className="login-box">
          <div className="msg msg--err">{(q.error as Error).message}</div>
          <p style={{ marginTop: 12 }}>
            <Link to="/client-setup">Rever URL do servidor</Link>
          </p>
        </div>
      </div>
    );
  }
  if (q.data?.database_configured) {
    return (
      <div className="login-page">
        <LoginCircuitBackdrop />
        <div className="login-box">
          <p>A base de dados deste servidor já está configurada.</p>
          <Link to="/login" className="btn btn--primary" style={{ display: "inline-block", marginTop: 12 }}>
            Ir para o login
          </Link>
        </div>
      </div>
    );
  }
  return (
    <div className="login-page">
      <LoginCircuitBackdrop />
      <FirstRunDatabaseSetup onConfigured={() => nav("/login", { replace: true })} />
    </div>
  );
}
