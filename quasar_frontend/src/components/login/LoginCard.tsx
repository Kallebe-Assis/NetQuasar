import { useId, useState, type FormEvent } from "react";
import { Eye, EyeOff, Loader2, Lock, Mail } from "lucide-react";

export type LoginCardProps = {
  email: string;
  password: string;
  remember: boolean;
  error: string;
  loading: boolean;
  onEmail: (v: string) => void;
  onPassword: (v: string) => void;
  onRemember: (v: boolean) => void;
  onSubmit: (e: FormEvent) => void;
};

/** Cartão de vidro com o formulário de acesso (lado direito da tela de login). Só apresentação — a lógica fica em LoginPage. */
export function LoginCard(p: LoginCardProps) {
  const uid = useId();
  const emailId = `${uid}-email`;
  const passId = `${uid}-pass`;
  const [showPass, setShowPass] = useState(false);
  const [showForgot, setShowForgot] = useState(false);

  return (
    <div className="login-card">
      <div className="login-card__brand">
        <span className="login-card__logo">
          <img src="/Logo-NetQuasar.png" alt="" width={200} height={133} decoding="async" />
        </span>
        <h1 className="login-card__title">NetQuasar</h1>
        <p className="login-card__sub">Entre na sua conta para continuar</p>
      </div>

      {p.error ? (
        <div className="login-card__error" role="alert">
          {p.error}
        </div>
      ) : null}

      <form onSubmit={p.onSubmit} noValidate>
        <div className="login-field">
          <label htmlFor={emailId}>E-mail</label>
          <div className="login-input">
            <Mail size={17} className="login-input__icon" aria-hidden />
            <input
              id={emailId}
              className="login-input__field"
              type="email"
              inputMode="email"
              autoComplete="username"
              placeholder="nome@empresa.com"
              value={p.email}
              onChange={(e) => p.onEmail(e.target.value)}
            />
          </div>
        </div>

        <div className="login-field">
          <label htmlFor={passId}>Palavra-passe</label>
          <div className="login-input">
            <Lock size={17} className="login-input__icon" aria-hidden />
            <input
              id={passId}
              className="login-input__field"
              type={showPass ? "text" : "password"}
              autoComplete="current-password"
              placeholder="••••••••"
              value={p.password}
              onChange={(e) => p.onPassword(e.target.value)}
            />
            <button
              type="button"
              className="login-input__toggle"
              aria-label={showPass ? "Ocultar palavra-passe" : "Mostrar palavra-passe"}
              aria-pressed={showPass}
              onClick={() => setShowPass((v) => !v)}
            >
              {showPass ? <EyeOff size={17} aria-hidden /> : <Eye size={17} aria-hidden />}
            </button>
          </div>
        </div>

        <button className="login-submit" type="submit" disabled={p.loading}>
          {p.loading ? (
            <>
              <Loader2 size={17} className="login-submit__spin" aria-hidden /> A entrar…
            </>
          ) : (
            "Entrar"
          )}
        </button>

        <div className="login-card__row">
          <button type="button" className="login-link" aria-expanded={showForgot} onClick={() => setShowForgot((v) => !v)}>
            Esqueceu a senha?
          </button>
        </div>
        {showForgot ? (
          <p className="login-card__help" role="note">
            Para redefinir a palavra-passe, peça a um administrador do NetQuasar em <b>Configurações → Usuários</b>.
          </p>
        ) : null}

        <label className="login-check">
          <input type="checkbox" checked={p.remember} onChange={(e) => p.onRemember(e.target.checked)} />
          <span className="login-check__box" aria-hidden />
          <span>Manter-me conectado</span>
        </label>
      </form>

      <p className="login-card__foot">NetQuasar · acesso restrito à equipe</p>
    </div>
  );
}
