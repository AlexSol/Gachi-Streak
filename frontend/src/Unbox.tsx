import { useEffect, useMemo, useRef } from "preact/hooks";
import { Briefs, PackageArt, type Look } from "./briefs";
import { schedule, sfx } from "./sfx";

// First-time opening effects. Each one is a fixed CSS timeline plus timed
// sound cues; onDone fires when it ends (or at once on a click = skip).

export type Effect = "unbox" | "spin" | "flex" | "shame";

export const EFFECTS: Effect[] = ["unbox", "spin", "flex", "shame"];

/** Shameful achievements always get "shame"; the rest a random one. */
export function pickEffect(category: string): Effect {
  if (category === "shame") return "shame";
  const pool: Effect[] = ["unbox", "spin", "flex"];
  return pool[Math.floor(Math.random() * pool.length)];
}

const DURATION: Record<Effect, number> = { unbox: 2600, spin: 2700, flex: 2500, shame: 3200 };

const reducedMotion = () =>
  typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

function Art({ id, look, cls = "" }: { id: string; look: Look; cls?: string }) {
  return (
    <svg viewBox="0 0 200 160" class={`fx-art ${cls}`} aria-hidden="true">
      <Briefs id={id} look={look} big />
    </svg>
  );
}

/** Random particles: CSS custom properties drive one shared keyframe. */
function useParticles(n: number, spread: number, seed: string) {
  return useMemo(
    () =>
      Array.from({ length: n }, (_, i) => {
        const angle = (i / n) * Math.PI * 2 + Math.random() * 0.5;
        const dist = spread * (0.55 + Math.random() * 0.6);
        return {
          dx: Math.cos(angle) * dist,
          dy: Math.sin(angle) * dist * 0.8 - spread * 0.15,
          r: Math.random() * 720 - 360,
          d: Math.random() * 0.15,
          i,
        };
      }),
    [n, spread, seed],
  );
}

const CONFETTI = ["#ff8c1a", "#ffd166", "#c43cff", "#2ecc71", "#29e0ff", "#ff4fa3"];

export function Unboxing({
  effect,
  id,
  look,
  flexText,
  stampText,
  skipHint,
  onDone,
}: {
  effect: Effect;
  id: string;
  look: Look;
  flexText: string;
  stampText: string;
  skipHint: string;
  onDone: () => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const confetti = useParticles(36, 190, effect);
  const males = useParticles(14, 170, effect + "m");
  const reduced = reducedMotion();

  useEffect(() => {
    const quake = () => {
      const v = root.current?.closest(".viewer");
      v?.classList.remove("fx-quake");
      void (v as HTMLElement | undefined)?.offsetWidth; // restart the animation
      v?.classList.add("fx-quake");
    };
    const cues: Record<Effect, [number, () => void][]> = {
      unbox: [
        [0, sfx.tap],
        [300, sfx.tap],
        [600, sfx.tap],
        [900, sfx.rip],
        [1050, sfx.pop],
        [1200, sfx.chime],
      ],
      spin: [
        [0, () => sfx.swell(2)],
        [2000, sfx.chime],
        [2000, () => sfx.boom(0.4)],
      ],
      flex: [
        [0, sfx.fall],
        [550, () => sfx.boom(1)],
        [550, quake],
        [950, () => sfx.boom(0.5)],
        [1000, sfx.chime],
      ],
      shame: [
        [100, sfx.glitch],
        [500, sfx.sadTrombone],
        [900, sfx.glitch],
        [2300, () => sfx.boom(0.7)],
        [2300, quake],
      ],
    };
    const total = reduced ? 600 : DURATION[effect];
    const cancel = schedule([...(reduced ? [] : cues[effect]), [total, onDone]]);
    return () => {
      cancel();
      root.current?.closest(".viewer")?.classList.remove("fx-quake");
    };
  }, [effect]);

  const skip = (e: MouseEvent) => {
    e.stopPropagation();
    onDone();
  };

  return (
    <div ref={root} class={`fx fx-${effect}`} onClick={skip} title={skipHint}>
      {effect === "unbox" && (
        <>
          <div class="fx-rays" />
          <Art id={id} look={look} cls="fx-pop" />
          {(["l", "r"] as const).map((side) => (
            <svg key={side} viewBox="0 0 200 160" class={`fx-art fx-half fx-half-${side}`} aria-hidden="true">
              <PackageArt id={id} />
            </svg>
          ))}
          {confetti.map((p) => (
            <i
              key={p.i}
              class="fx-confetti"
              style={{
                "--dx": `${p.dx}px`,
                "--dy": `${p.dy}px`,
                "--r": `${p.r}deg`,
                "--d": `${p.d}s`,
                background: CONFETTI[p.i % CONFETTI.length],
              }}
            />
          ))}
        </>
      )}

      {effect === "spin" && (
        <>
          <div class="fx-spinner">
            <Art id={id} look={look} cls="fx-silhouette" />
            <Art id={id} look={look} cls="fx-fill" />
          </div>
          <div class="fx-ring" />
          <div class="fx-flash" />
        </>
      )}

      {effect === "flex" && (
        <>
          <Art id={id} look={look} cls="fx-drop" />
          {Array.from({ length: 10 }, (_, i) => (
            <i key={i} class="fx-dust" style={{ "--dx": `${(i - 4.5) * 34}px`, "--d": `${(i % 3) * 0.04}s` }} />
          ))}
          {males.map((p) => (
            <i key={p.i} class="fx-male" style={{ "--dx": `${p.dx}px`, "--dy": `${p.dy}px`, "--r": `${p.r / 4}deg`, "--d": `${p.d}s` }}>
              ♂
            </i>
          ))}
          <p class="fx-slam">{flexText}</p>
        </>
      )}

      {effect === "shame" && (
        <>
          <div class="fx-sag">
            <Art id={id} look={look} cls="fx-gray" />
            <Art id={id} look={look} cls="fx-ghost fx-ghost-a" />
            <Art id={id} look={look} cls="fx-ghost fx-ghost-b" />
          </div>
          <div class="fx-scan" />
          <p class="fx-stamp">{stampText}</p>
        </>
      )}
    </div>
  );
}
