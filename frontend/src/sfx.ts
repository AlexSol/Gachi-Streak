// Opening-effect sounds, synthesized with Web Audio: no files to ship.
// Every call is fire-and-forget and silently does nothing without audio.

let ctx: AudioContext | null = null;

function ac(): AudioContext | null {
  try {
    ctx ??= new AudioContext();
    if (ctx.state === "suspended") void ctx.resume();
    return ctx;
  } catch {
    return null;
  }
}

const MASTER = 0.35;

function out(a: AudioContext, gain: number) {
  const g = a.createGain();
  g.gain.value = gain * MASTER;
  g.connect(a.destination);
  return g;
}

function noise(a: AudioContext, seconds: number) {
  const buf = a.createBuffer(1, Math.ceil(a.sampleRate * seconds), a.sampleRate);
  const d = buf.getChannelData(0);
  for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
  const src = a.createBufferSource();
  src.buffer = buf;
  return src;
}

/** A single enveloped oscillator note. */
function tone(type: OscillatorType, f0: number, f1: number, dur: number, vol: number, delay = 0) {
  const a = ac();
  if (!a) return;
  const t = a.currentTime + delay;
  const o = a.createOscillator();
  const g = out(a, vol);
  o.type = type;
  o.frequency.setValueAtTime(f0, t);
  o.frequency.exponentialRampToValueAtTime(Math.max(f1, 1), t + dur);
  g.gain.setValueAtTime(0, t);
  g.gain.linearRampToValueAtTime(vol * MASTER, t + 0.01);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  o.connect(g);
  o.start(t);
  o.stop(t + dur + 0.05);
}

/** Filtered noise burst with a moving band. */
function hiss(dur: number, f0: number, f1: number, vol: number, q = 1, delay = 0) {
  const a = ac();
  if (!a) return;
  const t = a.currentTime + delay;
  const src = noise(a, dur);
  const bp = a.createBiquadFilter();
  bp.type = "bandpass";
  bp.Q.value = q;
  bp.frequency.setValueAtTime(f0, t);
  bp.frequency.exponentialRampToValueAtTime(f1, t + dur);
  const g = out(a, vol);
  g.gain.setValueAtTime(vol * MASTER, t);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  src.connect(bp).connect(g);
  src.start(t);
}

export const sfx = {
  /** Cardboard taps while the box shakes. */
  tap: () => hiss(0.06, 600, 400, 0.6, 2),
  /** Cardboard / tape ripping. */
  rip: () => {
    for (let i = 0; i < 5; i++) hiss(0.09, 900 + i * 500, 2600 + i * 300, 0.7, 3, i * 0.06);
  },
  /** Something pops out. */
  pop: () => tone("sine", 260, 980, 0.18, 0.8),
  /** Bright arpeggio. */
  chime: () => [1047, 1319, 1568, 2093].forEach((f, i) => tone("triangle", f, f, 0.5, 0.35, i * 0.07)),
  /** Rising swell while spinning. */
  swell: (dur = 2) => {
    tone("sawtooth", 110, 880, dur, 0.12);
    hiss(dur, 300, 4000, 0.25, 0.7);
  },
  /** Falling whistle. */
  fall: () => tone("sine", 1400, 200, 0.55, 0.35),
  /** Heavy impact. */
  boom: (vol = 1) => {
    tone("sine", 140, 38, 0.7, 0.9 * vol);
    hiss(0.35, 400, 90, 0.8 * vol, 0.6);
  },
  /** Digital glitch bursts. */
  glitch: () => {
    for (let i = 0; i < 6; i++) {
      const f = 200 + Math.random() * 1800;
      tone("square", f, f * (0.5 + Math.random()), 0.05, 0.18, i * 0.11 + Math.random() * 0.05);
    }
  },
  /** Wah-wah-wah-waaah. */
  sadTrombone: () => {
    const notes = [392, 370, 349, 330];
    notes.forEach((f, i) => {
      const last = i === notes.length - 1;
      const a = ac();
      if (!a) return;
      const t = a.currentTime + i * 0.42;
      const dur = last ? 1.1 : 0.38;
      const o = a.createOscillator();
      o.type = "sawtooth";
      o.frequency.setValueAtTime(f, t);
      if (last) {
        const lfo = a.createOscillator();
        const depth = a.createGain();
        lfo.frequency.value = 6;
        depth.gain.value = 9;
        lfo.connect(depth).connect(o.frequency);
        lfo.start(t);
        lfo.stop(t + dur);
      }
      const lp = a.createBiquadFilter();
      lp.type = "lowpass";
      lp.frequency.value = 1100;
      const g = out(a, 0.3);
      g.gain.setValueAtTime(0, t);
      g.gain.linearRampToValueAtTime(0.3 * MASTER, t + 0.04);
      g.gain.setValueAtTime(0.3 * MASTER, t + dur - 0.08);
      g.gain.linearRampToValueAtTime(0, t + dur);
      o.connect(lp).connect(g);
      o.start(t);
      o.stop(t + dur);
    });
  },
};

/** Runs sound cues at times (ms); returns a cancel function. */
export function schedule(cues: [number, () => void][]): () => void {
  const ids = cues.map(([ms, fn]) => setTimeout(fn, ms));
  return () => ids.forEach(clearTimeout);
}
