import { useId } from "react";

/**
 * Fundo decorativo tipo circuito / grelha tecnológica (muito subtil, não interfere na leitura).
 */
export function LoginCircuitBackdrop() {
  const uid = useId().replace(/:/g, "");
  const lineGrad = `lc-line-${uid}`;

  return (
    <div className="login-circuit-backdrop" aria-hidden>
      <div className="login-circuit-backdrop__grid" />
      <div className="login-circuit-backdrop__grid login-circuit-backdrop__grid--slow" />
      <div className="login-circuit-backdrop__glow" />
      <div className="login-circuit-backdrop__noise" />
      <div className="login-circuit-backdrop__scan" />
      <svg className="login-circuit-backdrop__svg" viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice">
        <defs>
          <linearGradient id={lineGrad} x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="rgba(100, 190, 255, 0)" />
            <stop offset="45%" stopColor="rgba(120, 200, 255, 0.35)" />
            <stop offset="55%" stopColor="rgba(120, 200, 255, 0.35)" />
            <stop offset="100%" stopColor="rgba(100, 190, 255, 0)" />
          </linearGradient>
        </defs>
        <g fill="none" stroke={`url(#${lineGrad})`} strokeWidth="0.6" opacity="0.45">
          <path d="M20 40h80l20 20h120l30-25h90" className="login-circuit-backdrop__trace" />
          <path d="M0 120h60l40-35h100l25 40h175" className="login-circuit-backdrop__trace login-circuit-backdrop__trace--d2" />
          <path d="M40 200h70l35-30h130l20 25h105" className="login-circuit-backdrop__trace login-circuit-backdrop__trace--d3" />
          <path d="M320 20v70l-25 30v100" className="login-circuit-backdrop__trace login-circuit-backdrop__trace--d4" />
        </g>
        <g className="login-circuit-backdrop__nodes" fill="none">
          <circle className="login-circuit-backdrop__node login-circuit-backdrop__node--a" cx="120" cy="60" r="3" stroke="none" />
          <circle className="login-circuit-backdrop__node login-circuit-backdrop__node--b" cx="260" cy="95" r="2.5" stroke="none" />
          <circle className="login-circuit-backdrop__node login-circuit-backdrop__node--c" cx="180" cy="210" r="2.5" stroke="none" />
        </g>
      </svg>
    </div>
  );
}
