import { useEffect, useRef, useState } from "preact/hooks";
import type { Dict } from "./i18n";

// Animated background for the login screen. Everything is drawn on one fixed
// full-viewport canvas behind the page; the form itself never moves with it.
// Many effects live here on purpose: they are A/B-tested for a few days and the
// winners stay.

export const BG_EFFECTS = [
  "embers", "grid", "glyphs", "floor",
  "spotlight", "magnet", "ripple",
  "aurora", "flow", "metaballs",
  "matrix", "crt", "warp",
  "fire", "gears",
  "daily", "off",
] as const;
export type BgEffect = (typeof BG_EFFECTS)[number];
type Drawable = Exclude<BgEffect, "daily" | "off">;
const POOL = BG_EFFECTS.filter((e): e is Drawable => e !== "daily" && e !== "off");

/** Same effect for the whole local day, a different one tomorrow. */
export function dailyEffect(d = new Date()): Drawable {
  const key = `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
  let h = 2166136261;
  for (let i = 0; i < key.length; i++) h = Math.imul(h ^ key.charCodeAt(i), 16777619);
  return POOL[(h >>> 0) % POOL.length];
}
export const resolveEffect = (e: BgEffect): Drawable | "off" => (e === "daily" ? dailyEffect() : e);

// "Jump" impulse (warp speed-up, fire flare…) fired by the login form.
const BOOST_MS = 1600;
let boostAt = 0;
export function bgBoost() {
  boostAt = performance.now();
}

// ---------- prefs ----------

const KEY = "gs_bg";
export type BgPrefs = { effect: BgEffect; strength: number; formFx: boolean };

function loadPrefs(): BgPrefs {
  const def: BgPrefs = { effect: "embers", strength: 100, formFx: true };
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? "null");
    if (raw && BG_EFFECTS.includes(raw.effect)) {
      const s = Number(raw.strength);
      return {
        effect: raw.effect,
        strength: Number.isFinite(s) ? Math.min(200, Math.max(0, s)) : 100,
        formFx: raw.formFx !== false,
      };
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
  return { prefs, update };
}

const RM = "(prefers-reduced-motion: reduce)";
export const prefersReducedMotion = () => window.matchMedia?.(RM).matches ?? false;

function useReducedMotion() {
  const [reduced, setReduced] = useState(prefersReducedMotion);
  useEffect(() => {
    const m = window.matchMedia?.(RM);
    if (!m) return;
    const on = () => setReduced(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return reduced;
}

// ---------- effect plumbing ----------

type Ptr = { x: number; y: number; rx: number; ry: number; active: boolean; moved: number };
type Ctx = {
  cx: CanvasRenderingContext2D;
  W: number;
  H: number;
  t: number; // animation time, already scaled by strength
  dt: number; // real frame time, s
  dts: number; // frame time scaled by strength
  now: number; // wall clock, s
  px: number; // smoothed pointer for parallax, [-1, 1] × strength
  py: number;
  A: number; // brightness multiplier
  K: number; // strength, 0..2
  boost: number; // 0..1, decays after bgBoost()
  ptr: Ptr; // pointer in pixels (x/y smoothed, rx/ry raw)
  clicks: { x: number; y: number }[]; // pointerdowns since last frame
};
// `trail` > 0: fade the previous frame instead of clearing it (motion trails).
type Effect<S = any> = { trail?: number; init?: (c: Ctx) => S; draw: (c: Ctx, s: S) => void };

const R = Math.random;
const TAU = Math.PI * 2;
const clamp01 = (x: number) => Math.max(0, Math.min(1, x));
const orange = (a: number) => `rgba(240,138,28,${clamp01(a)})`;
const purple = (a: number) => `rgba(192,79,232,${clamp01(a)})`;

function offscreen(w: number, h: number) {
  const oc = document.createElement("canvas");
  oc.width = w;
  oc.height = h;
  const octx = oc.getContext("2d")!;
  return { oc, octx, img: octx.createImageData(w, h) };
}

// ---------- effects ----------

type Ember = { x: number; y: number; z: number; p: number };
const embers: Effect<Ember[]> = {
  init: () => Array.from({ length: 240 }, () => ({ x: R(), y: R(), z: [0.3, 0.6, 1][(R() * 3) | 0], p: R() * TAU })),
  draw({ cx, W, H, t, px, py, A, K }, list) {
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
      const a = clamp01((0.25 + 0.6 * e.z) * A * 0.8) * Math.min(1, y * 3);
      cx.fillStyle = `rgba(${e.z > 0.9 ? 255 : 240},${(120 + e.z * 60) | 0},40,${a})`;
      cx.beginPath();
      cx.arc(x, y * H - py * 35 * e.z, 1 + e.z * 2.2, 0, TAU);
      cx.fill();
    }
  },
};

const grid: Effect = {
  draw({ cx, W, H, t, px, py, A: A0 }) {
    const A = A0 * 0.7;
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
        cx.fillStyle = lv > 0.45 ? orange((0.25 + lv * 0.5) * A) : lv > 0.2 ? purple((0.15 + lv * 0.4) * A) : "rgba(255,255,255,.035)";
        cx.fillRect(i * s - px * 30, j * s - py * 30, s - 5, s - 5);
      }
    }
    cx.restore();
    const vg = cx.createRadialGradient(W / 2, H / 2, Math.min(W, H) * 0.2, W / 2, H / 2, Math.max(W, H) * 0.7);
    vg.addColorStop(0, "rgba(13,10,11,.55)");
    vg.addColorStop(1, "rgba(13,10,11,.95)");
    cx.fillStyle = vg;
    cx.fillRect(0, 0, W, H);
  },
};

const GLYPHS = ["♂", "♂", "♂", "func", "go", "chan", ":=", "defer", "{}", "err != nil", "commit", "select"];
type Glyph = { x: number; y: number; z: number; s: string; p: number };
const glyphs: Effect<Glyph[]> = {
  init: () => Array.from({ length: 56 }, () => ({ x: R(), y: R(), z: 0.3 + R() * 0.7, s: GLYPHS[(R() * GLYPHS.length) | 0], p: R() * TAU })),
  draw({ cx, W, H, t, px, py, A, K }, list) {
    cx.textAlign = "center";
    const n = Math.round(Math.min(list.length, 28 * K * Math.max(1, W / 1400)));
    for (let i = 0; i < n; i++) {
      const g = list[i];
      const y = (((g.y - t * 0.012 * g.z) % 1) + 1) % 1;
      cx.font = `600 ${12 + g.z * 26}px ui-monospace, Consolas, monospace`;
      cx.fillStyle = g.s === "♂" ? orange((0.12 + g.z * 0.4) * A) : purple((0.08 + g.z * 0.3) * A);
      cx.save();
      cx.translate(g.x * W + Math.sin(t * 0.4 + g.p) * 24 - px * 80 * g.z, y * H - py * 50 * g.z);
      cx.rotate(Math.sin(t * 0.3 + g.p) * 0.2);
      cx.fillText(g.s, 0, 0);
      cx.restore();
    }
  },
};

const floor: Effect = {
  draw({ cx, W, H, t, px, py, A }) {
    const hz = H * 0.45 - py * 30, vx = W / 2 - px * 90;
    const fg = cx.createLinearGradient(0, 0, 0, hz);
    fg.addColorStop(0, "rgba(0,0,0,0)");
    fg.addColorStop(1, `rgba(150,50,200,${0.18 * A})`);
    cx.fillStyle = fg;
    cx.fillRect(0, 0, W, hz);
    cx.lineWidth = 1;
    cx.strokeStyle = orange(0.35 * A);
    cx.beginPath();
    for (let i = -24; i <= 24; i++) {
      cx.moveTo(vx, hz);
      cx.lineTo(vx + i * W * 0.1, H);
    }
    cx.stroke();
    for (let k = 0; k < 16; k++) {
      const z = ((k + t * 0.5) % 16) / 16;
      const y = hz + (H - hz) * Math.pow(z, 2.2);
      cx.strokeStyle = orange((0.08 + z * 0.4) * A);
      cx.beginPath();
      cx.moveTo(0, y);
      cx.lineTo(W, y);
      cx.stroke();
    }
    cx.fillStyle = purple(0.5 * A);
    cx.fillRect(0, hz - 1, W, 1);
  },
};

function bricks(cx: CanvasRenderingContext2D, W: number, H: number, color: string) {
  const bw = 64, bh = 28;
  cx.beginPath();
  for (let row = 0, y = 0; y < H; row++, y += bh) {
    for (let x = row % 2 ? -bw / 2 : 0; x < W; x += bw) cx.rect(x + 1.5, y + 1.5, bw - 3, bh - 3);
  }
  cx.strokeStyle = color;
  cx.lineWidth = 1;
  cx.stroke();
}

// Torch light following the pointer over a brick wall; wanders when idle.
const spotlight: Effect<{ x: number; y: number }> = {
  init: ({ W, H }) => ({ x: W / 2, y: H * 0.4 }),
  draw({ cx, W, H, t, now, A, K, ptr }, s) {
    const idle = !ptr.active || now - ptr.moved > 3;
    const gx = idle ? W * (0.5 + 0.32 * Math.sin(t * 0.23)) : ptr.x;
    const gy = idle ? H * (0.42 + 0.25 * Math.sin(t * 0.31 + 1)) : ptr.y;
    s.x += (gx - s.x) * 0.08;
    s.y += (gy - s.y) * 0.08;
    const r = (160 + 180 * Math.min(K, 2)) * (1 + 0.03 * Math.sin(now * 13) + 0.02 * Math.sin(now * 29));
    bricks(cx, W, H, orange(0.55 * A));
    cx.globalCompositeOperation = "destination-in";
    const m = cx.createRadialGradient(s.x, s.y, 0, s.x, s.y, r);
    m.addColorStop(0, "rgba(0,0,0,1)");
    m.addColorStop(1, "rgba(0,0,0,0)");
    cx.fillStyle = m;
    cx.fillRect(0, 0, W, H);
    cx.globalCompositeOperation = "source-over";
    const g = cx.createRadialGradient(s.x, s.y, 0, s.x, s.y, r * 1.3);
    g.addColorStop(0, `rgba(255,150,60,${clamp01(0.16 * A)})`);
    g.addColorStop(1, "rgba(255,150,60,0)");
    cx.fillStyle = g;
    cx.fillRect(0, 0, W, H);
    bricks(cx, W, H, orange(0.035 * A));
  },
};

// Dot field that the pointer pushes away; clicks send a shockwave.
type Dot = { hx: number; hy: number; x: number; y: number; vx: number; vy: number };
const magnet: Effect<Dot[]> = {
  init: ({ W, H }) => {
    const g = 30, pts: Dot[] = [];
    for (let y = g / 2; y < H; y += g)
      for (let x = g / 2; x < W; x += g) {
        const hx = x + (R() - 0.5) * 8, hy = y + (R() - 0.5) * 8;
        pts.push({ hx, hy, x: hx, y: hy, vx: 0, vy: 0 });
      }
    return pts;
  },
  draw({ cx, t, dts, A, K, ptr, clicks }, pts) {
    const dt = Math.min(dts, 0.033);
    const rad = 90 + 90 * Math.min(K, 2);
    const damp = Math.exp(-7 * dt);
    for (const p of pts) {
      const hx = p.hx + Math.sin(t * 0.6 + p.hy * 0.02) * 4, hy = p.hy + Math.cos(t * 0.5 + p.hx * 0.02) * 4;
      p.vx += (hx - p.x) * 40 * dt;
      p.vy += (hy - p.y) * 40 * dt;
      if (ptr.active) {
        const dx = p.x - ptr.rx, dy = p.y - ptr.ry, d = Math.hypot(dx, dy);
        if (d < rad && d > 0.1) {
          const f = ((1 - d / rad) ** 2 * 2600 * K * dt) / d;
          p.vx += dx * f;
          p.vy += dy * f;
        }
      }
      for (const k of clicks) {
        const dx = p.x - k.x, dy = p.y - k.y, d = Math.hypot(dx, dy) + 1;
        if (d < 320) {
          const f = ((1 - d / 320) * 900) / d;
          p.vx += dx * f;
          p.vy += dy * f;
        }
      }
      p.vx *= damp;
      p.vy *= damp;
      p.x += p.vx * dt;
      p.y += p.vy * dt;
    }
    cx.fillStyle = purple(0.35 * A);
    for (const p of pts) if (Math.abs(p.x - p.hx) + Math.abs(p.y - p.hy) < 8) cx.fillRect(p.x - 1, p.y - 1, 2, 2);
    cx.fillStyle = `rgba(255,150,50,${clamp01(0.8 * A)})`;
    for (const p of pts) if (Math.abs(p.x - p.hx) + Math.abs(p.y - p.hy) >= 8) cx.fillRect(p.x - 1.5, p.y - 1.5, 3, 3);
  },
};

// Rings from clicks, from pointer movement and from random "drops".
type Ring = { x: number; y: number; at: number; str: number; hue: number };
const ripple: Effect<{ rings: Ring[]; lx: number; ly: number; next: number }> = {
  init: () => ({ rings: [], lx: -1e4, ly: -1e4, next: 0 }),
  draw({ cx, W, H, t, A, ptr, clicks, px, py }, s) {
    for (const k of clicks) s.rings.push({ x: k.x, y: k.y, at: t, str: 1, hue: 0 });
    if (ptr.active && Math.hypot(ptr.rx - s.lx, ptr.ry - s.ly) > 90) {
      s.rings.push({ x: ptr.rx, y: ptr.ry, at: t, str: 0.45, hue: 1 });
      s.lx = ptr.rx;
      s.ly = ptr.ry;
    }
    if (t > s.next) {
      s.rings.push({ x: R() * W, y: R() * H, at: t, str: 0.6, hue: R() < 0.5 ? 0 : 1 });
      s.next = t + 0.5 + R() * 1.2;
    }
    const life = 3.2, sp = 150;
    s.rings = s.rings.filter((r) => t - r.at < life).slice(-60);
    cx.lineWidth = 1.5;
    for (const r of s.rings) {
      const age = t - r.at, k = 1 - age / life;
      for (let j = 0; j < 3; j++) {
        const rad = age * sp - j * 16;
        if (rad <= 0) continue;
        const a = k * k * r.str * A * (0.7 - j * 0.2);
        cx.strokeStyle = r.hue ? purple(a) : orange(a);
        cx.beginPath();
        cx.arc(r.x - px * 20, r.y - py * 20, rad, 0, TAU);
        cx.stroke();
      }
    }
  },
};

// Slow additive colour blobs, Stripe/Linear-style, in dungeon colours.
const BLOBS = [
  { c: "255,120,30", r: 0.55, sx: 0.13, sy: 0.17, ph: 0, z: 0.4 },
  { c: "196,60,255", r: 0.5, sx: 0.11, sy: 0.09, ph: 2, z: 0.7 },
  { c: "190,40,70", r: 0.45, sx: 0.07, sy: 0.15, ph: 4, z: 0.3 },
  { c: "255,170,60", r: 0.3, sx: 0.19, sy: 0.12, ph: 1, z: 1 },
  { c: "110,40,200", r: 0.4, sx: 0.09, sy: 0.2, ph: 5, z: 0.55 },
];
const aurora: Effect = {
  draw({ cx, W, H, t, px, py, A }) {
    const M = Math.max(W, H);
    cx.globalCompositeOperation = "lighter";
    for (const b of BLOBS) {
      const x = W * (0.5 + 0.38 * Math.sin(t * b.sx * 2 + b.ph)) - px * 90 * b.z;
      const y = H * (0.5 + 0.32 * Math.cos(t * b.sy * 2 + b.ph * 1.3)) - py * 70 * b.z;
      const g = cx.createRadialGradient(x, y, 0, x, y, M * b.r);
      g.addColorStop(0, `rgba(${b.c},${clamp01(0.2 * A)})`);
      g.addColorStop(1, `rgba(${b.c},0)`);
      cx.fillStyle = g;
      cx.fillRect(0, 0, W, H);
    }
    cx.globalCompositeOperation = "source-over";
  },
};

// Smoke: particles following a smooth vector field; the pointer swirls it.
type FlowP = { x: number; y: number; l: number };
const flow: Effect<FlowP[]> = {
  trail: 0.07,
  init: ({ W, H }) => Array.from({ length: 1100 }, () => ({ x: R() * W, y: R() * H, l: R() * 5 })),
  draw({ cx, W, H, t, dts, A, K, ptr }, ps) {
    const n = Math.min(ps.length, Math.round(250 + 450 * K));
    const a1 = new Path2D(), a2 = new Path2D();
    for (let i = 0; i < n; i++) {
      const p = ps[i];
      const a = (Math.sin(p.x * 0.0035 + t * 0.05) + Math.cos(p.y * 0.0042 - t * 0.04) + Math.sin((p.x + p.y) * 0.0021 + t * 0.03)) * 1.7;
      let vx = Math.cos(a) * 70, vy = Math.sin(a) * 70;
      if (ptr.active) {
        const dx = p.x - ptr.x, dy = p.y - ptr.y, d = Math.hypot(dx, dy);
        if (d < 240 && d > 1) {
          const f = (1 - d / 240) * 160;
          vx += (-dy / d) * f;
          vy += (dx / d) * f;
        }
      }
      const nx = p.x + vx * dts, ny = p.y + vy * dts;
      const path = i % 3 ? a1 : a2;
      path.moveTo(p.x, p.y);
      path.lineTo(nx, ny);
      p.x = nx;
      p.y = ny;
      p.l -= dts;
      if (p.l < 0 || nx < 0 || ny < 0 || nx > W || ny > H) {
        p.x = R() * W;
        p.y = R() * H;
        p.l = 2 + R() * 5;
      }
    }
    cx.lineWidth = 1.2;
    cx.strokeStyle = orange(0.35 * A);
    cx.stroke(a1);
    cx.strokeStyle = purple(0.4 * A);
    cx.stroke(a2);
  },
};

// Lava lamp: metaball field rendered at low resolution and upscaled.
type Ball = { x: number; y: number; vx: number; vy: number; r: number; z: number };
type MetaState = ReturnType<typeof offscreen> & { cell: number; gw: number; gh: number; balls: Ball[]; f: Float64Array };
const metaballs: Effect<MetaState> = {
  init: ({ W, H }) => {
    const cell = 6, gw = Math.ceil(W / cell), gh = Math.ceil(H / cell);
    const balls = Array.from({ length: 6 }, () => ({ x: R() * W, y: R() * H, vx: (R() - 0.5) * 90, vy: (R() - 0.5) * 90, r: 60 + R() * 70, z: 0.3 + R() * 0.7 }));
    return { ...offscreen(gw, gh), cell, gw, gh, balls, f: new Float64Array(21) };
  },
  draw({ cx, W, H, dts, A, px, py, ptr }, s) {
    let n = 0;
    for (const b of s.balls) {
      b.x += b.vx * dts;
      b.y += b.vy * dts;
      if (b.x < 0 || b.x > W) b.vx *= -1;
      if (b.y < 0 || b.y > H) b.vy *= -1;
      b.x = Math.max(0, Math.min(W, b.x));
      b.y = Math.max(0, Math.min(H, b.y));
      s.f[n++] = b.x - px * 60 * b.z;
      s.f[n++] = b.y - py * 40 * b.z;
      s.f[n++] = b.r * b.r;
    }
    if (ptr.active) {
      s.f[n++] = ptr.x;
      s.f[n++] = ptr.y;
      s.f[n++] = 80 * 80;
    }
    const d = s.img.data, f = s.f, al = Math.min(1, A);
    let i = 0;
    for (let gy = 0; gy < s.gh; gy++) {
      const wy = gy * s.cell;
      for (let gx = 0; gx < s.gw; gx++, i += 4) {
        const wx = gx * s.cell;
        let v = 0;
        for (let j = 0; j < n; j += 3) {
          const dx = wx - f[j], dy = wy - f[j + 1];
          v += f[j + 2] / (dx * dx + dy * dy + 1);
        }
        if (v < 0.9) d[i + 3] = 0;
        else if (v < 1.15) {
          d[i] = 192;
          d[i + 1] = 79;
          d[i + 2] = 232;
          d[i + 3] = (((v - 0.9) / 0.25) * 90 * al) | 0;
        } else {
          const k = Math.min(1, (v - 1.15) / 1.5);
          d[i] = 255;
          d[i + 1] = (90 + k * 80) | 0;
          d[i + 2] = (30 + k * 30) | 0;
          d[i + 3] = ((80 + k * 60) * al) | 0;
        }
      }
    }
    s.octx.putImageData(s.img, 0, 0);
    cx.imageSmoothingEnabled = true;
    cx.drawImage(s.oc, 0, 0, s.gw * s.cell, s.gh * s.cell);
  },
};

// Matrix rain made of Go keywords and ♂.
const MCHARS = "♂gofuncchandeferselect:={}()<-errnil01";
type Col = { y: number; purple: boolean; last: string; row: number; sp: number };
const matrix: Effect<{ cw: number; cols: Col[] }> = {
  trail: 0.1,
  init: ({ W }) => ({
    cw: 18,
    cols: Array.from({ length: Math.ceil(W / 18) + 2 }, () => ({ y: -R() * 40, purple: R() < 0.18, last: "", row: -1, sp: 0.6 + R() * 0.4 })),
  }),
  draw({ cx, H, dts, A, px, boost }, s) {
    const rh = 20;
    cx.font = "600 16px ui-monospace, Consolas, monospace";
    cx.textAlign = "center";
    s.cols.forEach((col, i) => {
      col.y += dts * 16 * col.sp * (1 + boost * 3);
      const row = Math.floor(col.y);
      if (row === col.row) return;
      const x = i * s.cw - px * 15;
      if (col.row >= 0 && col.last) {
        cx.fillStyle = col.purple ? purple(0.75 * A) : orange(0.75 * A);
        cx.fillText(col.last, x, col.row * rh);
      }
      const ch = MCHARS[(R() * MCHARS.length) | 0];
      cx.fillStyle = `rgba(255,226,190,${clamp01(A)})`;
      cx.fillText(ch, x, row * rh);
      col.last = ch;
      col.row = row;
      if (row * rh > H && R() > 0.95) {
        col.y = -R() * 10;
        col.row = -1;
      }
    });
  },
};

// Old monitor: scanlines, rolling band, noise and rare RGB-split glitches.
const crt: Effect<{ until: number; next: number }> = {
  init: () => ({ until: 0, next: 2 }),
  draw({ cx, W, H, t, A, K, px, py }, s) {
    if (t > s.next) {
      s.until = t + 0.12 + R() * 0.18;
      s.next = t + 1.5 + R() * 4;
    }
    const glitch = t < s.until;
    const fl = 0.85 + R() * 0.15;
    cx.font = `800 ${Math.min(W * 0.11, 160)}px system-ui, sans-serif`;
    cx.textAlign = "center";
    cx.textBaseline = "middle";
    const tx = W / 2 - px * 30, ty = H * 0.84 - py * 20, txt = "♂ DUNGEON ♂";
    if (glitch) {
      const o = 4 + R() * 10;
      cx.fillStyle = `rgba(255,40,80,${clamp01(0.18 * A)})`;
      cx.fillText(txt, tx - o, ty);
      cx.fillStyle = `rgba(40,220,255,${clamp01(0.18 * A)})`;
      cx.fillText(txt, tx + o, ty + 1);
    }
    cx.fillStyle = orange(0.07 * A * fl);
    cx.fillText(txt, tx, ty);
    cx.textBaseline = "alphabetic";
    const by = ((t * 0.12) % 1) * (H + 240) - 120;
    const bg = cx.createLinearGradient(0, by - 120, 0, by + 120);
    bg.addColorStop(0, "rgba(255,160,80,0)");
    bg.addColorStop(0.5, `rgba(255,160,80,${clamp01(0.06 * A)})`);
    bg.addColorStop(1, "rgba(255,160,80,0)");
    cx.fillStyle = bg;
    cx.fillRect(0, by - 120, W, 240);
    cx.fillStyle = `rgba(255,140,40,${clamp01(0.035 * A * fl)})`;
    for (let y = 0; y < H; y += 3) cx.fillRect(0, y, W, 1);
    cx.fillStyle = `rgba(255,255,255,${clamp01(0.12 * A)})`;
    for (let i = Math.round(250 * K); i > 0; i--) cx.fillRect(R() * W, R() * H, 1, 1);
    if (glitch) {
      const cs = [`rgba(255,40,80,${0.12 * A})`, `rgba(40,220,255,${0.12 * A})`, `rgba(196,60,255,${0.14 * A})`];
      for (let i = 0; i < 6; i++) {
        cx.fillStyle = cs[i % 3];
        cx.fillRect(R() * W * 0.3 - 50, R() * H, W * (0.3 + R() * 0.7), 2 + R() * 18);
      }
    }
    const vg = cx.createRadialGradient(W / 2, H / 2, Math.min(W, H) * 0.35, W / 2, H / 2, Math.max(W, H) * 0.75);
    vg.addColorStop(0, "rgba(0,0,0,0)");
    vg.addColorStop(1, "rgba(0,0,0,.55)");
    cx.fillStyle = vg;
    cx.fillRect(0, 0, W, H);
  },
};

// Starfield flying at the viewer; the pointer steers, sign-in jumps to hyperspace.
type Star = { x: number; y: number; z: number };
const warp: Effect<Star[]> = {
  init: () => Array.from({ length: 520 }, () => ({ x: R() * 2 - 1, y: R() * 2 - 1, z: R() * 0.95 + 0.05 })),
  draw({ cx, W, H, dt, A, K, px, py, boost }, stars) {
    const ox = W / 2 - px * W * 0.18, oy = H / 2 - py * H * 0.18, sc = Math.max(W, H) * 0.5;
    const sp = (0.12 + 0.3 * K) * (1 + boost * 10);
    const n = Math.min(stars.length, Math.round(160 + 300 * K));
    cx.lineCap = "round";
    for (let i = 0; i < n; i++) {
      const s = stars[i], pz = s.z;
      s.z -= dt * sp;
      const x1 = ox + (s.x / pz) * sc, y1 = oy + (s.y / pz) * sc;
      const x2 = ox + (s.x / s.z) * sc, y2 = oy + (s.y / s.z) * sc;
      if (s.z <= 0.02 || x2 < -50 || x2 > W + 50 || y2 < -50 || y2 > H + 50) {
        s.x = R() * 2 - 1;
        s.y = R() * 2 - 1;
        s.z = 1;
        continue;
      }
      const k = 1 - s.z;
      cx.strokeStyle = boost > 0.05 && i % 3 === 0 ? purple(k * A) : `rgba(255,${(200 + k * 55) | 0},${(150 + k * 90) | 0},${clamp01(k * A * 0.9)})`;
      cx.lineWidth = 0.5 + k * 2.2;
      cx.beginPath();
      cx.moveTo(x1, y1);
      cx.lineTo(x2, y2);
      cx.stroke();
    }
  },
};

// Doom-style fire from the bottom edge; strength = flame height, pointer = wind.
type FireState = ReturnType<typeof offscreen> & { cell: number; fw: number; fh: number; buf: Uint8Array; pal: Uint8Array; acc: number };
const fire: Effect<FireState> = {
  init: ({ W }) => {
    const cell = 6, fw = Math.ceil(W / cell), fh = 100;
    const pal = new Uint8Array(37 * 4);
    for (let i = 0; i <= 36; i++) {
      const t = i / 36;
      pal[i * 4] = 255 * clamp01(t * 1.9);
      pal[i * 4 + 1] = 200 * clamp01((t - 0.35) * 1.7);
      pal[i * 4 + 2] = 255 * clamp01((t - 0.9) * 2);
      pal[i * 4 + 3] = 255 * clamp01(t * 2.4);
    }
    return { ...offscreen(fw, fh), cell, fw, fh, buf: new Uint8Array(fw * fh), pal, acc: 0 };
  },
  draw({ cx, W, H, dts, A, K, px, boost }, s) {
    const { fw, fh, buf } = s, size = fw * fh;
    buf.fill(36, (fh - 1) * fw);
    const decay = Math.max(0.25, Math.min(0.92, 0.9 - 0.3 * K) - boost * 0.3);
    s.acc += dts * 45;
    let steps = Math.min(4, Math.floor(s.acc));
    s.acc -= steps;
    const wind = px * 1.2;
    while (steps-- > 0) {
      for (let x = 0; x < fw; x++) {
        for (let y = 1; y < fh; y++) {
          const i = y * fw + x, v = buf[i];
          if (!v) {
            buf[i - fw] = 0;
            continue;
          }
          const r = R();
          const dst = i - fw + Math.round(r * 3 - 1.5 + wind);
          if (dst >= 0 && dst < size) buf[dst] = v - (r < decay ? 1 : 0);
        }
      }
    }
    const d = s.img.data, pal = s.pal;
    for (let i = 0; i < size; i++) {
      const p = buf[i] * 4, o = i * 4;
      d[o] = pal[p];
      d[o + 1] = pal[p + 1];
      d[o + 2] = pal[p + 2];
      d[o + 3] = pal[p + 3];
    }
    s.octx.putImageData(s.img, 0, 0);
    const g = cx.createRadialGradient(W / 2, H * 1.2, 0, W / 2, H, H);
    g.addColorStop(0, `rgba(255,100,20,${clamp01(0.2 * A)})`);
    g.addColorStop(1, "rgba(255,100,20,0)");
    cx.fillStyle = g;
    cx.fillRect(0, 0, W, H);
    cx.globalAlpha = clamp01(0.55 * A);
    cx.imageSmoothingEnabled = true;
    cx.drawImage(s.oc, 0, H - fh * s.cell, fw * s.cell, fh * s.cell);
    cx.globalAlpha = 1;
  },
};

// Meshing gears and swinging chains at different depths.
function gear(cx: CanvasRenderingContext2D, x: number, y: number, r: number, n: number, rot: number, color: string, lw: number) {
  const d = r * 0.13, st = TAU / n, rim = r - d * 1.6;
  cx.beginPath();
  for (let i = 0; i < n; i++) {
    const a = rot + i * st;
    cx.lineTo(x + Math.cos(a) * (r - d), y + Math.sin(a) * (r - d));
    cx.lineTo(x + Math.cos(a + st * 0.15) * r, y + Math.sin(a + st * 0.15) * r);
    cx.lineTo(x + Math.cos(a + st * 0.45) * r, y + Math.sin(a + st * 0.45) * r);
    cx.lineTo(x + Math.cos(a + st * 0.6) * (r - d), y + Math.sin(a + st * 0.6) * (r - d));
  }
  cx.closePath();
  cx.moveTo(x + r * 0.32, y);
  cx.arc(x, y, r * 0.32, 0, TAU);
  cx.moveTo(x + rim, y);
  cx.arc(x, y, rim, 0, TAU);
  for (let k = 0; k < 5; k++) {
    const a = rot + (k / 5) * TAU;
    cx.moveTo(x + Math.cos(a) * r * 0.32, y + Math.sin(a) * r * 0.32);
    cx.lineTo(x + Math.cos(a) * rim, y + Math.sin(a) * rim);
  }
  cx.strokeStyle = color;
  cx.lineWidth = lw;
  cx.stroke();
}
type Gear = { x: number; y: number; r: number; n: number; z: number; w: number; ph: number; c: string };
type Chain = { x: number; len: number; ph: number; z: number };
const gears: Effect<{ gs: Gear[]; chains: Chain[] }> = {
  init: ({ W, H }) => {
    const gs: Gear[] = [];
    const mesh = (g: Gear, a: number, r: number, n: number) => {
      const dist = g.r + r - g.r * 0.12;
      gs.push({ x: g.x + Math.cos(a) * dist, y: g.y + Math.sin(a) * dist, r, n, z: g.z, w: (-g.w * g.r) / r, ph: Math.PI / n, c: g.c });
    };
    const g1: Gear = { x: W * 0.1, y: H * 0.28, r: 120, n: 16, z: 0.6, w: 0.25, ph: 0, c: "240,138,28" };
    const g2: Gear = { x: W * 0.88, y: H * 0.72, r: 150, n: 20, z: 0.9, w: -0.18, ph: 0, c: "196,60,255" };
    gs.push(g1, g2);
    mesh(g1, 0.5, 70, 10);
    mesh(g2, 3.6, 90, 12);
    gs.push({ x: W * 0.62, y: H * 0.15, r: 60, n: 9, z: 0.3, w: 0.4, ph: 0, c: "196,60,255" });
    gs.push({ x: W * 0.32, y: H * 0.86, r: 80, n: 12, z: 0.35, w: -0.3, ph: 0, c: "240,138,28" });
    const chains = [0.05, 0.26, 0.75, 0.95].map((x, i) => ({ x, len: H * (0.35 + (i % 2) * 0.3), ph: i * 1.7, z: [0.5, 0.3, 0.4, 0.7][i] }));
    return { gs, chains };
  },
  draw({ cx, W, t, A, px, py }, s) {
    for (const g of s.gs) gear(cx, g.x - px * 70 * g.z, g.y - py * 50 * g.z, g.r, g.n, t * g.w + g.ph, `rgba(${g.c},${clamp01((0.07 + 0.22 * g.z) * A)})`, 1 + g.z * 1.5);
    for (const ch of s.chains) {
      const ax = ch.x * W - px * 70 * ch.z, ay = -py * 30 * ch.z;
      const sway = 0.05 * Math.sin(t * 0.8 + ch.ph) - px * 0.04;
      cx.strokeStyle = `rgba(169,156,151,${clamp01((0.06 + 0.2 * ch.z) * A)})`;
      cx.lineWidth = 1.5 + ch.z;
      const link = 22, n = Math.floor(ch.len / link);
      for (let i = 0; i < n; i++) {
        const L = i * link + link / 2;
        cx.beginPath();
        cx.ellipse(ax + Math.sin(sway) * L, ay + Math.cos(sway) * L, i % 2 ? 2.5 : 7, 12, -sway, 0, TAU);
        cx.stroke();
      }
    }
  },
};

const EFFECTS: Record<Drawable, Effect> = { embers, grid, glyphs, floor, spotlight, magnet, ripple, aurora, flow, metaballs, matrix, crt, warp, fire, gears };

// ---------- component ----------

export function DungeonBg({ effect, strength }: { effect: BgEffect; strength: number }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const reduced = useReducedMotion();
  const resolved = resolveEffect(effect);
  // Read by the render loop without restarting it on every slider tick.
  const live = useRef({ effect: resolved, k: strength / 100, reduced });
  live.current = { effect: resolved, k: strength / 100, reduced };
  const kick = useRef<() => void>(() => undefined);

  useEffect(() => {
    const cv = ref.current;
    const cx = cv?.getContext("2d");
    if (!cv || !cx) return;

    let W = 0, H = 0;
    const states = new Map<string, unknown>();
    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      W = window.innerWidth;
      H = window.innerHeight;
      cv.width = Math.round(W * dpr);
      cv.height = Math.round(H * dpr);
      cx.setTransform(dpr, 0, 0, dpr, 0, 0);
      states.clear(); // layouts depend on the viewport
    };
    resize();

    let tx = 0, ty = 0, mx = 0, my = 0;
    const ptr: Ptr = { x: W / 2, y: H / 2, rx: W / 2, ry: H / 2, active: false, moved: -1e9 };
    const clicks: { x: number; y: number }[] = [];
    const onMove = (e: PointerEvent) => {
      tx = (e.clientX / W) * 2 - 1;
      ty = (e.clientY / H) * 2 - 1;
      ptr.rx = e.clientX;
      ptr.ry = e.clientY;
      if (!ptr.active) {
        ptr.x = e.clientX;
        ptr.y = e.clientY;
      }
      ptr.active = true;
      ptr.moved = performance.now() / 1000;
    };
    const onDown = (e: PointerEvent) => clicks.push({ x: e.clientX, y: e.clientY });
    const onLeave = () => {
      tx = ty = 0;
      ptr.active = false;
    };

    let t = 0, last = performance.now(), raf = 0, lastEff = "";

    const render = (dt: number) => {
      const { effect, k, reduced } = live.current;
      const K = reduced ? Math.min(k, 1) : k;
      const speed = 0.3 + 0.7 * K;
      t += dt * speed;
      mx += (tx - mx) * 0.06;
      my += (ty - my) * 0.06;
      ptr.x += (ptr.rx - ptr.x) * 0.15;
      ptr.y += (ptr.ry - ptr.y) * 0.15;
      if (effect !== lastEff) {
        cx.clearRect(0, 0, W, H);
        lastEff = effect;
      }
      if (effect === "off") {
        cx.clearRect(0, 0, W, H);
        return;
      }
      const fx = EFFECTS[effect];
      if (fx.trail) {
        cx.globalCompositeOperation = "destination-out";
        cx.fillStyle = `rgba(0,0,0,${fx.trail})`;
        cx.fillRect(0, 0, W, H);
        cx.globalCompositeOperation = "source-over";
      } else cx.clearRect(0, 0, W, H);
      const c: Ctx = {
        cx, W, H, t, dt, dts: dt * speed, now: performance.now() / 1000,
        px: reduced ? 0 : mx * K,
        py: reduced ? 0 : my * K,
        A: Math.min(1.6, K), K,
        boost: boostAt ? clamp01(1 - (performance.now() - boostAt) / BOOST_MS) : 0,
        ptr, clicks,
      };
      if (!states.has(effect)) states.set(effect, fx.init ? fx.init(c) : null);
      cx.save();
      fx.draw(c, states.get(effect));
      cx.restore();
      clicks.length = 0;
    };

    const frame = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000);
      last = now;
      render(dt);
      const { effect, reduced } = live.current;
      raf = document.hidden || effect === "off" || reduced ? 0 : requestAnimationFrame(frame);
    };
    // A paused loop (hidden tab, "off", reduced motion) needs a restart when settings change.
    const start = () => {
      cancelAnimationFrame(raf);
      raf = 0;
      const { effect, reduced } = live.current;
      if (effect === "off") return render(0);
      if (reduced) {
        // Reduced motion: let the effect settle off-screen, then keep one still frame.
        for (let i = 0; i < 120; i++) render(1 / 60);
        return;
      }
      last = performance.now();
      raf = requestAnimationFrame(frame);
    };
    kick.current = start;

    const onVis = () => {
      if (!document.hidden) start();
    };
    const onResize = () => {
      resize();
      start();
    };
    window.addEventListener("resize", onResize);
    window.addEventListener("pointermove", onMove, { passive: true });
    window.addEventListener("pointerdown", onDown, { passive: true });
    document.addEventListener("pointerleave", onLeave);
    document.addEventListener("visibilitychange", onVis);
    start();

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", onResize);
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerdown", onDown);
      document.removeEventListener("pointerleave", onLeave);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, []);

  useEffect(() => {
    kick.current();
  }, [resolved, strength, reduced]);

  return <canvas ref={ref} class="dungeon-bg" aria-hidden="true" />;
}

/** Effect switcher, strength slider and form-animation toggle under the login card. */
export function BgControls({ t, prefs, update }: { t: Dict; prefs: BgPrefs; update: (p: Partial<BgPrefs>) => void }) {
  const { effect, strength, formFx } = prefs;
  const active = effect === "daily" ? `${t.bgNames.daily} → ${t.bgNames[dailyEffect()]}` : t.bgNames[effect];
  return (
    <div class="bg-controls" role="group" aria-label={t.bgLabel}>
      <div class="bg-row">
        <span class="bg-title">{t.bgLabel}</span>
        <span class="bg-active">
          {t.bgActive}: {active}
        </span>
      </div>
      <div class="bg-seg">
        {BG_EFFECTS.map((e) => (
          <button key={e} type="button" class={"ghost small" + (e === effect ? " on" : "")} aria-pressed={e === effect} onClick={() => update({ effect: e })}>
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
      <label class="bg-row bg-check">
        <input type="checkbox" checked={formFx} onChange={(e) => update({ formFx: (e.target as HTMLInputElement).checked })} />
        <span>{t.bgFormFx}</span>
      </label>
    </div>
  );
}
