// The Score — a generative ambient engine the agent "plays". Plain Web Audio API,
// zero deps. One AudioContext, a fixed node graph built once; the live loop only
// writes AudioParams (setTargetAtTime) — never creates/connects nodes per frame
// (the classic Web Audio click/leak footgun). Off by default; the context is
// created lazily on the enable click (the autoplay-policy user gesture).
import { reduced } from "./reduced";

let ctx: AudioContext | null = null;
let master: GainNode, bedGain: GainNode, filter: BiquadFilterNode;
let enabled = false;
let recent: number[] = []; // ping timestamps (ms) for the polyphony guard

// An A pentatonic set — every pitch is consonant with every other, so bursts of
// event pings never clash.
const PENTA = [220, 261.63, 293.66, 349.23, 392.0, 440, 523.25, 587.33];

export function audioEnabled(): boolean { return enabled; }

export function enableAudio(): boolean {
  try {
    if (!ctx) {
      const AC: typeof AudioContext = (window.AudioContext || (window as any).webkitAudioContext);
      if (!AC) return false;
      ctx = new AC();
      master = ctx.createGain(); master.gain.value = 0;
      master.connect(ctx.destination);
      // a feedback delay for space
      const delay = ctx.createDelay(1); delay.delayTime.value = 0.3;
      const fb = ctx.createGain(); fb.gain.value = 0.3;
      master.connect(delay); delay.connect(fb); fb.connect(delay); delay.connect(ctx.destination);
      // bed: detuned drone through a lowpass that opens with activity
      filter = ctx.createBiquadFilter(); filter.type = "lowpass"; filter.frequency.value = 350; filter.Q.value = 0.7;
      bedGain = ctx.createGain(); bedGain.gain.value = 0;
      filter.connect(bedGain); bedGain.connect(master);
      [110, 164.81, 220].forEach((f, i) => {
        const o = ctx!.createOscillator(); o.type = "sine"; o.frequency.value = f; o.detune.value = (i - 1) * 4;
        o.connect(filter); o.start();
      });
    }
    void ctx.resume();
    enabled = true;
    master.gain.setTargetAtTime(0.12, ctx.currentTime, 0.4);   // hard ceiling 0.12 master
    bedGain.gain.setTargetAtTime(0.16, ctx.currentTime, 0.7);
    return true;
  } catch { enabled = false; return false; }
}

export function disableAudio() {
  enabled = false;
  if (!ctx) return;
  master.gain.setTargetAtTime(0, ctx.currentTime, 0.2);
  const c = ctx;
  setTimeout(() => { if (!enabled) void c.suspend(); }, 450);
}

// The continuous score: called from the existing activity-decay rAF tick.
export function scoreActivity(a: number) {
  if (!enabled || !ctx || reduced()) return;
  const t = ctx.currentTime;
  bedGain.gain.setTargetAtTime(0.12 + a * 0.16, t, 0.3);
  filter.frequency.setTargetAtTime(300 + a * 1700, t, 0.25);
}

// Discrete foley for an agent event. Each ping is a short, self-cleaning voice.
export function ping(kind: string) {
  if (!enabled || !ctx || reduced()) return;
  const nowMs = ctx.currentTime * 1000;
  recent = recent.filter((x) => nowMs - x < 100);
  if (recent.length > 6) return; // machine-gun guard
  recent.push(nowMs);
  const t = ctx.currentTime;
  const notes: number[] =
    kind === "tool_call" ? [PENTA[4], PENTA[6]] :
    kind === "tool_result" ? [PENTA[5]] :
    kind === "approval" ? [PENTA[3], PENTA[5]] :
    kind === "error" ? [98, 90] :
    kind === "end" ? [PENTA[2], PENTA[4], PENTA[6]] :
    [PENTA[4]];
  notes.forEach((f, i) => {
    const o = ctx!.createOscillator(); o.type = kind === "error" ? "sawtooth" : "triangle"; o.frequency.value = f;
    const g = ctx!.createGain();
    const st = t + i * 0.07;
    g.gain.setValueAtTime(0, st);
    g.gain.linearRampToValueAtTime(0.16, st + 0.008);
    g.gain.exponentialRampToValueAtTime(0.0001, st + 0.2);
    o.connect(g); g.connect(master);
    o.start(st); o.stop(st + 0.24);
  });
}
