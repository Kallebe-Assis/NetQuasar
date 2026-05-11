export function ToastMessage({ text, tone = "ok" }: { text: string; tone?: "ok" | "err" | "off" }) {
  return (
    <div className={`toast-message toast-message--${tone}`} role="status" aria-live="polite">
      <span className="toast-message__body">{text}</span>
    </div>
  );
}

