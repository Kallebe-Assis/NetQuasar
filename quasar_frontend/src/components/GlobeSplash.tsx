import { useEffect, useId, useMemo, useState } from "react";

/** Pontos na esfera (distribuição esférica) para o “globo digital”. */
function useGlobeDots(n: number, seed = 42) {
  return useMemo(() => {
    const pts: { cx: number; cy: number; r: number; o: number }[] = [];
    let s = seed;
    const rnd = () => {
      s = (s * 1103515245 + 12345) & 0x7fffffff;
      return s / 0x7fffffff;
    };
    const golden = Math.PI * (3 - Math.sqrt(5));
    for (let i = 0; i < n; i++) {
      const t = i / Math.max(1, n - 1);
      const y = 1 - t * 2;
      const rr = Math.sqrt(Math.max(0, 1 - y * y));
      const phi = i * golden;
      const x = Math.cos(phi) * rr;
      const z = Math.sin(phi) * rr;
      const scale = 88;
      const px = 100 + x * scale;
      const py = 100 + z * scale;
      const depth = (y + 1) / 2;
      const o = 0.25 + depth * 0.55 + rnd() * 0.15;
      const r = 0.35 + rnd() * 0.55;
      pts.push({ cx: px, cy: py, r, o: Math.min(1, o) });
    }
    return pts;
  }, [n, seed]);
}

function CloudRouterMotif({ prefix }: { prefix: string }) {
  const fg = `${prefix}-motif-glow`;
  return (
    <svg className="globe-splash__motif" viewBox="0 0 200 72" aria-hidden>
      <defs>
        <filter id={fg} x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="1.2" result="b" />
          <feMerge>
            <feMergeNode in="b" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <g filter={`url(#${fg})`} fill="none" stroke="#00d4ff" strokeWidth="1.2" strokeLinecap="round" opacity="0.85">
        <path d="M62 38c-8 0-14-6-14-14s6-14 14-14h76c8 0 14 6 14 14s-6 14-14 14H62z" opacity="0.9" />
        <path d="M92 22v12M108 22v12" strokeWidth="1.6" />
        <path d="M100 34v18" className="globe-splash__motif-pulse" />
        <circle cx="100" cy="56" r="2.2" fill="#55ffff" stroke="none" />
        <rect x="118" y="52" width="36" height="14" rx="2" />
        <path d="M124 38v14M148 38v14" />
        <circle cx="128" cy="58" r="1.2" fill="#55ffff" stroke="none" />
        <circle cx="136" cy="58" r="1.2" fill="#55ffff" stroke="none" />
        <circle cx="144" cy="58" r="1.2" fill="#55ffff" stroke="none" />
        <circle cx="152" cy="58" r="1.2" fill="#55ffff" stroke="none" />
      </g>
    </svg>
  );
}

function WifiIcon({ prefix }: { prefix: string }) {
  const fb = `${prefix}-wifi-bloom`;
  return (
    <svg className="globe-splash__wifi" viewBox="0 0 48 40" aria-hidden>
      <defs>
        <filter id={fb} x="-50%" y="-50%" width="200%" height="200%">
          <feGaussianBlur stdDeviation="1.5" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <g filter={`url(#${fb})`} fill="none" stroke="#00e5ff" strokeWidth="2" strokeLinecap="round" opacity="0.95">
        <path d="M8 28 Q24 12 40 28" className="globe-splash__wifi-wave" />
        <path d="M14 28 Q24 18 34 28" className="globe-splash__wifi-wave" style={{ animationDelay: "0.12s" }} />
        <path d="M20 28 Q24 22 28 28" className="globe-splash__wifi-wave" style={{ animationDelay: "0.24s" }} />
      </g>
      <circle cx="24" cy="30" r="2.5" fill="#55ffff" />
    </svg>
  );
}

/**
 * Loader alinhado às referências visuais: fundo escuro, globo em pontos, órbitas,
 * motivo nuvem↔router, barra cápsula, textos e Wi‑Fi.
 */
export function GlobeSplash({ variant = "full" }: { variant?: "full" | "route" }) {
  const subtle = variant === "route";
  const dots = useGlobeDots(140);
  const uid = useId().replace(/:/g, "");
  const gBg = `${uid}-globe-bg`;
  const gClip = `${uid}-globe-clip`;
  const gGlow = `${uid}-globe-ring-glow`;

  return (
    <div
      className={`globe-splash ${subtle ? "globe-splash--route" : ""}`}
      role="progressbar"
      aria-busy="true"
      aria-label="A carregar"
    >
      <div className="globe-splash__vignette" />
      <div className="globe-splash__side-wave globe-splash__side-wave--left" aria-hidden />
      <div className="globe-splash__side-wave globe-splash__side-wave--right" aria-hidden />

      <div className="globe-splash__content">
        <CloudRouterMotif prefix={uid} />

        <div className="globe-splash__globe-wrap">
          <svg className="globe-splash__globe-svg" viewBox="0 0 200 200" aria-hidden>
            <defs>
              <radialGradient id={gBg} cx="50%" cy="42%" r="58%">
                <stop offset="0%" stopColor="rgba(0, 128, 255, 0.2)" />
                <stop offset="70%" stopColor="rgba(0, 10, 30, 0.06)" />
                <stop offset="100%" stopColor="rgba(0, 0, 0, 0)" />
              </radialGradient>
              <clipPath id={gClip}>
                <circle cx="100" cy="100" r="82" />
              </clipPath>
              <filter id={gGlow} x="-30%" y="-30%" width="160%" height="160%">
                <feGaussianBlur stdDeviation="2" result="blur" />
                <feMerge>
                  <feMergeNode in="blur" />
                  <feMergeNode in="SourceGraphic" />
                </feMerge>
              </filter>
            </defs>
            <circle cx="100" cy="100" r="90" fill={`url(#${gBg})`} />
            <g className="globe-splash__orbit-group">
              <ellipse cx="100" cy="100" rx="84" ry="30" fill="none" stroke="rgba(0, 229, 255, 0.35)" strokeWidth="1" />
              <ellipse cx="100" cy="100" rx="84" ry="52" fill="none" stroke="rgba(0, 128, 255, 0.22)" strokeWidth="0.9" />
              <ellipse cx="100" cy="100" rx="32" ry="84" fill="none" stroke="rgba(85, 255, 255, 0.28)" strokeWidth="0.9" />
              <ellipse cx="100" cy="100" rx="58" ry="84" fill="none" stroke="rgba(0, 204, 255, 0.18)" strokeWidth="0.75" />
            </g>
            <g clipPath={`url(#${gClip})`} className="globe-splash__dots-group">
              {dots.map((d, i) => (
                <circle key={i} cx={d.cx} cy={d.cy} r={d.r} fill="#00ccff" opacity={d.o} />
              ))}
            </g>
            <circle
              cx="100"
              cy="100"
              r="82"
              fill="none"
              stroke="rgba(0, 229, 255, 0.55)"
              strokeWidth="1.3"
              filter={`url(#${gGlow})`}
            />
          </svg>
        </div>

        <p className="globe-splash__title">C A R R E G A N D O . . .</p>

        <div className="globe-splash__bar" aria-hidden>
          <div className="globe-splash__bar-fill" />
        </div>

        <p className="globe-splash__tagline">CONECTANDO AO QUE IMPORTA</p>

        <WifiIcon prefix={uid} />
      </div>
    </div>
  );
}

/** Só mostra o splash após `delayMs` (evita flash em navegações rápidas). */
export function DelayedGlobeFallback({ delayMs = 500 }: { delayMs?: number }) {
  const [show, setShow] = useState(false);
  useEffect(() => {
    const id = window.setTimeout(() => setShow(true), delayMs);
    return () => {
      window.clearTimeout(id);
      setShow(false);
    };
  }, [delayMs]);
  if (!show) {
    return <div className="globe-splash__placeholder" aria-hidden />;
  }
  return <GlobeSplash variant="route" />;
}
