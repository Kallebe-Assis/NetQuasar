import type { ReactNode } from "react";
import type { SystemReportId } from "./systemReports";

export type SystemReportInfo = {
  delivers: string;
  formats: string;
  columns?: string;
  notes?: string;
};

export const SYSTEM_REPORT_INFO: Record<SystemReportId, SystemReportInfo> = {
  "active-alerts": {
    delivers: "Todos os alertas em aberto no sistema, com equipamento, severidade e mensagem.",
    formats: "Pré-visualização na aplicação, CSV, PDF e Telegram (texto simples).",
    columns: "Equipamento, IP, Severidade, Tipo, Categoria, POP, Desde, Mensagem",
  },
  connections: {
    delivers: "Conexões cadastradas ou logins PPPoE do cache BNG, em formato resumido (totais) ou detalhado.",
    formats: "Pré-visualização, CSV, PDF e Telegram.",
    columns: "Cadastro: Nº, Cliente, Login, IP, Tipo, Meio, Plano. BNG: Login, IPv4, MAC, IPv6, Tipo IP, Domínio, VLAN.",
    notes: "Escolha a fonte (cadastro ou cache BNG) e o formato antes de gerar.",
  },
  "equipment-by-pop": {
    delivers: "Equipamentos agrupados por POP, com nome e categoria [Tipo]. Opcionalmente inclui sem POP e coordenadas.",
    formats: "Lista agrupada na aplicação/PDF; CSV tabular; Telegram agrupado por POP.",
    columns: "CSV: POP, Equipamento (e Lat/Lon se activo). Pré-visualização: listas por POP.",
    notes: "Use «Gerar relatório» para escolher opções antes de visualizar ou exportar.",
  },
  "olt-overview": {
    delivers: "Resumo da frota OLT, totais de ONUs e gráfico de evolução no período escolhido.",
    formats: "Resumo + gráfico na aplicação/PDF; CSV com detalhes por OLT; Telegram com totais.",
    columns: "OLT, Marca, PONs, ONUs total, Online, Offline, Snapshot",
    notes: "Período configurável: hoje (por hora), 3, 7 ou 30 dias.",
  },
  "system-general": {
    delivers: "Painel consolidado: equipamentos, POPs, clientes, PONs, Mikrotik, alertas e integrações.",
    formats: "Tabela resumo (métrica/valor); CSV e Telegram com os mesmos totais.",
    columns: "Métrica, Valor",
  },
  integrations: {
    delivers: "Integrações configuradas, URL, autenticação e resultado do último teste.",
    formats: "Tabela detalhada; exportação CSV/PDF/Telegram.",
    columns: "Nome, Slug, URL, Auth, Activa, Último teste, Resultado, Mensagem, Actualizado",
  },
  "attention-devices": {
    delivers: "Equipamentos com lacunas de cadastro ou alertas abertos que precisam de atenção.",
    formats: "Lista com motivo; CSV/PDF/Telegram.",
    columns: "Equipamento, Categoria, IP, Motivo, Detalhe",
  },
  "alerts-by-category": {
    delivers: "Contagem de alertas activos agrupados por categoria operacional, tipo e severidade.",
    formats: "Tabela agregada; CSV/PDF/Telegram.",
    columns: "Categoria, Tipo, Severidade, Quantidade",
  },
  "onu-per-pon": {
    delivers: "Última coleta SNMP por porta PON (sem nova coleta — usa snapshot guardado).",
    formats: "Tabela por OLT/PON; CSV/PDF/Telegram.",
    columns: "OLT, PON, Nome PON, Total, Online, Offline, Snapshot",
  },
  "bng-subscribers": {
    delivers: "Totais de logins PPPoE, IPv4, IPv6 e dual-stack por BNG, com gráfico de 7 dias e médias.",
    formats: "Tabela + gráfico na aplicação/PDF; CSV detalhado; Telegram com totais e médias.",
    columns: "BNG, IP, Última coleta, Total online, PPPoE, IPv4, IPv6, Dual-stack",
  },
};

export function SystemReportInfoContent({ id }: { id: SystemReportId }) {
  const info = SYSTEM_REPORT_INFO[id];
  if (!info) return null;
  return (
    <>
      <p style={{ margin: "0 0 6px" }}>
        <strong>O que entrega:</strong> {info.delivers}
      </p>
      <p style={{ margin: "0 0 6px" }}>
        <strong>Formatos:</strong> {info.formats}
      </p>
      {info.columns ? (
        <p style={{ margin: "0 0 6px" }}>
          <strong>Colunas / estrutura:</strong> {info.columns}
        </p>
      ) : null}
      {info.notes ? (
        <p style={{ margin: 0, color: "var(--muted)" }}>{info.notes}</p>
      ) : null}
    </>
  );
}

export function SystemReportInfoTooltip({ id }: { id: SystemReportId }): ReactNode {
  return <SystemReportInfoContent id={id} />;
}
