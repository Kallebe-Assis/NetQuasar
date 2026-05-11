/** Apenas dígitos. */
export function brPhoneDigitsOnly(s: string): string {
  return s.replace(/\D/g, "");
}

/** null = válido; senão mensagem de erro. */
export function validateBRPhoneMessage(s: string): string | null {
  const d = brPhoneDigitsOnly(s);
  if (d.length !== 10 && d.length !== 11) {
    return "Telefone com DDD: 10 ou 11 dígitos.";
  }
  if (d.slice(0, 2) === "00") {
    return "DDD inválido (use 2 dígitos do código de área, ex.: 11, 85).";
  }
  return null;
}

export function normalizeBRPhoneForApi(s: string): string {
  return brPhoneDigitsOnly(s);
}

/** Ex.: (11) 98765-4321 ou (85) 3333-4444; valores inválidos mostra o texto original. */
export function formatBRPhoneDisplay(value: string | null | undefined): string {
  if (value == null || value === "") return "—";
  const d = brPhoneDigitsOnly(value);
  if (d.length === 11) {
    return `(${d.slice(0, 2)}) ${d.slice(2, 7)}-${d.slice(7)}`;
  }
  if (d.length === 10) {
    return `(${d.slice(0, 2)}) ${d.slice(2, 6)}-${d.slice(6)}`;
  }
  return value;
}
