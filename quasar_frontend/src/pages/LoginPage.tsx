import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";
import { APP_ROUTES } from "../app/routes";
import { GlobeSplash } from "../components/GlobeSplash";
import { LoginBrandLogo } from "../components/LoginBrandLogo";
import { LoginCircuitBackdrop } from "../components/LoginCircuitBackdrop";
import { apiFetch, ApiError } from "../lib/api";
import {
  getAuthToken,
  isClientConfigured,
  isSessionReady,
  markClientConfigured,
  markSessionReady,
  saveAuthToken,
  saveUserDisplayLabel,
  saveUserRole,
} from "../lib/auth";
import { prefetchStaticPages } from "../lib/prefetchStaticPages";

type SetupStatus = { database_configured?: boolean };

type AuthLoginResponse = { token?: string; email?: string; display_name?: string; role?: string };

/** Duração mínima do ecrã de loading após requisição de login (ms). */
const LOGIN_SPLASH_MIN_MS = 2000;

export function LoginPage() {
  const nav = useNavigate();
  const loc = useLocation();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const loginStartedAtRef = useRef<number | null>(null);
  const postLoginNavTimerRef = useRef<number | null>(null);
  const [postLoginNavPending, setPostLoginNavPending] = useState(false);

  useEffect(() => {
    return () => {
      if (postLoginNavTimerRef.current != null) {
        window.clearTimeout(postLoginNavTimerRef.current);
      }
    };
  }, []);

  const setupQ = useQuery({
    queryKey: ["setup-status-prelogin"],
    queryFn: () => apiFetch<SetupStatus>("/api/v1/setup/status"),
  });

  const authMut = useMutation({
    mutationKey: ["auth-login"],
    mutationFn: (body: { email: string; password: string }) =>
      apiFetch<AuthLoginResponse>("/api/v1/auth/login", { method: "POST", json: body, skipAuth: true }),
    onMutate: () => {
      loginStartedAtRef.current = Date.now();
    },
    onSuccess: async (data) => {
      const t = typeof data?.token === "string" ? data.token : "";
      if (!t) {
        setErr("Resposta do servidor sem token de sessão.");
        return;
      }
      saveAuthToken(t);
      saveUserRole(typeof data?.role === "string" ? data.role : "admin");
      const display =
        typeof data?.display_name === "string" && data.display_name.trim()
          ? data.display_name.trim()
          : typeof data?.email === "string" && data.email.trim()
            ? data.email.trim()
            : "";
      saveUserDisplayLabel(display);
      markSessionReady();
      if (!isClientConfigured()) {
        markClientConfigured();
      }
      const started = loginStartedAtRef.current ?? Date.now();
      const remain = Math.max(0, LOGIN_SPLASH_MIN_MS - (Date.now() - started));
      loginStartedAtRef.current = null;
      if (postLoginNavTimerRef.current != null) {
        window.clearTimeout(postLoginNavTimerRef.current);
        postLoginNavTimerRef.current = null;
      }
      setPostLoginNavPending(true);
      try {
        await Promise.all([prefetchStaticPages(qc), new Promise((r) => window.setTimeout(r, remain))]);
      } catch {
        /* dashboard pode falhar; a página tenta de novo */
      }
      setPostLoginNavPending(false);
      const from = (loc.state as { from?: string } | null)?.from;
      const dest =
        from && from !== APP_ROUTES.login && from.startsWith("/") && !from.startsWith("//")
          ? from
          : APP_ROUTES.dashboard;
      nav(dest, { replace: true });
    },
    onError: (e) => {
      loginStartedAtRef.current = null;
      if (postLoginNavTimerRef.current != null) {
        window.clearTimeout(postLoginNavTimerRef.current);
        postLoginNavTimerRef.current = null;
      }
      setPostLoginNavPending(false);
      if (e instanceof ApiError) {
        setErr(e.message || "Credenciais inválidas.");
        return;
      }
      setErr(String(e));
    },
  });

  if (setupQ.isLoading) {
    return (
      <div className="login-page login-page--splash" style={{ position: "relative" }}>
        <LoginCircuitBackdrop />
        <div className="login-page__center-brand">
          <LoginBrandLogo />
        </div>
        <GlobeSplash />
      </div>
    );
  }

  if (setupQ.isError) {
    return (
      <div className="login-page">
        <LoginCircuitBackdrop />
        <div className="login-box">
          <LoginBrandLogo />
          <div className="msg msg--err">{(setupQ.error as Error).message}</div>
          <p style={{ marginTop: 12, fontSize: 13, color: "var(--muted)" }}>
            Não foi possível contactar a API. Confirme a URL do servidor ou a rede.
          </p>
          <p style={{ marginTop: 12, fontSize: 13 }}>
            <Link to="/client-setup">Configurar URL da API e chave</Link>
          </p>
        </div>
      </div>
    );
  }

  if (!setupQ.data?.database_configured) {
    return <Navigate to={APP_ROUTES.configSetup} replace />;
  }

  if (getAuthToken() && isSessionReady()) {
    return <Navigate to={APP_ROUTES.dashboard} replace />;
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    const u = email.trim();
    if (!u || !password) {
      setErr("Preencha e-mail e palavra-passe.");
      return;
    }
    authMut.mutate({ email: u, password });
  }

  const loading = authMut.isPending || postLoginNavPending;

  return (
    <div className="login-page" style={{ position: "relative" }}>
      <LoginCircuitBackdrop />
      {loading ? <GlobeSplash /> : null}
      <div className="login-box">
        <LoginBrandLogo />
        {err ? <div className="msg msg--err">{err}</div> : null}
        <form onSubmit={onSubmit}>
          <div className="login-credentials-row">
            <div className="field">
              <label htmlFor="login-email">E-mail</label>
              <input
                id="login-email"
                className="input"
                style={{ width: "100%" }}
                autoComplete="username"
                type="email"
                inputMode="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="nome@empresa.com"
              />
            </div>
            <div className="field">
              <label htmlFor="login-pass">Palavra-passe</label>
              <input
                id="login-pass"
                className="input"
                style={{ width: "100%" }}
                type="password"
                autoComplete="current-password"
                inputMode="text"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          </div>
          <button className="btn btn--primary" type="submit" disabled={loading} style={{ width: "100%", marginTop: 8 }}>
            {loading ? "A entrar…" : "Entrar"}
          </button>
        </form>
      </div>
    </div>
  );
}
