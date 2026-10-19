import { useEffect, useRef } from "preact/hooks";
import type { ComponentChildren } from "preact";
import { Briefs, type Look } from "./briefs";

export type { Look } from "./briefs";

/** Small card icon: the achievement's own model (or a locked silhouette). */
export function BriefsIcon({ id, look, width = 44 }: { id: string; look: Look; width?: number }) {
  return (
    <svg width={width} height={(width * 160) / 200} viewBox="0 0 200 160" aria-hidden="true" class="ach-icon">
      <Briefs id={id} look={look} />
    </svg>
  );
}

function BriefsFace({ id, look, back, label }: { id: string; look: Look; back?: boolean; label: string }) {
  return (
    <svg viewBox="0 0 200 160" class="viewer-svg" aria-hidden="true">
      <Briefs id={id} look={look} side={back ? "back" : "front"} label={label} big />
    </svg>
  );
}

// ---------- viewer ----------

const IMPULSE = 540; // deg/s added per click
const FRICTION = 1.1; // 1/s, exponential slow-down
const SETTLE_BELOW = 50; // deg/s: below this a spring snaps to the nearest face

/** Spins the element by rotateY: kick(-1) left, kick(1) right. */
function useSpinner() {
  const el = useRef<HTMLDivElement>(null);
  const st = useRef({ angle: 0, v: 0, raf: 0, last: 0 });
  const reduced = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

  const paint = () => {
    if (el.current) el.current.style.transform = `rotateY(${st.current.angle}deg)`;
  };

  const frame = (t: number) => {
    const s = st.current;
    const dt = Math.min((t - s.last) / 1000, 0.05);
    s.last = t;
    s.v *= Math.exp(-FRICTION * dt);
    if (Math.abs(s.v) < SETTLE_BELOW) {
      const target = Math.round(s.angle / 180) * 180;
      s.v += (target - s.angle) * 30 * dt - s.v * 6 * dt; // damped spring
      if (Math.abs(target - s.angle) < 0.2 && Math.abs(s.v) < 1) {
        s.angle = target;
        s.v = 0;
        paint();
        s.raf = 0;
        return;
      }
    }
    s.angle += s.v * dt;
    paint();
    s.raf = requestAnimationFrame(frame);
  };

  const kick = (dir: -1 | 1) => {
    const s = st.current;
    if (reduced) {
      s.angle += dir * 180; // no motion: just flip
      paint();
      return;
    }
    s.v += dir * IMPULSE;
    if (!s.raf) {
      s.last = performance.now();
      s.raf = requestAnimationFrame(frame);
    }
  };

  useEffect(() => () => cancelAnimationFrame(st.current.raf), []);
  return { el, kick };
}

export function AchViewer({
  id,
  look,
  xp,
  closeLabel,
  hint,
  onClose,
  children,
}: {
  id: string;
  look: Look;
  xp: string;
  closeLabel: string;
  hint: string;
  onClose: () => void;
  children: ComponentChildren;
}) {
  const { el, kick } = useSpinner();
  const closeBtn = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const back = document.activeElement as HTMLElement | null;
    closeBtn.current?.focus();
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft") kick(-1);
      else if (e.key === "ArrowRight") kick(1);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
      back?.focus();
    };
  }, [onClose]);

  const spinBySide = (e: MouseEvent) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    kick(e.clientX < r.left + r.width / 2 ? -1 : 1);
  };

  return (
    <div class={`viewer look-${look}`} role="dialog" aria-modal="true" onClick={onClose}>
      <button ref={closeBtn} class="ghost small viewer-close" aria-label={closeLabel} onClick={onClose}>
        ×
      </button>
      <div class="viewer-body" onClick={(e) => e.stopPropagation()}>
        <div class="viewer-stage" onClick={spinBySide} title={hint}>
          <div class="viewer-coin" ref={el}>
            <div class="viewer-face">
              <BriefsFace id={id} look={look} label={xp} />
            </div>
            <div class="viewer-face viewer-back">
              <BriefsFace id={id} look={look} label={xp} back />
            </div>
          </div>
          <span class="viewer-arrow left" aria-hidden="true">⟲</span>
          <span class="viewer-arrow right" aria-hidden="true">⟳</span>
        </div>
        <p class="viewer-hint">{hint}</p>
        <div class="viewer-info">{children}</div>
      </div>
    </div>
  );
}
