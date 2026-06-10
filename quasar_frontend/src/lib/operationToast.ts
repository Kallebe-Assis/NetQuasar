import type { AppToastTone } from "./appToast";
import { errorMessageFromUnknown } from "./apiErrors";

export type ToastPush = (input: {
  tone: AppToastTone;
  text: string;
  loading?: boolean;
  autoMs?: number;
}) => string;

/** Toast de sucesso após operação concluída. */
export function toastOk(push: ToastPush, text: string): void {
  push({ tone: "ok", text });
}

/** Toast informativo (avisos parciais, dicas). */
export function toastInfo(push: ToastPush, text: string): void {
  push({ tone: "info", text });
}

/** Toast de erro com mensagem amigável. */
export function toastErr(push: ToastPush, e: unknown, fallback = "Operação falhou."): void {
  push({ tone: "err", text: errorMessageFromUnknown(e) || fallback });
}

/** Inicia toast de loading; devolve id para dismiss no fim. */
export function toastLoading(push: ToastPush, text: string): string {
  return push({ tone: "info", text, loading: true, autoMs: 0 });
}
