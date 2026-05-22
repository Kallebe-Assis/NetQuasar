/** Rótulos e ordem dos campos IXC (su_ticket / su_oss_chamado) para «Ver mais». */

export type SupportDetailKind = "attendance" | "work_order";

const ATTENDANCE_LABELS: Record<string, string> = {
  id: "ID",
  tipo: "Tipo",
  id_estrutura: "Estrutura",
  protocolo: "Protocolo",
  id_circuito: "Circuito",
  id_cliente: "Cliente",
  id_login: "Login",
  id_contrato: "Contrato",
  id_filial: "Filial",
  id_assunto: "Assunto",
  titulo: "Título",
  origem_endereco: "Origem do endereço",
  origem_endereco_estrutura: "Origem endereço (estrutura)",
  endereco: "Endereço",
  latitude: "Latitude",
  longitude: "Longitude",
  id_wfl_processo: "Processo WFL",
  id_ticket_setor: "Setor",
  id_responsavel_tecnico: "Responsável técnico",
  data_criacao: "Data criação",
  data_ultima_alteracao: "Última alteração",
  prioridade: "Prioridade",
  data_reservada: "Data reservada",
  melhor_horario_reserva: "Melhor horário reserva",
  id_ticket_origem: "Origem do ticket",
  id_usuarios: "Usuário",
  id_resposta: "Resposta",
  menssagem: "Mensagem",
  mensagem: "Mensagem",
  interacao_pendente: "Interação pendente",
  su_status: "Status SU",
  id_evento_status_processo: "Evento status",
  id_canal_atendimento: "Canal",
  status: "Status",
  mensagens_nao_lida_cli: "Msgs. não lidas (cliente)",
  mensagens_nao_lida_sup: "Msgs. não lidas (suporte)",
  token: "Token",
  finalizar_atendimento: "Finalizar atendimento",
  id_su_diagnostico: "Diagnóstico",
  status_sla: "Status SLA",
  origem_cadastro: "Origem cadastro",
  updated_user: "Atualizado por",
  ultima_atualizacao: "Última atualização",
  cliente_fone: "Telefone",
  cliente_telefone_comercial: "Tel. comercial",
  cliente_telefone_celular: "Celular",
  cliente_whatsapp: "WhatsApp",
  cliente_email: "E-mail",
  cliente_contato: "Contato",
};

const WORK_ORDER_LABELS: Record<string, string> = {
  id: "ID",
  tipo: "Tipo",
  id_ticket: "Atendimento (ticket)",
  protocolo: "Protocolo",
  id_assunto: "Assunto",
  id_cliente: "Cliente",
  id_estrutura: "Estrutura",
  id_filial: "Filial",
  id_login: "Login",
  id_contrato_kit: "Contrato",
  origem_endereco: "Origem do endereço",
  latitude: "Latitude",
  longitude: "Longitude",
  status_conexao: "Status conexão",
  prioridade: "Prioridade",
  melhor_horario_agenda: "Melhor horário agenda",
  setor: "Setor",
  id_tecnico: "Técnico",
  mensagem: "Mensagem",
  status: "Status",
  status_pesquisa_satisfacao: "Pesquisa satisfação",
  data_abertura: "Data abertura",
  data_inicio: "Data início",
  data_agenda: "Data agenda",
  data_agenda_final: "Data agenda (fim)",
  data_final: "Data final",
  data_fechamento: "Data fechamento",
  data_prazo_limite: "Prazo limite",
  data_reservada: "Data reservada",
  data_reagendar: "Reagendar",
  data_prev_final: "Previsão final",
  mensagem_resposta: "Mensagem resposta",
  valor_total: "Valor total",
  valor_outras_despesas: "Outras despesas",
  valor_total_comissao: "Comissão total",
  endereco: "Endereço",
  bairro: "Bairro",
  complemento: "Complemento",
  referencia: "Referência",
  id_cidade: "Cidade",
  id_condominio: "Condomínio",
  bloco: "Bloco",
  apartamento: "Apartamento",
  regiao_manutencao: "Região manutenção",
  origem_cadastro: "Origem cadastro",
  status_sla: "Status SLA",
  ultima_atualizacao: "Última atualização",
};

const ATTENDANCE_ORDER = [
  "id",
  "protocolo",
  "tipo",
  "titulo",
  "status",
  "su_status",
  "menssagem",
  "mensagem",
  "data_criacao",
  "data_ultima_alteracao",
  "ultima_atualizacao",
  "id_cliente",
  "id_login",
  "id_contrato",
  "id_circuito",
  "id_filial",
  "id_assunto",
  "prioridade",
  "id_responsavel_tecnico",
  "id_ticket_setor",
  "endereco",
  "origem_endereco",
  "latitude",
  "longitude",
  "data_reservada",
  "melhor_horario_reserva",
  "id_canal_atendimento",
  "interacao_pendente",
  "status_sla",
  "cliente_fone",
  "cliente_telefone_celular",
  "cliente_email",
  "cliente_contato",
];

const WORK_ORDER_ORDER = [
  "id",
  "protocolo",
  "tipo",
  "status",
  "mensagem",
  "id_cliente",
  "id_ticket",
  "id_assunto",
  "id_login",
  "id_contrato_kit",
  "id_filial",
  "id_tecnico",
  "setor",
  "prioridade",
  "status_conexao",
  "data_abertura",
  "data_inicio",
  "data_agenda",
  "data_agenda_final",
  "data_final",
  "data_fechamento",
  "data_prazo_limite",
  "data_reservada",
  "valor_total",
  "valor_total_comissao",
  "endereco",
  "bairro",
  "complemento",
  "id_cidade",
  "latitude",
  "longitude",
  "mensagem_resposta",
  "status_sla",
  "ultima_atualizacao",
];

const VALUE_MAP: Record<string, Record<string, string>> = {
  tipo: { C: "Cliente", E: "Estrutura própria" },
  origem_endereco: { C: "Cliente", L: "Login", CC: "Contrato", M: "Manual" },
  prioridade: { B: "Baixa", M: "Normal", A: "Alta", C: "Crítica" },
  melhor_horario_reserva: { M: "Manhã", T: "Tarde", N: "Noite", Q: "Qualquer" },
  melhor_horario_agenda: { M: "Manhã", T: "Tarde", N: "Noite", Q: "Qualquer" },
  status_conexao: { S: "Online", N: "Offline" },
  status: {
    A: "Aberta",
    AN: "Análise",
    EN: "Encaminhada",
    AS: "Assumida",
    AG: "Agendada",
    DS: "Deslocamento",
    EX: "Execução",
    F: "Finalizada",
    RAG: "Aguardando agendamento",
    T: "Aberto",
  },
  status_pesquisa_satisfacao: { "1": "Enviado", "2": "Respondida", "3": "Expirada" },
  status_assinatura: { A: "Pendente", F: "Assinada" },
  gera_comissao: { S: "Sim", N: "Não" },
  liberado: { "1": "Sim", "2": "Não" },
  finalizar_atendimento: { S: "Sim", N: "Não" },
  interacao_pendente: { S: "Sim", N: "Não" },
  atualizar_cliente: { S: "Sim", N: "Não" },
  atualizar_login: { S: "Sim", N: "Não" },
};

export function supportFieldLabel(kind: SupportDetailKind, key: string): string {
  const map = kind === "attendance" ? ATTENDANCE_LABELS : WORK_ORDER_LABELS;
  return map[key] ?? key.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function formatSupportFieldValue(key: string, value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "boolean") return value ? "Sim" : "Não";
  if (typeof value === "object") return JSON.stringify(value, null, 2);
  const s = String(value).trim();
  if (!s) return "";
  const mapped = VALUE_MAP[key]?.[s];
  return mapped ?? s;
}

export function orderedRawEntries(
  raw: Record<string, unknown> | undefined,
  kind: SupportDetailKind,
): [string, unknown][] {
  if (!raw) return [];
  const order = kind === "attendance" ? ATTENDANCE_ORDER : WORK_ORDER_ORDER;
  const seen = new Set<string>();
  const out: [string, unknown][] = [];
  for (const key of order) {
    if (key in raw && formatSupportFieldValue(key, raw[key]) !== "") {
      out.push([key, raw[key]]);
      seen.add(key);
    }
  }
  const rest = Object.keys(raw)
    .filter((k) => !seen.has(k) && formatSupportFieldValue(k, raw[k]) !== "")
    .sort((a, b) => a.localeCompare(b));
  for (const key of rest) {
    out.push([key, raw[key]]);
  }
  return out;
}
