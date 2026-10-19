import type { JSX } from "preact";
import { useId } from "preact/hooks";

// A wardrobe of schematic men's underwear, one model per achievement:
// from 1900s union suits to leather, lace, anime prints and metal.
// Everything is drawn in a 200×160 box.

export type Look = "done" | "shame" | "progress" | "locked" | "soon" | "secret";

type U = (k: string) => string; // makes a page-unique SVG id
type ShapeProps = { fill: string; stroke: string; sw: number; dash?: string };

interface Model {
  shape: (p: ShapeProps) => JSX.Element;
  fabric: (u: U) => string; // fill of the shape
  edge: string; // outline color
  defs?: (u: U) => JSX.Element;
  details?: (u: U) => JSX.Element; // clipped to the shape
  extras?: (u: U) => JSX.Element; // drawn on top, unclipped
  fuzzy?: boolean; // outline gets a fur filter
}

// ---------- silhouettes ----------

const BRIEF = "M10 22 Q100 34 190 22 L186 64 Q138 74 126 142 L74 142 Q62 74 14 64 Z";
const BIKINI = "M18 44 Q100 54 182 44 L180 60 Q130 68 118 134 L82 134 Q70 68 20 60 Z";
const TRUNK = "M20 22 Q100 30 180 22 L186 108 L118 114 Q100 98 82 114 L14 108 Z";
const BOXER = "M24 16 Q100 24 176 16 L194 148 L108 148 L100 104 L92 148 L6 148 Z";
const BOXER_BRIEF = "M28 18 Q100 26 172 18 L182 132 L114 136 Q100 112 86 136 L18 132 Z";
const LONG_SHORTS = "M28 14 Q100 22 172 14 L184 150 L116 152 Q100 120 84 152 L16 150 Z";
const UNION = "M40 4 Q100 10 160 4 L172 156 L120 156 L104 72 Q100 66 96 72 L80 156 L28 156 Z";
const JOCK_BAND = "M14 20 Q100 30 186 20 L186 36 Q100 46 14 36 Z";
const JOCK_POUCH = "M72 42 Q100 50 128 42 Q132 92 116 124 Q100 136 84 124 Q68 92 72 42 Z";
const JOCK_STRAPS = "M88 120 Q40 110 20 36 M112 120 Q160 110 180 36";
const FUNDO_BAND = "M8 28 Q100 40 192 28 L192 42 Q100 54 8 42 Z";
const FUNDO_APRON = "M78 46 L122 46 L128 152 L72 152 Z";

const path = (d: string) => (p: ShapeProps) => (
  <path d={d} fill={p.fill} stroke={p.stroke} stroke-width={p.sw} stroke-dasharray={p.dash} stroke-linejoin="round" />
);

// Shapes return fragments, never <g>: they are also used inside <clipPath>,
// which ignores groups.
const jock = (p: ShapeProps) => (
  <>
    {p.fill === "none" ? (
      <path d={JOCK_STRAPS} fill="none" stroke={p.stroke} stroke-width={p.sw} stroke-dasharray={p.dash} stroke-linecap="round" />
    ) : (
      <>
        <path d={JOCK_STRAPS} fill="none" stroke={p.stroke} stroke-width={10 + p.sw * 2} stroke-linecap="round" />
        <path d={JOCK_STRAPS} fill="none" stroke={p.fill} stroke-width={10} stroke-linecap="round" />
      </>
    )}
    {path(JOCK_BAND)(p)}
    {path(JOCK_POUCH)(p)}
  </>
);

const fundoshi = (p: ShapeProps) => (
  <>
    {path(FUNDO_APRON)(p)}
    {path(FUNDO_BAND)(p)}
  </>
);

// Two small briefs side by side.
const PAIR_SCALE = 0.5;
const pair = (p: ShapeProps) => (
  <>
    {[0, 100].map((x) => (
      <path
        key={x}
        d={BRIEF}
        transform={`translate(${x} 30) scale(${PAIR_SCALE})`}
        fill={p.fill}
        stroke={p.stroke}
        stroke-width={p.sw / PAIR_SCALE}
        stroke-dasharray={p.dash}
        stroke-linejoin="round"
      />
    ))}
  </>
);

// ---------- fabric helpers ----------

const grad = (id: string, stops: string[], x2 = "1", y2 = "1") => (
  <linearGradient id={id} x1="0" y1="0" x2={x2} y2={y2}>
    {stops.map((c, i) => (
      <stop key={i} offset={stops.length === 1 ? 0 : i / (stops.length - 1)} stop-color={c} />
    ))}
  </linearGradient>
);

const url = (id: string) => `url(#${id})`;

/** y of a waistband curve "M x0 y0 Q100 ymid x1 y0" at x (quadratic). */
const bandY = (x: number, edgeY: number, ctrlY: number, x0 = 10, x1 = 190) => {
  const t = (x - x0) / (x1 - x0);
  return (1 - t) * (1 - t) * edgeY + 2 * t * (1 - t) * ctrlY + t * t * edgeY;
};

const range = (from: number, to: number, step: number) => {
  const out: number[] = [];
  for (let x = from; x <= to; x += step) out.push(x);
  return out;
};

const sparkle = (x: number, y: number, r: number, fill: string) => (
  <path
    d={`M${x} ${y - r} Q${x} ${y} ${x + r} ${y} Q${x} ${y} ${x} ${y + r} Q${x} ${y} ${x - r} ${y} Q${x} ${y} ${x} ${y - r} Z`}
    fill={fill}
  />
);

const heart = (x: number, y: number, s: number, fill: string) => (
  <path
    d={`M${x} ${y + s * 0.9} C${x - s * 1.4} ${y} ${x - s * 0.8} ${y - s} ${x} ${y - s * 0.3} C${x + s * 0.8} ${y - s} ${x + s * 1.4} ${y} ${x} ${y + s * 0.9} Z`}
    fill={fill}
  />
);

// ---------- the wardrobe ----------

export const MODELS: Record<string, Model> = {
  // 1900s: red flannel union suit, cropped. "A hundred" years ago.
  streak_100: {
    shape: path(UNION),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#d0473a", "#9c2a1f", "#6e170f"]),
    edge: "#4a0d08",
    details: () => (
      <g>
        <path d="M100 6 V68" stroke="#5a120c" stroke-width="3" />
        {[18, 32, 46, 60].map((y) => (
          <circle key={y} cx="104" cy={y} r="3.4" fill="#f3e5c7" stroke="#5a120c" stroke-width="1" />
        ))}
        <path d="M29 140 H82 M29 146 H82 M29 152 H82 M118 140 H171 M118 146 H171 M118 152 H171" stroke="#5a120c" stroke-width="2" opacity=".7" />
        <path d="M46 20 Q52 60 60 120" stroke="#ffffff" stroke-width="5" opacity=".08" fill="none" />
      </g>
    ),
  },
  // 1940s: pinstriped boxer shorts with buttons.
  streak_30: {
    shape: path(BOXER),
    fabric: (u) => url(u("p")),
    defs: (u) => (
      <pattern id={u("p")} width="9" height="9" patternUnits="userSpaceOnUse">
        <rect width="9" height="9" fill="#dbe6f4" />
        <path d="M4.5 0 V9" stroke="#7d9cc6" stroke-width="1.4" />
      </pattern>
    ),
    edge: "#46638c",
    details: () => (
      <g>
        <path d="M24 16 Q100 24 176 16 L178 32 Q100 40 22 32 Z" fill="#eef2f8" />
        {range(30, 170, 7).map((x) => (
          <path key={x} d={`M${x} ${bandY(x, 18, 26, 24, 176)} v12`} stroke="#9fb4d2" stroke-width="1.2" />
        ))}
        <path d="M100 40 V104" stroke="#46638c" stroke-width="2" />
        {[52, 70, 88].map((y) => (
          <circle key={y} cx="106" cy={y} r="3.6" fill="#f7f2e4" stroke="#7a6a4f" stroke-width="1.2" />
        ))}
      </g>
    ),
  },
  // 1950s: the classic white Y-front.
  streak_7: {
    shape: path(BRIEF),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#ffffff", "#e9e3d6", "#cfc7b6"]),
    edge: "#8d96a0",
    details: () => (
      <g fill="none">
        <path d="M11 30 Q100 42 189 30" stroke="#2b4c7e" stroke-width="3" />
        <path d="M12 38 Q100 50 188 38" stroke="#2b4c7e" stroke-width="3" />
        <path d="M82 50 Q98 76 100 104 M118 50 Q102 76 100 104" stroke="#a79f8f" stroke-width="2.6" stroke-dasharray="4 3" />
      </g>
    ),
  },
  // 1960s: school tighty-whities, mum wrote the name on the tag.
  first_test: {
    shape: path(TRUNK),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#ffffff", "#ecebe6"]),
    edge: "#9b9b94",
    details: () => (
      <g>
        <path d="M20 22 Q100 30 180 22 L181 34 Q100 42 19 34 Z" fill="#e3e2dc" />
        <rect x="110" y="48" width="50" height="20" rx="2" fill="#fff" stroke="#3a6ea5" stroke-width="1.6" />
        <text x="135" y="62" text-anchor="middle" font-size="11" font-weight="800" font-family="monospace" fill="#3a6ea5">
          TEST
        </text>
        <path d="M100 44 Q94 74 100 100" stroke="#c9c8c0" stroke-width="2" fill="none" />
      </g>
    ),
  },
  // 1970s: sport briefs with contrast piping.
  goroutine: {
    shape: path(BRIEF),
    fabric: () => "#f7f6f0",
    edge: "#b71c1c",
    details: () => (
      <g fill="none" stroke-linecap="round">
        <path d="M74 142 Q62 74 14 64 M186 64 Q138 74 126 142" stroke="#c62828" stroke-width="10" />
        <path d="M82 142 Q70 78 16 72 M184 72 Q130 78 118 142" stroke="#1e4fa3" stroke-width="3.5" />
        <path d="M11 29 Q100 41 189 29" stroke="#c62828" stroke-width="7" />
        <path d="M12 38 Q100 50 188 38" stroke="#1e4fa3" stroke-width="3" />
      </g>
    ),
  },
  // 1980s: neon striped trunks.
  channel: {
    shape: path(TRUNK),
    fabric: (u) => url(u("p")),
    defs: (u) => (
      <pattern id={u("p")} width="200" height="27" patternUnits="userSpaceOnUse">
        <rect width="200" height="9" fill="#ff4fa3" />
        <rect y="9" width="200" height="9" fill="#29e0ff" />
        <rect y="18" width="200" height="9" fill="#ffe14d" />
      </pattern>
    ),
    edge: "#1a1a2e",
    details: () => (
      <g>
        <path d="M20 22 Q100 30 180 22 L181 38 Q100 46 19 38 Z" fill="#1a1a2e" />
        <path d="M30 32 l8 -5 l8 5 l8 -5 l8 5 M138 32 l8 -5 l8 5 l8 -5 l8 5" stroke="#29e0ff" stroke-width="2" fill="none" />
        <path d="M14 108 L20 60 M186 108 L180 60" stroke="#ffffff" stroke-width="4" opacity=".7" />
      </g>
    ),
  },
  // 1990s: leopard bikini briefs.
  select: {
    shape: path(BIKINI),
    fabric: (u) => url(u("p")),
    defs: (u) => (
      <pattern id={u("p")} width="28" height="24" patternUnits="userSpaceOnUse">
        <rect width="28" height="24" fill="#e8a33d" />
        <ellipse cx="7" cy="7" rx="4.5" ry="3.5" fill="none" stroke="#3b2410" stroke-width="2.4" />
        <ellipse cx="21" cy="18" rx="4" ry="3" fill="none" stroke="#3b2410" stroke-width="2.4" />
        <circle cx="21" cy="5" r="1.6" fill="#3b2410" />
        <circle cx="8" cy="19" r="1.4" fill="#3b2410" />
      </pattern>
    ),
    edge: "#4a2d0c",
    details: () => <path d="M18 46 Q100 56 182 46" stroke="#2a1a08" stroke-width="6" fill="none" />,
  },
  // 2000s: boxer briefs with a branded waistband.
  context: {
    shape: path(BOXER_BRIEF),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#4a5059", "#2d3138"]),
    edge: "#121418",
    details: () => (
      <g>
        <path d="M28 18 Q100 26 172 18 L173 42 Q100 50 27 42 Z" fill="#ececec" />
        <text x="100" y="38" text-anchor="middle" font-size="13" font-weight="900" letter-spacing="3" font-family="sans-serif" fill="#121418">
          GOPHER KLEIN
        </text>
        <path d="M78 48 Q80 100 100 110 Q120 100 122 48" stroke="#22262c" stroke-width="2.5" fill="none" />
      </g>
    ),
  },
  // Modern sport: jockstrap.
  generics: {
    shape: jock,
    fabric: () => "#f2f2f2",
    edge: "#8a8a8a",
    details: (u) => (
      <g>
        <path d="M14 25 Q100 35 186 25 M14 31 Q100 41 186 31" stroke="#d32f2f" stroke-width="2.6" fill="none" />
        <path d={JOCK_POUCH} fill={url(u("m"))} />
      </g>
    ),
    defs: (u) => (
      <pattern id={u("m")} width="6" height="6" patternUnits="userSpaceOnUse">
        <rect width="6" height="6" fill="#d6d6d6" />
        <circle cx="3" cy="3" r="1.2" fill="#b5b5b5" />
      </pattern>
    ),
  },
  // Wrapped Japanese loincloth for the error wrapper.
  errwrap: {
    shape: fundoshi,
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#fbf9f3", "#e4ddcd"], "0", "1"),
    edge: "#9d9382",
    details: () => (
      <g fill="none">
        <path d="M88 48 L85 150 M100 48 L100 150 M112 48 L115 150" stroke="#d6cdb9" stroke-width="2" />
      </g>
    ),
    extras: () => (
      <g>
        <path d="M8 35 Q100 47 192 35" stroke="#b23a2e" stroke-width="5" stroke-dasharray="7 4" fill="none" />
        <circle cx="100" cy="44" r="8" fill="#b23a2e" />
        <path d="M100 50 l-8 14 M100 50 l8 14" stroke="#b23a2e" stroke-width="4" stroke-linecap="round" />
      </g>
    ),
  },
  // Compression shorts with racing stripes.
  benchmark: {
    shape: path(LONG_SHORTS),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#24272b", "#0e0f11"]),
    edge: "#000000",
    details: () => (
      <g>
        <path d="M28 14 Q100 22 172 14 L173 28 Q100 36 27 28 Z" fill="#b6ff3b" />
        <path d="M32 30 L22 150 M168 30 L178 150" stroke="#b6ff3b" stroke-width="6" />
        <path d="M100 34 Q92 80 100 118 M50 60 Q60 100 56 150 M150 60 Q140 100 144 150" stroke="#34383e" stroke-width="2" fill="none" />
      </g>
    ),
  },
  // Plaid boxers: a table of cases.
  table_test: {
    shape: path(BOXER),
    fabric: (u) => url(u("p")),
    defs: (u) => (
      <pattern id={u("p")} width="24" height="24" patternUnits="userSpaceOnUse">
        <rect width="24" height="24" fill="#a3201c" />
        <rect width="9" height="24" fill="#1f4d2b" opacity=".75" />
        <rect width="24" height="9" fill="#1f4d2b" opacity=".6" />
        <path d="M16 0 V24 M0 16 H24" stroke="#f2c94c" stroke-width="1" />
      </pattern>
    ),
    edge: "#3d0b09",
    details: () => (
      <g>
        <path d="M24 16 Q100 24 176 16 L178 32 Q100 40 22 32 Z" fill="#efe4d0" />
        {[54, 74].map((y) => (
          <circle key={y} cx="106" cy={y} r="3.6" fill="#efe4d0" stroke="#3d0b09" stroke-width="1.2" />
        ))}
      </g>
    ),
  },
  // Faux fur for the fuzzer.
  fuzz: {
    shape: path(BRIEF),
    fabric: (u) => url(u("f")),
    defs: (u) => (
      <>
        <radialGradient id={u("f")} cx=".5" cy=".4" r=".7">
          <stop offset="0" stop-color="#ffc2df" />
          <stop offset="1" stop-color="#f06aa8" />
        </radialGradient>
        <filter id={u("fur")} x="-10%" y="-10%" width="120%" height="120%">
          <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" seed="3" />
          <feDisplacementMap in="SourceGraphic" scale="9" />
        </filter>
      </>
    ),
    edge: "#ffe1ef",
    fuzzy: true,
    details: () => (
      <g fill="#ffe1ef" opacity=".55">
        {[
          [50, 50],
          [80, 70],
          [130, 54],
          [150, 66],
          [104, 96],
          [92, 120],
          [116, 128],
        ].map(([x, y]) => (
          <circle key={`${x}-${y}`} cx={x} cy={y} r="3" />
        ))}
      </g>
    ),
  },
  // Matching pair for the duo.
  duo_7: {
    shape: pair,
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#a8ccff", "#5d8ef0"]),
    edge: "#2b4c9a",
    extras: () => heart(100, 26, 9, "#ff4d6d"),
  },
  // Heart-print boxers: training together.
  duo_30: {
    shape: path(BOXER),
    fabric: (u) => url(u("p")),
    defs: (u) => (
      <pattern id={u("p")} width="26" height="26" patternUnits="userSpaceOnUse">
        <rect width="26" height="26" fill="#fff4f5" />
        {heart(7, 8, 4, "#e63950")}
        {heart(20, 20, 4, "#e63950")}
      </pattern>
    ),
    edge: "#a3202f",
    details: () => <path d="M24 16 Q100 24 176 16 L178 32 Q100 40 22 32 Z" fill="#e63950" />,
  },
  // Worn-out, holey briefs: the first failure.
  first_fail: {
    shape: path(BRIEF),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#c9c1b0", "#9e957f"]),
    edge: "#5f5848",
    details: () => (
      <g>
        <ellipse cx="88" cy="62" rx="16" ry="9" fill="#8f7a4a" opacity=".35" />
        <ellipse cx="70" cy="92" rx="8" ry="5.5" fill="#0d0a0b" />
        <ellipse cx="132" cy="72" rx="5.5" ry="4" fill="#0d0a0b" />
        <ellipse cx="108" cy="116" rx="4" ry="3" fill="#0d0a0b" />
        <path
          d={`M10 ${bandY(10, 22, 34)} ${range(16, 190, 6)
            .map((x, i) => `L${x} ${bandY(x, 22, 34) + (i % 2 ? 9 : 4)}`)
            .join(" ")}`}
          stroke="#5f5848"
          stroke-width="1.6"
          fill="none"
        />
        <path d="M150 34 l6 10 M158 33 l3 8" stroke="#5f5848" stroke-width="1.4" />
      </g>
    ),
  },
  // Black lace with a satin bow: the midnight commit.
  midnight: {
    shape: path(BIKINI),
    fabric: (u) => url(u("p")),
    defs: (u) => (
      <pattern id={u("p")} width="12" height="12" patternUnits="userSpaceOnUse">
        <rect width="12" height="12" fill="#151116" />
        <circle cx="6" cy="6" r="3.6" fill="none" stroke="#4a3f52" stroke-width="1" />
        <circle cx="0" cy="0" r="1.6" fill="#4a3f52" />
        <circle cx="12" cy="12" r="1.6" fill="#4a3f52" />
      </pattern>
    ),
    edge: "#0a080b",
    details: () => (
      <g fill="none">
        <path d="M18 47 Q100 57 182 47" stroke="#2c2530" stroke-width="5" />
        <path d="M100 66 Q120 96 104 126 M100 66 Q80 96 96 126" stroke="#3a3142" stroke-width="1.5" />
      </g>
    ),
    extras: () => (
      <g>
        <path d="M82 134 Q70 68 20 60 M180 60 Q130 68 118 134" stroke="#5a4d66" stroke-width="6" stroke-dasharray="0 8" stroke-linecap="round" fill="none" />
        <path d="M100 56 L86 49 L86 63 Z M100 56 L114 49 L114 63 Z" fill="#c43cff" />
        <circle cx="100" cy="56" r="3.2" fill="#e7a6ff" />
      </g>
    ),
  },
  // Leather with a zipper and studs: the comeback.
  comeback: {
    shape: path(BRIEF),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#4a2e1f", "#24150d", "#0f0805"]),
    edge: "#000000",
    details: () => (
      <g>
        <ellipse cx="62" cy="52" rx="34" ry="10" fill="#ffffff" opacity=".08" transform="rotate(-8 62 52)" />
        <path d="M17 58 Q66 70 78 136 M183 58 Q134 70 122 136 M12 36 Q100 48 188 36" stroke="#9a6a48" stroke-width="1.6" stroke-dasharray="4 3" fill="none" />
        <path d="M100 38 V128" stroke="#c8c8c8" stroke-width="5" stroke-dasharray="2 2" />
        <rect x="95" y="40" width="10" height="15" rx="2" fill="#e0e0e0" stroke="#7a7a7a" />
        {[26, 40, 160, 174].map((x) => (
          <circle key={x} cx={x} cy={bandY(x, 22, 34) + 22} r="3.4" fill="#d9d9d9" stroke="#6b6b6b" stroke-width="1" />
        ))}
      </g>
    ),
  },
  // Anime print: kawaii face and sparkles for the night shift.
  night_owl: {
    shape: path(TRUNK),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#ffe0f1", "#ffb3d9", "#c9a7ff"]),
    edge: "#c2448a",
    details: () => (
      <g>
        <path d="M20 22 Q100 30 180 22 L181 34 Q100 42 19 34 Z" fill="#7c4dff" />
        {[80, 120].map((x) => (
          <g key={x}>
            <ellipse cx={x} cy="68" rx="10" ry="13" fill="#2a1840" />
            <ellipse cx={x} cy="72" rx="6" ry="7" fill="#6a3fc4" />
            <circle cx={x + 3} cy="63" r="4" fill="#fff" />
            <circle cx={x - 3} cy="75" r="1.8" fill="#fff" />
          </g>
        ))}
        <ellipse cx="66" cy="86" rx="7" ry="3.5" fill="#ff6fae" opacity=".6" />
        <ellipse cx="134" cy="86" rx="7" ry="3.5" fill="#ff6fae" opacity=".6" />
        <path d="M93 86 q3.5 4 7 0 q3.5 4 7 0" stroke="#2a1840" stroke-width="2" fill="none" stroke-linecap="round" />
        {sparkle(40, 56, 7, "#fff")}
        {sparkle(160, 52, 6, "#fff59d")}
        {sparkle(150, 92, 5, "#fff")}
        {sparkle(46, 96, 4, "#fff59d")}
        <path d="M168 76 a8 8 0 1 0 6 12 a6 6 0 1 1 -6 -12 Z" fill="#fff59d" />
      </g>
    ),
  },
  // Gold armor: a year of daily Go.
  streak_365: {
    shape: path(BRIEF),
    fabric: (u) => url(u("f")),
    defs: (u) => grad(u("f"), ["#fff3b8", "#f0c040", "#b98400", "#7a5200"]),
    edge: "#5c3d00",
    details: () => (
      <g>
        <path d="M60 32 Q70 90 80 142 M140 32 Q130 90 120 142" stroke="#8a5a00" stroke-width="2.4" fill="none" />
        <path d="M12 40 Q100 52 188 40" stroke="#8a5a00" stroke-width="3" fill="none" />
        {range(22, 180, 20).map((x) => (
          <circle key={x} cx={x} cy={bandY(x, 40, 52) - 8} r="2.6" fill="#fff3b8" stroke="#7a5200" stroke-width="1" />
        ))}
        <path d="M84 98 L84 80 L92 88 L100 74 L108 88 L116 80 L116 98 Z" fill="#fff3b8" stroke="#7a5200" stroke-width="1.6" stroke-linejoin="round" />
        <ellipse cx="58" cy="56" rx="26" ry="7" fill="#ffffff" opacity=".35" transform="rotate(-10 58 56)" />
      </g>
    ),
  },
  // Secret: leather harness, chainmail pouch and an O-ring.
  nil_map: {
    shape: jock,
    fabric: (u) => url(u("f")),
    defs: (u) => (
      <>
        {grad(u("f"), ["#2c2c2e", "#0b0b0c"])}
        <pattern id={u("c")} width="10" height="9" patternUnits="userSpaceOnUse">
          <rect width="10" height="9" fill="#5b636b" />
          <circle cx="5" cy="4.5" r="3.6" fill="none" stroke="#dfe5ea" stroke-width="1.4" />
          <circle cx="0" cy="0" r="3.6" fill="none" stroke="#b8c0c8" stroke-width="1.4" />
          <circle cx="10" cy="0" r="3.6" fill="none" stroke="#b8c0c8" stroke-width="1.4" />
          <circle cx="0" cy="9" r="3.6" fill="none" stroke="#b8c0c8" stroke-width="1.4" />
          <circle cx="10" cy="9" r="3.6" fill="none" stroke="#b8c0c8" stroke-width="1.4" />
        </pattern>
      </>
    ),
    edge: "#000000",
    details: (u) => (
      <g>
        <path d={JOCK_POUCH} fill={url(u("c"))} />
        {range(24, 176, 16).map((x) => (
          <circle key={x} cx={x} cy={bandY(x, 20, 30, 14, 186) + 8} r="2.4" fill="#e6ebef" />
        ))}
      </g>
    ),
    extras: () => (
      <g>
        <circle cx="100" cy="40" r="9" fill="none" stroke="#e6ebef" stroke-width="4" />
        <circle cx="100" cy="40" r="9" fill="none" stroke="#8d969e" stroke-width="1" />
      </g>
    ),
  },
};

const FALLBACK = "streak_7";

// ---------- rendering ----------

/** What sits on locked briefs. */
function Mark({ look }: { look: Look }) {
  if (look === "secret")
    return (
      <path
        d="M86 74 A14 14 0 1 1 104 88 C100 91 100 94 100 100 M100 112 V114"
        fill="none"
        stroke="currentColor"
        stroke-width="9"
        stroke-linecap="round"
      />
    );
  return (
    <g fill="none" stroke="currentColor" stroke-width="7" stroke-linejoin="round">
      <rect x="80" y="80" width="40" height="32" rx="5" />
      <path d="M88 80 V72 A12 12 0 0 1 112 72 V80" />
    </g>
  );
}

export const isOpen = (look: Look) => look === "done" || look === "shame";

/**
 * One model. Unlocked: in full color. Locked: a dashed silhouette of its
 * shape with a lock (secret ones do not even show their shape).
 * side="back" shows the plain fabric with a care label.
 */
export function Briefs({
  id,
  look,
  side = "front",
  label = "",
  big = false,
}: {
  id: string;
  look: Look;
  side?: "front" | "back";
  label?: string;
  big?: boolean;
}) {
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const u: U = (k) => `b${uid}-${k}`;
  const open = isOpen(look);
  const m = MODELS[look === "secret" ? FALLBACK : id] ?? MODELS[FALLBACK];
  const sw = big ? 3 : 7;

  if (!open) {
    return (
      <g>
        {m.shape({ fill: big ? "currentColor" : "none", stroke: "currentColor", sw, dash: big ? "10 7" : "16 12" })}
        {big && <g opacity=".9">{m.shape({ fill: "#0d0a0b", stroke: "none", sw: 0 })}</g>}
        {side === "front" ? (
          <g transform={big ? "translate(100 88) scale(.55) translate(-100 -88)" : undefined}>
            <Mark look={look} />
          </g>
        ) : (
          <CareLabel label={label} color="currentColor" />
        )}
      </g>
    );
  }

  return (
    <g>
      <defs>
        {m.defs?.(u)}
        <clipPath id={u("clip")}>{m.shape({ fill: "#000", stroke: "none", sw: 0 })}</clipPath>
      </defs>
      {m.shape({ fill: m.fabric(u), stroke: "none", sw: 0 })}
      {side === "front" && m.details && <g clip-path={url(u("clip"))}>{m.details(u)}</g>}
      <g filter={m.fuzzy ? url(u("fur")) : undefined}>{m.shape({ fill: "none", stroke: m.edge, sw: m.fuzzy ? sw + 3 : sw })}</g>
      {side === "front" ? m.extras?.(u) : <CareLabel label={label} color={m.edge} />}
    </g>
  );
}

function CareLabel({ label, color }: { label: string; color: string }) {
  return (
    <g>
      <rect x="72" y="56" width="56" height="36" rx="3" fill="#f6f1e6" stroke={color} stroke-width="2.5" />
      <text x="100" y="79" text-anchor="middle" class="viewer-label" fill="#2a1d14">
        {label}
      </text>
    </g>
  );
}

/**
 * A retail pack: kraft cardboard with a window that shows only the dark
 * silhouette of the model inside. Used until the user unpacks it.
 */
export function PackageArt({ id }: { id: string }) {
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const m = MODELS[id] ?? MODELS[FALLBACK];
  const g = `p${uid}`;
  return (
    <g>
      <defs>
        {grad(`${g}-k`, ["#d9ad6c", "#b9874a", "#946634"])}
        <clipPath id={`${g}-w`}>
          <rect x="62" y="44" width="76" height="66" rx="10" />
        </clipPath>
      </defs>
      <rect x="34" y="8" width="132" height="146" rx="8" fill={url(`${g}-k`)} stroke="#5e3f1c" stroke-width="3" />
      <path d="M34 30 H166" stroke="#5e3f1c" stroke-width="2" opacity=".5" />
      <path d="M92 8 H108 V30 H92 Z" fill="#e8d7b8" opacity=".55" />
      <text x="100" y="25" text-anchor="middle" font-size="12" font-weight="900" letter-spacing="2" font-family="sans-serif" fill="#3b2410">
        ♂ GACHI
      </text>
      <rect x="62" y="44" width="76" height="66" rx="10" fill="#1a1412" stroke="#5e3f1c" stroke-width="2.5" />
      <g clip-path={url(`${g}-w`)}>
        <g transform="translate(62 46) scale(.38)" opacity=".85">
          {m.shape({ fill: "#000", stroke: "#3a2c24", sw: 4 })}
        </g>
        <path d="M70 110 L118 40 M84 112 L130 46" stroke="#ffffff" stroke-width="5" opacity=".12" />
      </g>
      <text x="100" y="132" text-anchor="middle" font-size="10" font-weight="800" letter-spacing="1" font-family="sans-serif" fill="#3b2410">
        1 PACK · 100% GO
      </text>
      <path d="M34 140 H166" stroke="#5e3f1c" stroke-width="1.5" stroke-dasharray="4 3" opacity=".6" />
    </g>
  );
}
