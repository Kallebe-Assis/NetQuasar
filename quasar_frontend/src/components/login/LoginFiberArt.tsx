import { useEffect, useRef } from "react";

/**
 * Ilustração animada da tela de login: um feixe de fibras ópticas que sobe de uma base luminosa e se abre em leque.
 * Canvas 2D (leve): cada fibra é uma curva de Bézier que balança devagar; pulsos de luz correm pelas fibras, as pontas
 * piscam suavemente e pequenas partículas flutuam. Tudo discreto — a ideia é dar vida ao fundo sem competir com o formulário.
 *
 * Cuidados: respeita `prefers-reduced-motion` (desenha um único quadro parado), pausa quando a aba está escondida ou o
 * desenho sai da tela, limita a ~40 quadros/s e ajusta a resolução ao `devicePixelRatio`.
 */

type Strand = {
  angle: number; // abertura (rad) a partir da vertical
  reach: number; // quão longe/alto chega (0.45–1.05)
  hue: number; // 205 (azul) → 300 (magenta)
  alpha: number;
  phase: number;
  swayAmp: number;
  pulse: number; // posição do pulso (0–1)
  pulseSpeed: number;
  hasPulse: boolean;
};
type Speck = { x: number; y: number; r: number; vy: number; tw: number; hue: number };

const FRAME_MS = 1000 / 40;

function rand(a: number, b: number) {
  return a + Math.random() * (b - a);
}

/** ponto de uma Bézier cúbica em t */
function bez(t: number, p0: number, p1: number, p2: number, p3: number) {
  const u = 1 - t;
  return u * u * u * p0 + 3 * u * u * t * p1 + 3 * u * t * t * p2 + t * t * t * p3;
}

function makeGlowSprite(hue: number): HTMLCanvasElement {
  const s = document.createElement("canvas");
  s.width = s.height = 64;
  const c = s.getContext("2d")!;
  const g = c.createRadialGradient(32, 32, 0, 32, 32, 32);
  g.addColorStop(0, `hsla(${hue}, 100%, 88%, 1)`);
  g.addColorStop(0.25, `hsla(${hue}, 100%, 70%, 0.55)`);
  g.addColorStop(1, `hsla(${hue}, 100%, 60%, 0)`);
  c.fillStyle = g;
  c.fillRect(0, 0, 64, 64);
  return s;
}

export function LoginFiberArt() {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const sprites = [makeGlowSprite(215), makeGlowSprite(265), makeGlowSprite(305)];
    let w = 0;
    let h = 0;
    let raf = 0;
    let lastDraw = 0;
    let visible = true;
    let strands: Strand[] = [];
    let specks: Speck[] = [];

    function build() {
      const n = w < 420 ? 70 : 120;
      strands = Array.from({ length: n }, () => {
        const angle = rand(-1.12, 1.12);
        return {
          angle,
          reach: rand(0.5, 1.05),
          hue: 205 + ((angle + 1.12) / 2.24) * 95 + rand(-12, 12),
          alpha: rand(0.22, 0.6),
          phase: rand(0, Math.PI * 2),
          swayAmp: rand(2, 9),
          pulse: Math.random(),
          pulseSpeed: rand(0.08, 0.22),
          hasPulse: Math.random() < 0.55,
        };
      });
      specks = Array.from({ length: 36 }, () => ({
        x: Math.random() * w,
        y: Math.random() * h,
        r: rand(0.6, 1.8),
        vy: rand(4, 14),
        tw: rand(0, Math.PI * 2),
        hue: rand(215, 300),
      }));
    }

    function resize() {
      const rect = canvas!.getBoundingClientRect();
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      w = Math.max(1, Math.round(rect.width));
      h = Math.max(1, Math.round(rect.height));
      canvas!.width = Math.round(w * dpr);
      canvas!.height = Math.round(h * dpr);
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
      build();
      draw(performance.now(), true);
    }

    function draw(now: number, still = false) {
      const t = still ? 0 : now / 1000;
      const c = ctx!;
      c.clearRect(0, 0, w, h);
      const cx = w * 0.5;
      const cy = h * 0.8;
      const H = h * 0.78;
      const R = w * 0.5;

      // halo atrás do feixe
      const halo = c.createRadialGradient(cx, cy - H * 0.35, 0, cx, cy - H * 0.35, Math.max(w, h) * 0.55);
      halo.addColorStop(0, "rgba(139, 92, 246, 0.28)");
      halo.addColorStop(0.5, "rgba(79, 70, 229, 0.10)");
      halo.addColorStop(1, "rgba(79, 70, 229, 0)");
      c.fillStyle = halo;
      c.fillRect(0, 0, w, h);

      // partículas flutuando
      c.globalCompositeOperation = "lighter";
      for (const s of specks) {
        if (!still) {
          s.y -= s.vy * (1 / 40);
          if (s.y < -4) {
            s.y = h + 4;
            s.x = Math.random() * w;
          }
        }
        const a = 0.25 + 0.25 * Math.sin(t * 1.4 + s.tw);
        c.fillStyle = `hsla(${s.hue}, 100%, 78%, ${a})`;
        c.beginPath();
        c.arc(s.x, s.y, s.r, 0, Math.PI * 2);
        c.fill();
      }

      // fibras
      c.lineCap = "round";
      for (const s of strands) {
        const sin = Math.sin(s.angle);
        const cos = Math.cos(s.angle);
        const sway = Math.sin(t * 0.6 + s.phase) * s.swayAmp;
        const x1 = cx + sin * R * 0.1;
        const y1 = cy - H * 0.5 * (0.7 + s.reach * 0.4);
        const x3 = cx + sin * R * (0.5 + s.reach * 0.6) + sway;
        const y3 = cy - cos * H * (0.42 + s.reach * 0.55) - Math.abs(sin) * H * 0.05 + sway * 0.4;
        const x2 = x3 - sin * R * 0.12 + sway * 0.6;
        const y2 = y3 - H * 0.22;
        c.strokeStyle = `hsla(${s.hue}, 95%, 66%, ${s.alpha})`;
        c.lineWidth = 0.55 + s.reach * 0.55;
        c.beginPath();
        c.moveTo(cx, cy);
        c.bezierCurveTo(x1, y1, x2, y2, x3, y3);
        c.stroke();

        // ponta que respira
        const sprite = sprites[s.hue < 240 ? 0 : s.hue < 285 ? 1 : 2];
        const tip = 5 + 6 * (0.5 + 0.5 * Math.sin(t * 1.3 + s.phase * 2));
        c.globalAlpha = 0.75;
        c.drawImage(sprite, x3 - tip, y3 - tip, tip * 2, tip * 2);

        // pulso de luz percorrendo a fibra
        if (s.hasPulse) {
          if (!still) s.pulse = (s.pulse + s.pulseSpeed / 40) % 1;
          const px = bez(s.pulse, cx, x1, x2, x3);
          const py = bez(s.pulse, cy, y1, y2, y3);
          const k = 7;
          c.globalAlpha = 0.95;
          c.drawImage(sprite, px - k, py - k, k * 2, k * 2);
        }
        c.globalAlpha = 1;
      }

      // base luminosa (pedestal elíptico)
      const pulse = 0.5 + 0.5 * Math.sin(t * 1.1);
      const rx = w * 0.2;
      const ry = Math.max(6, h * 0.045);
      const grad = c.createRadialGradient(cx, cy, 0, cx, cy, rx);
      grad.addColorStop(0, `rgba(196, 181, 253, ${0.55 + pulse * 0.25})`);
      grad.addColorStop(0.55, "rgba(124, 58, 237, 0.35)");
      grad.addColorStop(1, "rgba(76, 29, 149, 0)");
      c.fillStyle = grad;
      c.beginPath();
      c.ellipse(cx, cy, rx * 1.25, ry * 1.4, 0, 0, Math.PI * 2);
      c.fill();
      c.globalCompositeOperation = "source-over";
      c.strokeStyle = `rgba(167, 139, 250, ${0.55 + pulse * 0.3})`;
      c.lineWidth = 1.4;
      c.beginPath();
      c.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2);
      c.stroke();
      c.strokeStyle = "rgba(96, 165, 250, 0.35)";
      c.lineWidth = 1;
      c.beginPath();
      c.ellipse(cx, cy + ry * 0.5, rx * 1.12, ry * 1.15, 0, 0, Math.PI * 2);
      c.stroke();
    }

    function loop(now: number) {
      raf = requestAnimationFrame(loop);
      if (!visible || document.hidden) return;
      if (now - lastDraw < FRAME_MS) return;
      lastDraw = now;
      draw(now);
    }

    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(canvas);
    const io = new IntersectionObserver((es) => {
      visible = es.some((e) => e.isIntersecting);
    });
    io.observe(canvas);
    if (!reduceMotion) raf = requestAnimationFrame(loop);

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      io.disconnect();
    };
  }, []);

  return <canvas ref={ref} className="login-fiber" aria-hidden />;
}
