import { useEffect, useState } from "react";

const STEPS = [
  "A ligar aos equipamentos…",
  "A agregar telemetria e ping…",
  "A preparar gráficos analíticos…",
  "Quase pronto…",
];

export function DashboardPageLoader() {
  const [step, setStep] = useState(0);

  useEffect(() => {
    const id = window.setInterval(() => setStep((s) => (s + 1) % STEPS.length), 2200);
    return () => window.clearInterval(id);
  }, []);

  return (
    <div className="dashboard-loader" role="status" aria-live="polite" aria-busy="true">
      <div className="dashboard-loader__orb" aria-hidden />
      <div className="dashboard-loader__rings" aria-hidden>
        <span />
        <span />
        <span />
      </div>
      <p className="dashboard-loader__title">A carregar dashboard</p>
      <p className="dashboard-loader__step">{STEPS[step]}</p>
      <div className="dashboard-loader__bars" aria-hidden>
        {Array.from({ length: 12 }).map((_, i) => (
          <span key={i} style={{ animationDelay: `${i * 80}ms` }} />
        ))}
      </div>
      <div className="dashboard-loader__skeleton-grid" aria-hidden>
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="dashboard-loader__skel-card" style={{ animationDelay: `${i * 120}ms` }} />
        ))}
      </div>
    </div>
  );
}
