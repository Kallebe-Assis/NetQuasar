import { useMemo, useState } from "react";
import { CircleHelp, ExternalLink, Mail } from "lucide-react";
import { InstagramIcon } from "../components/icons/InstagramIcon";
import { ABOUT_FAQ, ABOUT_FAQ_CATEGORIES, type AboutFaqCategory } from "../lib/aboutFaq";

const DEVELOPER = {
  name: "Kallebe Assis Nogueira",
  portfolio: "https://portfolio-kallebe-assis.vercel.app/",
  instagram: "kallbeassiss_",
  email: "kallebe.assiss@gmail.com",
  createdAtLabel: "11 de maio de 2026",
};

type TabId = "overview" | "developer" | "stack" | "production" | "backups" | "security" | "faq";

const TABS: { id: TabId; label: string }[] = [
  { id: "overview", label: "Visão geral" },
  { id: "developer", label: "Desenvolvedor" },
  { id: "stack", label: "Tecnologias" },
  { id: "production", label: "Produção & deploy" },
  { id: "backups", label: "Backups & dados" },
  { id: "security", label: "Segurança" },
  { id: "faq", label: "FAQ" },
];

export function AboutPage() {
  const [tab, setTab] = useState<TabId>("overview");
  const [faqQ, setFaqQ] = useState("");
  const [faqCat, setFaqCat] = useState<AboutFaqCategory | "all">("all");

  const faqGrouped = useMemo(() => {
    const q = faqQ.trim().toLowerCase();
    const filtered = ABOUT_FAQ.filter((item) => {
      if (faqCat !== "all" && item.cat !== faqCat) return false;
      if (!q) return true;
      return item.q.toLowerCase().includes(q) || item.a.toLowerCase().includes(q);
    });
    return ABOUT_FAQ_CATEGORIES.map((c) => ({
      ...c,
      items: filtered.filter((i) => i.cat === c.id),
    })).filter((g) => g.items.length > 0);
  }, [faqQ, faqCat]);

  return (
    <div className="about-page">
      <header className="about-page__head about-anim about-anim--1">
        <h1 className="about-page__title">
          Sobre o NetQuasar
          <CircleHelp size={20} className="about-page__title-icon" aria-hidden />
        </h1>
        <p className="about-page__subtitle">Documentação amigável do produto — do conceito à configuração.</p>
      </header>

      <nav className="about-tabs about-anim about-anim--2" aria-label="Secções Sobre">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`about-tabs__btn${tab === t.id ? " is-active" : ""}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </nav>

      <div key={tab} className="about-panel about-anim about-anim--enter">
        {tab === "overview" ? (
          <>
            <div className="about-hero card">
              <div>
                <p className="about-hero__eyebrow">Documentação do software</p>
                <h2 className="about-hero__title">NetQuasar</h2>
                <p className="about-hero__lead">
                  Plataforma de monitoramento e operação de rede para provedores ISP (NOC): inventário, coleta automática,
                  alertas, mapa óptico, integrações e ferramentas de diagnóstico numa interface web única.
                </p>
              </div>
              <dl className="about-hero__meta">
                <div>
                  <dt>Desenvolvedor</dt>
                  <dd>{DEVELOPER.name}</dd>
                </div>
                <div>
                  <dt>Criação</dt>
                  <dd>{DEVELOPER.createdAtLabel}</dd>
                </div>
                <div>
                  <dt>Foco</dt>
                  <dd>NOC · ISP</dd>
                </div>
              </dl>
            </div>

            <section className="card about-section about-section--grid">
              <div>
                <h3>O que é</h3>
                <p>
                  O NetQuasar centraliza a operação do NOC: equipamentos (routers, switches, OLTs, BNG), telemetria SNMP/ICMP,
                  snapshots de interfaces e potenciais ópticos SFP, coleta de ONUs por PON, alertas com limiares configuráveis,
                  notificações (Telegram/SMTP), mapa com infraestrutura de campo e módulos comerciais/integrações.
                </p>
              </div>
              <div>
                <h3>Objectivos</h3>
                <ul>
                  <li>Detectar falhas e degradações cedo (offline, latência, óptica, sessões).</li>
                  <li>Dar contexto geográfico e de inventário (POP, localidade, mapa).</li>
                  <li>Reduzir ruído com confirmações, ignorar padrão e incidentes correlacionados.</li>
                  <li>Acelerar o dia a dia com refresh manual, ferramentas e relatórios.</li>
                </ul>
              </div>
              <div className="about-section__span">
                <h3>Módulos principais</h3>
                <ul className="about-chips">
                  <li>Dashboard / Monitoramento / Tempo real</li>
                  <li>Equipamentos, Localidades, Clientes, Elementos</li>
                  <li>Alertas, Relatórios, Frota</li>
                  <li>OLT, MikroTik, Switch, BNG, BGP, Mapa</li>
                  <li>Ferramentas e Integrações</li>
                  <li>Registros, Eventos da Rede</li>
                  <li>Configurações e Automações</li>
                </ul>
              </div>
            </section>
          </>
        ) : null}

        {tab === "developer" ? (
          <section className="card about-section about-section--wide">
            <h3>Desenvolvedor</h3>
            <p className="about-lead-name">{DEVELOPER.name}</p>
            <p className="row" style={{ gap: 16, flexWrap: "wrap", margin: "0 0 12px" }}>
              <a
                href={`https://instagram.com/${DEVELOPER.instagram}`}
                target="_blank"
                rel="noopener noreferrer"
                className="about-link"
              >
                <InstagramIcon size={14} />@{DEVELOPER.instagram}
              </a>
              <a href={`mailto:${DEVELOPER.email}`} className="about-link">
                <Mail size={14} aria-hidden />
                {DEVELOPER.email}
              </a>
              <a href={DEVELOPER.portfolio} target="_blank" rel="noopener noreferrer" className="about-link">
                <ExternalLink size={14} aria-hidden />
                Portfólio
              </a>
            </p>
            <p>Autor e responsável técnico pelo NetQuasar.</p>
            <h3>Data de criação</h3>
            <p>
              Início do monorepo NetQuasar: <strong>{DEVELOPER.createdAtLabel}</strong>. O produto evolui continuamente com
              melhorias de monitoramento, alertas, mapa e operação ISP.
            </p>
          </section>
        ) : null}

        {tab === "stack" ? (
          <section className="card about-section about-section--wide">
            <h3>Stack tecnológica</h3>
            <div className="about-table-wrap">
              <table className="about-table">
                <thead>
                  <tr>
                    <th>Camada</th>
                    <th>Tecnologia</th>
                    <th>Função</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>Backend</td>
                    <td>Go</td>
                    <td>API REST, workers, SNMP/ICMP/OLT, alertas, integrações</td>
                  </tr>
                  <tr>
                    <td>Frontend</td>
                    <td>React + TypeScript + Vite</td>
                    <td>Painel NOC; dist embutido no binário em produção</td>
                  </tr>
                  <tr>
                    <td>Base de dados</td>
                    <td>PostgreSQL</td>
                    <td>Inventário, histórico, alertas, config, auditoria</td>
                  </tr>
                  <tr>
                    <td>Cache / realtime</td>
                    <td>Redis (recomendado)</td>
                    <td>WebSocket e cache do dashboard</td>
                  </tr>
                  <tr>
                    <td>Deploy</td>
                    <td>Docker Compose / binário</td>
                    <td>API + UI na porta publicada</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <h3>Protocolos e integrações</h3>
            <p>SNMPv2c, ICMP/TCP probing, Telnet (alguns perfis), Telegram Bot API, SMTP, APIs de sistemas externos (ex. IXC/Hubsoft).</p>
          </section>
        ) : null}

        {tab === "production" ? (
          <section className="card about-section about-section--wide">
            <h3>Como é produzido</h3>
            <ol>
              <li>Código no monorepo (<code>quasar_backend</code> + <code>quasar_frontend</code>).</li>
              <li>Frontend: build Vite gera artefactos estáticos.</li>
              <li>Backend: compila Go e embute a UI para servir na mesma origem em produção.</li>
              <li>Migrações Goose aplicadas na arranque da API.</li>
              <li>CI (quando activo) valida versões Go/frontend e fluxos de build.</li>
            </ol>
            <h3>Deploy típico</h3>
            <p>
              Docker Compose com Postgres (+ Redis opcional) e serviço NetQuasar, ou instalação em Debian conforme{" "}
              <code>deploy/linux-debian</code>. Reverse proxy (Nginx/Caddy) para HTTPS.
            </p>
            <h3>Ambiente de desenvolvimento</h3>
            <p>API Go local + Vite com proxy; Postgres/Redis via Docker ou instalação nativa. Variáveis em <code>.env</code>.</p>
          </section>
        ) : null}

        {tab === "backups" ? (
          <section className="card about-section about-section--wide">
            <h3>Política recomendada de backups</h3>
            <ul>
              <li>
                <strong>Diário:</strong> dump completo do PostgreSQL.
              </li>
              <li>
                <strong>Retenção:</strong> 7–30 dias (ou mais, conforme compliance).
              </li>
              <li>
                <strong>Off-site:</strong> cópia para NAS/S3/outro site (responsabilidade do operador).
              </li>
              <li>
                <strong>Teste:</strong> restauro periódico num ambiente de ensaio.
              </li>
            </ul>
            <h3>O que incluir</h3>
            <ul>
              <li>Dump SQL da base (<code>pg_dump</code>).</li>
              <li>Secrets / <code>.env</code> (armazenados de forma segura).</li>
              <li>Volumes/dados auxiliares (ex. discovery SNMP) se usados.</li>
              <li>Export de configurações críticas quando disponível.</li>
            </ul>
            <h3>Exemplo (Docker Compose)</h3>
            <pre className="about-pre">{`docker compose exec -T postgres pg_dump -U quasar netquasar > backup-$(date +%F).sql`}</pre>
            <p>
              Também existem backups de configuração de equipamento em <code>device_config_backups</code> quando essa
              funcionalidade é utilizada.
            </p>
          </section>
        ) : null}

        {tab === "security" ? (
          <section className="card about-section about-section--wide">
            <h3>Segurança operacional</h3>
            <ul>
              <li>Autenticação de usuários e sessão/token.</li>
              <li>Permissões por módulo (admin vs operador).</li>
              <li>Restringir o painel à rede de gestão / VPN.</li>
              <li>HTTPS no reverse proxy.</li>
              <li>Proteger community SNMP e credenciais Telnet/Telegram.</li>
              <li>Auditoria de alterações relevantes.</li>
            </ul>
            <h3>Boas práticas</h3>
            <p>
              Não exponha a API directamente à Internet sem TLS e controlo de acesso. Rode o worker com privilégios
              mínimos necessários para ICMP/SNMP. Mantenha backups e patches do SO/contentores.
            </p>
          </section>
        ) : null}

        {tab === "faq" ? (
          <section className="card about-section about-faq about-section--wide">
            <div className="about-faq__toolbar">
              <input
                className="input about-faq__search"
                type="search"
                placeholder="Pesquisar no FAQ…"
                value={faqQ}
                onChange={(e) => setFaqQ(e.target.value)}
                aria-label="Pesquisar FAQ"
              />
            </div>
            <div className="about-faq__cats" role="tablist" aria-label="Categorias do FAQ">
              <button
                type="button"
                className={`about-faq__cat${faqCat === "all" ? " is-active" : ""}`}
                onClick={() => setFaqCat("all")}
              >
                Todas
              </button>
              {ABOUT_FAQ_CATEGORIES.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  className={`about-faq__cat${faqCat === c.id ? " is-active" : ""}`}
                  onClick={() => setFaqCat(c.id)}
                >
                  {c.label}
                </button>
              ))}
            </div>

            <div className="about-faq__groups">
              {faqGrouped.map((g) => (
                <div key={g.id} className="about-faq__group">
                  <div className="about-faq__group-head">
                    <h3>{g.label}</h3>
                    <p>{g.blurb}</p>
                  </div>
                  <div className="about-faq__list">
                    {g.items.map((item) => (
                      <details key={item.q} className="about-faq__item">
                        <summary>{item.q}</summary>
                        <p>{item.a}</p>
                      </details>
                    ))}
                  </div>
                </div>
              ))}
              {faqGrouped.length === 0 ? (
                <p className="about-faq__empty">Nenhuma pergunta corresponde à pesquisa.</p>
              ) : null}
            </div>
          </section>
        ) : null}
      </div>
    </div>
  );
}
