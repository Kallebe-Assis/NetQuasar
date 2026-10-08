import { Activity, Network, Server } from "lucide-react";
import { LoginFiberArt } from "./LoginFiberArt";

const FEATURES = [
  { icon: Activity, text: "Monitoramento em Tempo Real" },
  { icon: Network, text: "Gestão de OLTs & ONUs" },
  { icon: Server, text: "Análise de Tráfego & BGP" },
];

/** Lado esquerdo da tela de login (só em telas largas): marca, promessa do produto, destaques e a ilustração animada. */
export function LoginHero() {
  return (
    <section className="login-hero" aria-label="Sobre o NetQuasar">
      <header className="login-hero__head">
        <p className="login-hero__title">NetQuasar</p>
        <p className="login-hero__lead">A revolução no monitoramento e gerenciamento de redes e OLTs.</p>
      </header>
      <div className="login-hero__body">
        <ul className="login-hero__features">
          {FEATURES.map((f, i) => (
            <li key={f.text} className="login-hero__feature" style={{ ["--i" as string]: i }}>
              <span className="login-hero__feature-icon" aria-hidden>
                <f.icon size={20} />
              </span>
              <span>{f.text}</span>
            </li>
          ))}
        </ul>
        <div className="login-hero__art">
          <LoginFiberArt />
        </div>
      </div>
      <p className="login-hero__foot">Controle total da sua infraestrutura, na ponta dos dedos.</p>
    </section>
  );
}
