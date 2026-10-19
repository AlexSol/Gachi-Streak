import { useEffect, useRef, useState } from "preact/hooks";
import type { Dict } from "./i18n";

// Animated, parallax background for the login screen. Everything is drawn on
// one fixed full-viewport canvas behind the page; the form itself never moves.

export const BG_EFFECTS = ["embers", "grid", "glyphs", "floor", "off"] as const;
export type BgEffect = (typeof BG_EFFECTS)[number];

const KEY = "gs_bg";

type BgPrefs = { effect: BgEffect; strength: number };

function loadPrefs(): BgPrefs {
  const def: BgPrefs = { effect: "embers", strength: 100 };
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? "null");
    if (raw && BG_EFFECTS.includes(raw.effect)) {
      const s = Number(raw.strength);
      return { effect: raw.effect, strength: Number.isFinite(s) ? Math.min(200, Math.max(0, s)) : 100 };
    }
  } catch {
    /* storage may be blocked or hold junk */
  }
  return def;
}

/** Remembered per browser; the controls and the canvas share it. */
export function useBgPrefs() {
  const [prefs, setPrefs] = useState<BgPrefs>(loadPrefs);
  const update = (p: Partial<BgPrefs>) =>
    setPrefs((old) => {
      const next = { ...old, ...p };
      try {
        localStorage.setItem(KEY, JSON.stringify(next));
      } catch {
        /* ignore */
      }
      return next;
    });
  return { ...prefs, update };
}

function useReducedMotion() {
  const q = "(prefers-reduced-motion: reduce)";
  const [reduced, setReduced] = useState(() => window.matchMedia?.(q).matches ?? false);
  useEffect(() => {
    const m = window.matchMedia?.(q);
    if (!m) return;
    const on = () => setReduced(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return reduced;
}

type Ember = { x: number; y: number; z: number; p: number };
type Glyph = { x: number; y: number; z: number; s: string; p: number };

const GLYPHS = ["♂", "♂", "♂", "func", "go", "chan", ":=", "defer", "{}", "err != nil", "commit", "select"];
const R = Math.random;
const clamp01 = (x: number) => Math.max(0, Math.min(1, x));

export function DungeonBg({ effect, strength }: { effect: BgEffect; strength: number }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const reduced = useReducedMotion();
  // Read by the render loop without restarting it on every slider tick.
  const live = useRef({ effect, k: strength / 100, reduced });
  live.current = { effect, k: strength / 100, reduced };
  const kick = useRef<() => void>(() => undefined);

  useEffect(() => {
    const cv = ref.current;
    const cx = cv?.getContext("2d");
    if (!cv || !cx) return;

    let W = 0, H = 0;
    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      W = window.innerWidth;
      H = window.innerHeight;
      cv.width = Math.round(W * dpr);
      cv.height = Math.round(H * dpr);
      cx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };
    resize();

    // Pointer target in [-1, 1], smoothed towards it every frame.
    let tx = 0, ty = 0, mx = 0, my = 0;
    const onMove = (e: PointerEvent) => {
      tx = (e.clientX / W) * 2 - 1;
      ty = (e.clientY / H) * 2 - 1;
    };
    const onLeave = () => { tx = 0; ty = 0; };

    const embers: Ember[] = Array.from({ length: 240 }, () => ({ x: R(), y: R(), z: [0.3, 0.6, 1][Math.floor(R() * 3)], p: R() * 6.28 }));
    const glyphs: Glyph[] = Array.from({ length: 56 }, () => ({ x: R(), y: R(), z: 0.3 + R() * 0.7, s: GLYPHS[Math.floor(R() * GLYPHS.length)], p: R() * 6.28 }));

    let t = 0, last = performance.now(), raf = 0;

    const frame = (now: number) => {
      const { effect, k, reduced } = live.current;
      const dt = Math.min(0.05, (now - last) / 1000);
      last = now;
      // Reduced motion: keep the look, freeze the motion.
      const K = reduced ? Math.min(k, 1) : k;
      if (!reduced) t += dt * (0.3 + 0.7 * K);
      mx += (tx - mx) * 0.06;
      my += (ty - my) * 0.06;
      const px = reduced ? 0 : mx * K, py = reduced ? 0 : my * K, A = Math.min(1.6, K);

      cx.clearRect(0, 0, W, H);
      if (effect === "embers") drawEmbers(cx, W, H, t, px, py, A, K, embers);
      else if (effect === "grid") drawGrid(cx, W, H, t, px, py, A * 0.7);
      else if (effect === "glyphs") drawGlyphs(cx, W, H, t, px, py, A, K, glyphs);
      else if (effect === "floor") drawFloor(cx, W, H, t, px, py, A);

      raf = document.hidden || effect === "off" || reduced ? 0 : requestAnimationFrame(frame);
    };
    const start = () => {
      if (raf) cancelAnimationFrame(raf);
      last = performance.now();
      raf = requestAnimationFrame(frame);
    };

    const onVis = () => { if (!document.hidden) start(); };
    const onResize = () => { resize(); start(); };
    window.addEventListener("resize", onResize);
    window.addEventListener("pointermove", onMove, { passive: true });
    document.addEventListener("pointerleave", onLeave);
    document.addEventListener("visibilitychange", onVis);
    start();
    // A paused loop (hidden tab, "off", reduced motion) needs a restart when settings change.
    kick.current = start;

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", onResize);
      window.removeEventListener("pointermove", onMove);
      document.removeEventListener("pointerleave", onLeave);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, []);

  useEffect(() => {
    kick.current();
  }, [effect, strength, reduced]);

  return <canvas ref={ref} class="dungeon-bg" aria-hidden="true" />;
}

function drawEmbers(cx: CanvasRenderingContext2D, W: number, H: number, t: number, px: number, py: number, A: number, K: number, list: Ember[]) {
  const g = cx.createRadialGradient(W / 2 - px * 40, H * 1.1, 0, W / 2, H, H * 0.9);
  g.addColorStop(0, `rgba(240,120,30,${0.18 * A})`);
  g.addColorStop(1, "rgba(0,0,0,0)");
  cx.fillStyle = g;
  cx.fillRect(0, 0, W, H);
  const n = Math.round(Math.min(list.length, 120 * K * Math.max(1, W / 1400)));
  for (let i = 0; i < n; i++) {
    const e = list[i];
    const y = (((e.y - t * 0.045 * e.z) % 1) + 1) % 1;
    const x = e.x * W + Math.sin(t * 0.8 + e.p) * 14 * e.z - px * 60 * e.z;
    const yy = y * H - py * 35 * e.z;
    const a = clamp01((0.25 + 0.6 * e.z) * A * 0.8) * Math.min(1, y * 3);
    cx.fillStyle = `rgba(${e.z > 0.9 ? 255 : 240},${(120 + e.z * 60) | 0},40,${a})`;
    cx.beginPath();
    cx.arc(x, yy, 1 + e.z * 2.2, 0, 6.28);
    cx.fill();
  }
}

function drawGrid(cx: CanvasRenderingContext2D, W: number, H: number, t: number, px: number, py: number, A: number) {
  const s = 22, cols = Math.ceil(W / s) + 10, rows = Math.ceil(H / s) + 10;
  cx.save();
  cx.translate(W / 2, H / 2);
  cx.transform(1, py * 0.08, px * 0.08, 1, 0, 0);
  cx.rotate(-0.18 + px * 0.03);
  for (let i = -cols / 2; i < cols / 2; i++) {
    for (let j = -rows / 2; j < rows / 2; j++) {
      const w = Math.sin((i + j) * 0.35 - t * 1.6) * 0.5 + 0.5;
      const h = Math.abs((Math.sin(i * 12.9898 + j * 78.233) * 43758.5) % 1);
      const lv = h > 0.55 ? w * h : w * 0.08;
      cx.fillStyle =
        lv > 0.45 ? `rgba(240,138,28,${clamp01((0.25 + lv * 0.5) * A)})`
        : lv > 0.2 ? `rgba(192,79,232,${clamp01((0.15 + lv * 0.4) * A)})`
        : "rgba(255,255,255,.035)";
      cx.fillRect(i * s - px * 30, j * s - py * 30, s - 5, s - 5);
    }
  }
  cx.restore();
  const vg = cx.createRadialGradient(W / 2, H / 2, Math.min(W, H) * 0.2, W / 2, H / 2, Math.max(W, H) * 0.7);
  vg.addColorStop(0, "rgba(13,10,11,.55)");
  vg.addColorStop(1, "rgba(13,10,11,.95)");
  cx.fillStyle = vg;
  cx.fillRect(0, 0, W, H);
}

function drawGlyphs(cx: CanvasRenderingContext2D, W: number, H: number, t: number, px: number, py: number, A: number, K: number, list: Glyph[]) {
  cx.textAlign = "center";
  const n = Math.round(Math.min(list.length, 28 * K * Math.max(1, W / 1400)));
  for (let i = 0; i < n; i++) {
    const g = list[i];
    const y = (((g.y - t * 0.012 * g.z) % 1) + 1) % 1;
    const x = g.x * W + Math.sin(t * 0.4 + g.p) * 24 - px * 80 * g.z;
    const yy = y * H - py * 50 * g.z;
    cx.font = `600 ${12 + g.z * 26}px ui-monospace, Consolas, monospace`;
    cx.fillStyle = g.s === "♂"
      ? `rgba(240,138,28,${clamp01((0.12 + g.z * 0.4) * A)})`
      : `rgba(192,79,232,${clamp01((0.08 + g.z * 0.3) * A)})`;
    cx.save();
    cx.translate(x, yy);
    cx.rotate(Math.sin(t * 0.3 + g.p) * 0.2);
    cx.fillText(g.s, 0, 0);
    cx.restore();
  }
}

function drawFloor(cx: CanvasRenderingContext2D, W: number, H: number, t: number, px: number, py: number, A: number) {
  const hz = H * 0.45 - py * 30, vx = W / 2 - px * 90;
  const fg = cx.createLinearGradient(0, 0, 0, hz);
  fg.addColorStop(0, "rgba(0,0,0,0)");
  fg.addColorStop(1, `rgba(150,50,200,${0.18 * A})`);
  cx.fillStyle = fg;
  cx.fillRect(0, 0, W, hz);
  cx.lineWidth = 1;
  cx.strokeStyle = `rgba(240,138,28,${clamp01(0.35 * A)})`;
  for (let i = -24; i <= 24; i++) {
    cx.beginPath();
    cx.moveTo(vx, hz);
    cx.lineTo(vx + i * W * 0.1, H);
    cx.stroke();
  }
  for (let k = 0; k < 16; k++) {
    const z = ((k + t * 0.5) % 16) / 16;
    const y = hz + (H - hz) * Math.pow(z, 2.2);
    cx.strokeStyle = `rgba(240,138,28,${clamp01((0.08 + z * 0.4) * A)})`;
    cx.beginPath();
    cx.moveTo(0, y);
    cx.lineTo(W, y);
    cx.stroke();
  }
  cx.fillStyle = `rgba(192,79,232,${clamp01(0.5 * A)})`;
  cx.fillRect(0, hz - 1, W, 1);
}

/** Effect switcher + strength slider shown under the login card. */
export function BgControls({ t, effect, strength, update }: { t: Dict; effect: BgEffect; strength: number; update: (p: Partial<BgPrefs>) => void }) {
  return (
    <div class="bg-controls" role="group" aria-label={t.bgLabel}>
      <div class="bg-row">
        <span class="bg-title">{t.bgLabel}</span>
        <span class="bg-active">{t.bgActive}: {t.bgNames[effect]}</span>
      </div>
      <div class="bg-seg">
        {BG_EFFECTS.map((e) => (
          <button
            key={e}
            type="button"
            class={"ghost small" + (e === effect ? " on" : "")}
            aria-pressed={e === effect}
            onClick={() => update({ effect: e })}
          >
            {t.bgNames[e]}
          </button>
        ))}
      </div>
      <label class="bg-row" for="bg-strength">
        <span>{t.bgStrength}</span>
        <input
          id="bg-strength"
          type="range"
          min={0}
          max={200}
          step={5}
          value={strength}
          disabled={effect === "off"}
          onInput={(e) => update({ strength: Number((e.target as HTMLInputElement).value) })}
        />
        <span class="bg-val">{strength}%</span>
      </label>
    </div>
  );
}
