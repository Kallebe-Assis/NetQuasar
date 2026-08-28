type InstagramIconProps = {
  size?: number;
  className?: string;
};

/**
 * Ícone do Instagram — lucide-react não inclui ícones de marca, então este é um SVG
 * próprio (glifo clássico: quadrado arredondado + círculo + ponto), no mesmo estilo
 * (stroke, sem preenchimento) dos ícones lucide usados no resto do projecto.
 */
export function InstagramIcon({ size = 16, className }: InstagramIconProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden
    >
      <rect x="2" y="2" width="20" height="20" rx="5" ry="5" />
      <path d="M16 11.37A4 4 0 1 1 12.63 8 4 4 0 0 1 16 11.37z" />
      <line x1="17.5" y1="6.5" x2="17.51" y2="6.5" />
    </svg>
  );
}
