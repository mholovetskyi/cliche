import React, { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import hljs from "highlight.js";
import {
  ShieldCheck, ShieldAlert, Download, RefreshCw, ExternalLink, ArrowUp, Sparkles,
  Check, Wrench, Globe, Wand2, Hammer, FileSearch, KeyRound, CircleAlert, Plus,
  MessageSquare, Folder, FolderOpen, FileText, Eye, ListTree, ChevronRight, Square,
  FileDiff, ImagePlus, X, CornerDownLeft, Trash2, Search, Keyboard, Volume2, Sparkle, Star, SlidersHorizontal,
  GitBranch, AtSign, PanelRight, Copy, Pencil, Pin, Rocket, Clock, Mic,
} from "lucide-react";

type GitStatus = { repo: boolean; gh: boolean; branch: string; dirty: boolean; stat: string; files: string[] };
function flattenTree(nodes: any[]): string[] {
  const out: string[] = [];
  const walk = (ns: any[]) => ns.forEach((n) => { if (n.dir) { if (n.children) walk(n.children); } else out.push(n.path); });
  walk(nodes || []);
  return out;
}
const ansiRe = /\x1b\[[0-9;]*m/g;

// CodeBlock — wraps a markdown <pre> with a hover copy button.
function CodeBlock(props: any) {
  const ref = useRef<HTMLPreElement>(null);
  const [done, setDone] = useState(false);
  const copy = () => {
    const t = ref.current?.innerText ?? "";
    navigator.clipboard?.writeText(t).then(() => { setDone(true); setTimeout(() => setDone(false), 1200); }).catch(() => {});
  };
  return (
    <div className="group/code relative">
      <pre ref={ref} {...props} />
      <button onClick={copy} title="Copy code"
        className="icon-btn absolute right-2 top-2 h-7 w-7 rounded-lg border border-[var(--line)] bg-[var(--s2)]/80 opacity-0 backdrop-blur transition-opacity group-hover/code:opacity-100">
        {done ? <Check size={13} className="text-[var(--ok)]" /> : <Copy size={13} />}
      </button>
    </div>
  );
}
const MD_COMPONENTS = { pre: CodeBlock };

// useIsMobile tracks a phone-width viewport so the 3-pane layout collapses to a
// single pane + bottom tabs (and the same applies to a narrow desktop window).
function useIsMobile() {
  const q = "(max-width: 768px)";
  const [m, setM] = useState(() => typeof window !== "undefined" && window.matchMedia(q).matches);
  useEffect(() => {
    const mq = window.matchMedia(q);
    const on = () => setM(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return m;
}

type MobileView = "chat" | "work" | "sessions";

// MobileTabBar is the bottom navigation shown only on phone-width screens: it
// switches which single pane is visible. Honors the iOS home-indicator safe area.
function MobileTabBar({ view, onView }: { view: MobileView; onView: (v: MobileView) => void }) {
  const tabs: { id: MobileView; label: string; icon: any }[] = [
    { id: "chat", label: "Chat", icon: MessageSquare },
    { id: "work", label: "Preview", icon: Eye },
    { id: "sessions", label: "Menu", icon: ListTree },
  ];
  return (
    <nav className="glass flex shrink-0 items-stretch border-t border-[var(--line)]" style={{ paddingBottom: "env(safe-area-inset-bottom)" }}>
      {tabs.map((t) => (
        <button key={t.id} onClick={() => onView(t.id)} aria-label={t.label}
          className="flex flex-1 flex-col items-center justify-center gap-1 py-2.5 text-[10.5px] font-medium transition-colors"
          style={{ color: view === t.id ? "var(--accent)" : "var(--dim)" }}>
          <t.icon size={20} strokeWidth={view === t.id ? 2.4 : 2} /> {t.label}
        </button>
      ))}
    </nav>
  );
}

const PROVIDERS = [
  { id: "anthropic", label: "Anthropic (Claude) — direct API", keyUrl: "https://console.anthropic.com", local: false },
  { id: "openai", label: "OpenAI", keyUrl: "https://platform.openai.com/api-keys", local: false },
  { id: "openrouter", label: "OpenRouter — many models, one key", keyUrl: "https://openrouter.ai/keys", local: false },
  { id: "google", label: "Google (Gemini)", keyUrl: "https://aistudio.google.com/apikey", local: false },
  { id: "groq", label: "Groq — very fast", keyUrl: "https://console.groq.com/keys", local: false },
  { id: "ollama", label: "Ollama — runs locally, no key", keyUrl: "", local: true },
];
import ShaderField from "./ShaderField";
import DepthField from "./DepthField";
import MemoryConstellation from "./MemoryConstellation";
import { LogoMark } from "./Logo";
import { api, sseUrl, apiBase, setServer, clearServer, isApp } from "./lib/api";
import { enableAudio, disableAudio, scoreActivity, ping } from "./lib/audio";
import { REDUCE, flag, setFlag } from "./lib/reduced";

const QUIPS: Record<string, string[]> = {
  idle: ["Standing by. The ledger is quiet.", "Every action, signed. Ask away.", "I keep the receipts so you don't have to."],
  focused: ["Working — watching every write.", "On it. Nothing slips the ledger.", "Forging. Each step is accounted for."],
  proud: ["Done, and verified. The chain holds.", "Receipts filed. Spotless.", "Built it — and I can prove every step."],
  frugal: ["Spend's getting warm. Mind the meter.", "We're nearing the cap. Want me to ease off?", "Costs are climbing — I'm watching."],
  wary: ["The ledger doesn't match. I don't like it.", "Tamper detected. Tread carefully.", "Something rewrote the record. Stay sharp."],
};
function pickQuip(mood: string): string { const a = QUIPS[mood] || QUIPS.idle; return a[Math.floor(Math.random() * a.length)]; }

const ACCENTS = [
  { id: "coral", a: "#ff6a4d", a2: "#ff9468" },
  { id: "violet", a: "#a78bfa", a2: "#c4b5fd" },
  { id: "emerald", a: "#34d399", a2: "#6ee7b7" },
  { id: "sky", a: "#56b6ff", a2: "#93d0ff" },
  { id: "amber", a: "#fbbf24", a2: "#fcd34d" },
];
function applyAccent(id: string) {
  const acc = ACCENTS.find((x) => x.id === id) || ACCENTS[0];
  const r = document.documentElement.style;
  r.setProperty("--accent", acc.a);
  r.setProperty("--accent2", acc.a2);
  r.setProperty("--accent-ink", "#0b0b0d");
  try { localStorage.setItem("cliche-accent", id); } catch { /* ignore */ }
}

type PItem = { id: string; group: string; label: string; hint?: string; run: () => void };
function fuzzy(q: string, s: string): number | null {
  if (!q) return 1;
  q = q.toLowerCase(); s = s.toLowerCase();
  let i = 0, score = 0, last = -2;
  for (let j = 0; j < s.length && i < q.length; j++) {
    if (s[j] === q[i]) { score += (j === last + 1 ? 4 : 1) + (j === 0 ? 3 : 0); last = j; i++; }
  }
  return i === q.length ? score : null;
}

function CommandPalette({ items, onClose }: { items: PItem[]; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => { inputRef.current?.focus(); }, []);
  const results = items
    .map((it) => ({ it, s: fuzzy(q, `${it.label} ${it.hint || ""} ${it.group}`) }))
    .filter((x) => x.s !== null)
    .sort((a, b) => (b.s as number) - (a.s as number))
    .slice(0, 30)
    .map((x) => x.it);
  useEffect(() => { setIdx(0); }, [q]);
  function key(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") { e.preventDefault(); setIdx((i) => Math.min(i + 1, results.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setIdx((i) => Math.max(i - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); results[idx]?.run(); onClose(); }
    else if (e.key === "Escape") { e.preventDefault(); onClose(); }
  }
  return (
    <div className="fade-in fixed inset-0 z-[100] flex items-start justify-center bg-black/55 p-4 pt-[13vh] backdrop-blur-sm" onClick={onClose}>
      <div className="glass elev pop-in w-[580px] overflow-hidden rounded-2xl border border-[var(--line2)]" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-2.5 border-b border-[var(--line)] px-4">
          <Search size={16} className="text-[var(--dim)]" />
          <input ref={inputRef} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={key} placeholder="Search commands, chats, views…" className="w-full bg-transparent py-3.5 text-[15px] outline-none placeholder:text-[var(--dim)]" />
          <span className="kbd">esc</span>
        </div>
        <div className="max-h-[52vh] overflow-auto p-2">
          {results.length === 0 && <div className="px-3 py-8 text-center text-sm text-[var(--dim)]">No matches</div>}
          {results.map((it, i) => (
            <button key={it.id} onMouseEnter={() => setIdx(i)} onClick={() => { it.run(); onClose(); }}
              className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-[13.5px] transition-colors ${i === idx ? "bg-[var(--accent)]/[0.14] text-[var(--ink)]" : "text-[var(--mut)]"}`}>
              <span className="w-12 shrink-0 text-[10px] uppercase tracking-wider text-[var(--faint)]">{it.group}</span>
              <span className="flex-1 truncate">{it.label}</span>
              {it.hint && <span className="font-mono text-[11px] text-[var(--dim)]">{it.hint}</span>}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-3 border-t border-[var(--line)] px-4 py-2 text-[11px] text-[var(--faint)]">
          <span><span className="kbd">↑↓</span> navigate</span><span><span className="kbd">↵</span> select</span><span className="flex-1" /><span>Cliché Studio</span>
        </div>
      </div>
    </div>
  );
}

function Boot() {
  return (
    <div className="boot fixed inset-0 z-[200] grid place-items-center bg-[var(--bg)]">
      <div className="text-center">
        <div className="boot-mark mx-auto mb-4 w-16 text-[#e8e8ea]"><LogoMark size={64} /></div>
        <div className="fade-up text-xl font-semibold tracking-tight" style={{ animationDelay: ".15s" }}>Cliché <span className="text-[var(--dim)]">Studio</span></div>
        <div className="fade-up text-xs text-[var(--mut)]" style={{ animationDelay: ".25s" }}>the trustworthy build-anything app</div>
      </div>
    </div>
  );
}

type Ev = { kind: string; text?: string; data?: any };
type State = { model?: string; provider?: string; mode?: string; spent_usd?: number; cap_usd?: number; ctx_frac?: number; running?: boolean; needs_setup?: boolean };
type Template = { name: string; desc: string; prompt: string };
type Audit = { ok: boolean; entries: number; verified: number; usd: number; turns: number; input_tokens?: number; output_tokens?: number; reason?: string; verdicts?: Record<string, number> };
type SessionMeta = { id: string; title: string; model: string; updated: string; messages: number; active: boolean };
type FileNode = { name: string; path: string; dir: boolean; children?: FileNode[] };
type Msg = { role: string; text: string };
type ModelInfo = { model: string; input_per_m: number; output_per_m: number };
type Change = { path: string; before: string; after: string; was_new: boolean; deleted: boolean };
type Task = { id: number; title: string; done: boolean };
type CommandInfo = { name: string; desc: string };
type Rules = { mode: string; mode_desc: string; allow: string[]; deny: string[]; egress: string[]; hooks: string[]; max_turns: number; max_wall_sec: number; max_failed_edits: number };

type DiffRow = { type: "ctx" | "add" | "del"; text: string };
function lineDiff(before: string, after: string): DiffRow[] {
  const a = before.split("\n"), b = after.split("\n");
  const n = a.length, m = b.length;
  if (n * m > 400000) return b.map((t) => ({ type: "add", text: t }));
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const out: DiffRow[] = [];
  let i = 0, j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) { out.push({ type: "ctx", text: a[i] }); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) { out.push({ type: "del", text: a[i] }); i++; }
    else { out.push({ type: "add", text: b[j] }); j++; }
  }
  while (i < n) out.push({ type: "del", text: a[i++] });
  while (j < m) out.push({ type: "add", text: b[j++] });
  return out;
}

const MODES = [
  { id: "plan", label: "Plan" },
  { id: "suggest", label: "Suggest" },
  { id: "auto-edit", label: "Auto" },
  { id: "full", label: "Full" },
];

const COMMANDS: { cmd: string; desc: string; arg?: boolean }[] = [
  { cmd: "new", desc: "Start a new chat" },
  { cmd: "plan", desc: "Add a task to the plan", arg: true },
  { cmd: "done", desc: "Mark a task done", arg: true },
  { cmd: "image", desc: "Attach an image" },
  { cmd: "stop", desc: "Stop the current run" },
  { cmd: "mode", desc: "Set permission mode", arg: true },
  { cmd: "suggest", desc: "Mode → ask before each action" },
  { cmd: "auto", desc: "Mode → auto-apply edits" },
  { cmd: "full", desc: "Mode → auto-approve everything" },
  { cmd: "model", desc: "Switch model", arg: true },
  { cmd: "undo", desc: "Undo the last file change" },
  { cmd: "rewind", desc: "Revert every change this session" },
  { cmd: "changes", desc: "Show what changed (diffs)" },
  { cmd: "git", desc: "Git — commit, branch, open a PR" },
  { cmd: "preview", desc: "Show the live preview" },
  { cmd: "files", desc: "Show the file tree" },
  { cmd: "trust", desc: "Show the trust ledger + rules" },
  { cmd: "focus", desc: "Chat only — hide the side panel (⌘\\)" },
  { cmd: "split", desc: "Show the side panel" },
  { cmd: "export", desc: "Download the project (.zip)" },
];
type Item =
  | { t: "you"; text: string }
  | { t: "assistant"; text: string }
  | { t: "tool"; text: string }
  | { t: "result"; text: string; images?: string[] }
  | { t: "error"; text: string }
  | { t: "end" }
  | { t: "approval"; id: string; kind: string; target: string; answered?: "allowed" | "declined" };

const TEMPLATE_ICONS: Record<string, any> = { Website: Globe, "Automate a task": Wand2, "Small tool": Hammer, "Explain this project": FileSearch };

function relTime(iso: string): string {
  const t = new Date(iso).getTime();
  if (!t) return "";
  const s = Math.max(0, (Date.now() - t) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function Sigil({ size = 28, running, trust = "idle", load = 0, oracle, celebrate }: { size?: number; running?: boolean; trust?: "idle" | "intact" | "tamper"; load?: number; oracle?: boolean; celebrate?: boolean }) {
  const lit = Math.round(Math.max(0, Math.min(1, load)) * 24);
  return (
    <svg className="sigil shrink-0" data-running={!!running} data-trust={trust} data-celebrate={!!celebrate} viewBox="0 0 100 100" width={size} height={size} aria-hidden="true">
      <g className="corona">
        {Array.from({ length: 24 }, (_, i) => (
          <line key={i} x1="50" y1="7" x2="50" y2="13" strokeWidth="2.2" strokeLinecap="round" className={`tick ${i < lit ? "lit" : ""}`} transform={`rotate(${i * 15} 50 50)`} />
        ))}
      </g>
      <g className="g1" fill="none" strokeWidth="3" strokeLinecap="round">
        <circle className="arc" cx="50" cy="50" r="30" strokeDasharray="42 160" />
        <circle className="arc" cx="50" cy="50" r="30" strokeDasharray="20 160" strokeDashoffset="102" />
      </g>
      <g className="g2" fill="none" strokeWidth="2.5" strokeLinecap="round">
        <circle className="arc2" cx="50" cy="50" r="21" strokeDasharray="30 110" strokeDashoffset="40" />
        <circle className="arc2" cx="50" cy="50" r="21" strokeDasharray="14 110" strokeDashoffset="92" />
      </g>
      <circle className="core" cx="50" cy="50" r="11" />
      {oracle && (
        <g className="eyes" fill="#0b0b0d">
          <circle cx="45.6" cy="50" r="2.2" /><circle cx="54.4" cy="50" r="2.2" />
        </g>
      )}
    </svg>
  );
}
function Mark({ size = 28, running, trust, load }: { size?: number; running?: boolean; trust?: "idle" | "intact" | "tamper"; load?: number }) {
  return <Sigil size={size} running={running} trust={trust} load={load} />;
}

function Sparks({ origin, onDone }: { origin: { x: number; y: number }; onDone: () => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const cv = ref.current; if (!cv) return;
    const ctx = cv.getContext("2d"); if (!ctx) return;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    cv.width = innerWidth * dpr; cv.height = innerHeight * dpr; ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const cs = getComputedStyle(document.documentElement);
    const cols = [cs.getPropertyValue("--accent").trim() || "#ff6a4d", cs.getPropertyValue("--accent2").trim() || "#ff9468", cs.getPropertyValue("--ok").trim() || "#34d399"];
    const parts = Array.from({ length: 120 }, () => { const a = Math.random() * 6.283, sp = 2 + Math.random() * 7.5; return { x: origin.x, y: origin.y, vx: Math.cos(a) * sp, vy: Math.sin(a) * sp - 2.5, life: 1, c: cols[Math.floor(Math.random() * 3)], r: 1.4 + Math.random() * 2.2 }; });
    let raf = 0; const t = setTimeout(onDone, 1600);
    const frame = () => {
      raf = requestAnimationFrame(frame);
      if (document.hidden) return;
      ctx.clearRect(0, 0, innerWidth, innerHeight);
      ctx.globalCompositeOperation = "lighter";
      for (const p of parts) { p.vy += 0.18; p.vx *= 0.99; p.x += p.vx; p.y += p.vy; p.life -= 0.016; ctx.globalAlpha = Math.max(0, p.life); ctx.fillStyle = p.c; ctx.beginPath(); ctx.arc(p.x, p.y, p.r, 0, 6.283); ctx.fill(); }
      ctx.globalAlpha = 1; ctx.globalCompositeOperation = "source-over";
    };
    raf = requestAnimationFrame(frame);
    return () => { cancelAnimationFrame(raf); clearTimeout(t); };
  }, []);
  return <canvas ref={ref} className="pointer-events-none fixed inset-0 z-[130]" aria-hidden="true" />;
}

function Kiln() {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    if (REDUCE.matches) return;
    const cv = ref.current; if (!cv) return;
    const ctx = cv.getContext("2d"); if (!ctx) return;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const root = document.documentElement;
    const cols = () => [getComputedStyle(root).getPropertyValue("--accent").trim() || "#ff6a4d", getComputedStyle(root).getPropertyValue("--accent2").trim() || "#ff9468"];
    const resize = () => { cv.width = cv.clientWidth * dpr; cv.height = cv.clientHeight * dpr; ctx.setTransform(dpr, 0, 0, dpr, 0, 0); };
    const ro = new ResizeObserver(resize); ro.observe(cv);
    let parts: any[] = [], raf = 0, last = 0;
    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (document.hidden || t - last < 32) return; last = t;
      const W = cv.clientWidth, H = cv.clientHeight, cx = W / 2, cy = H / 2;
      const act = parseFloat(getComputedStyle(root).getPropertyValue("--activity")) || 0;
      const cl = cols();
      const spawn = Math.min(4, 1 + Math.floor(act * 6));
      for (let i = 0; i < spawn && parts.length < 110; i++) { const a = Math.random() * 6.283, d = 30 + Math.random() * 40; parts.push({ x: cx + Math.cos(a) * d, y: cy + Math.sin(a) * d + 20, vy: -(0.5 + Math.random() * 1.4), a, life: 1, c: cl[i % 2], r: 1 + Math.random() * 2 }); }
      ctx.clearRect(0, 0, W, H);
      ctx.globalCompositeOperation = "lighter";
      for (const p of parts) { p.a += 0.04; p.x += Math.cos(p.a) * 0.6; p.y += p.vy; p.life -= 0.012; ctx.globalAlpha = Math.min(0.5, Math.max(0, p.life) * 0.5); ctx.fillStyle = p.c; ctx.beginPath(); ctx.arc(p.x, p.y, p.r, 0, 6.283); ctx.fill(); }
      ctx.globalAlpha = 1; ctx.globalCompositeOperation = "source-over";
      parts = parts.filter((p) => p.life > 0);
    };
    raf = requestAnimationFrame(frame);
    return () => { cancelAnimationFrame(raf); ro.disconnect(); };
  }, []);
  return <canvas ref={ref} className="pointer-events-none absolute inset-0 h-full w-full" aria-hidden="true" />;
}
function trustOf(audit: Audit | null): "idle" | "intact" | "tamper" {
  if (!audit || audit.entries === 0) return "idle";
  return audit.ok ? "intact" : "tamper";
}

function BudgetReactor({ frac, spent, cap, running }: { frac: number; spent: number; cap: number; running: boolean }) {
  const [disp, setDisp] = useState(spent);
  const fromRef = useRef(spent);
  useEffect(() => {
    const from = fromRef.current, to = spent, t0 = performance.now(), dur = 500;
    let raf = 0;
    const tick = (t: number) => {
      const k = Math.min(1, (t - t0) / dur);
      setDisp(from + (to - from) * k);
      if (k < 1) raf = requestAnimationFrame(tick); else fromRef.current = to;
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [spent]);
  const R = 42, C = 2 * Math.PI * R, vis = 0.75 * C;
  const f = Math.max(0, Math.min(1, frac));
  const color = f < 0.6 ? "var(--ok)" : f < 0.85 ? "var(--warn)" : "var(--accent)";
  return (
    <div className="relative mx-auto grid place-items-center" style={{ width: 104, height: 104 }}>
      {running && <div className="reactor-sweep" />}
      <svg viewBox="0 0 100 100" width={104} height={104} style={{ transform: "rotate(135deg)" }}>
        <circle cx="50" cy="50" r={R} fill="none" stroke="rgba(255,255,255,.08)" strokeWidth="7" strokeLinecap="round" strokeDasharray={`${vis} ${C}`} />
        <circle cx="50" cy="50" r={R} fill="none" stroke={color} strokeWidth="7" strokeLinecap="round" strokeDasharray={`${f * vis} ${C}`} style={{ transition: "stroke-dasharray .6s cubic-bezier(.2,.7,.3,1), stroke .4s ease" }} />
      </svg>
      <div className="absolute text-center">
        <div className="font-mono text-[17px] tabular-nums">${disp.toFixed(3)}</div>
        {cap > 0 && <div className="text-[10px] text-[var(--dim)]">of ${cap.toFixed(2)}</div>}
      </div>
    </div>
  );
}

function TracerField({ bind }: { bind: (fn: (kind: string) => void) => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const cv = ref.current;
    if (!cv) return;
    const ctx = cv.getContext("2d");
    if (!ctx) return;
    const root = getComputedStyle(document.documentElement);
    const col = (k: string) => (k === "tool_result" ? root.getPropertyValue("--ok") : k === "error" ? "#ef4444" : root.getPropertyValue("--accent")).trim() || "#ff6a4d";
    let parts: any[] = [];
    let raf = 0, last = 0;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const resize = () => { cv.width = innerWidth * dpr; cv.height = innerHeight * dpr; ctx.setTransform(dpr, 0, 0, dpr, 0, 0); };
    resize(); window.addEventListener("resize", resize);
    bind((kind) => {
      if (parts.length > 110) return;
      const w = innerWidth, h = innerHeight;
      const sx = w * (0.30 + Math.random() * 0.12), sy = h * (0.45 + Math.random() * 0.2);
      const ex = w * (0.66 + Math.random() * 0.18), ey = h * (0.2 + Math.random() * 0.5);
      const c = col(kind);
      if (kind === "tool_result") { parts.push({ ring: true, x: ex, y: ey, r: 3, c, life: 1 }); }
      else { const life = 60 + Math.random() * 20; parts.push({ x: sx, y: sy, vx: (ex - sx) / life, vy: (ey - sy) / life, c, life, maxLife: life, r: 1.8 + Math.random() }); }
    });
    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (document.hidden || parts.length === 0) { ctx.clearRect(0, 0, innerWidth, innerHeight); return; }
      if (t - last < 32) return; // ~30fps
      last = t;
      ctx.clearRect(0, 0, innerWidth, innerHeight);
      for (const p of parts) {
        if (p.ring) {
          p.r += 1.6; p.life -= 0.04;
          ctx.beginPath(); ctx.arc(p.x, p.y, p.r, 0, 6.283); ctx.strokeStyle = p.c; ctx.globalAlpha = Math.max(0, p.life) * 0.6; ctx.lineWidth = 1.4; ctx.stroke();
        } else {
          p.x += p.vx; p.y += p.vy; p.life -= 1;
          const a = Math.max(0, p.life / p.maxLife);
          ctx.beginPath(); ctx.arc(p.x, p.y, p.r, 0, 6.283); ctx.fillStyle = p.c; ctx.globalAlpha = a * 0.85; ctx.shadowBlur = 10; ctx.shadowColor = p.c; ctx.fill(); ctx.shadowBlur = 0;
        }
      }
      ctx.globalAlpha = 1;
      parts = parts.filter((p) => p.life > 0);
    };
    raf = requestAnimationFrame(frame);
    return () => { cancelAnimationFrame(raf); window.removeEventListener("resize", resize); };
  }, []);
  return <canvas ref={ref} className="tracer" aria-hidden="true" />;
}

const SHORTCUTS = [
  { keys: "⌘K", desc: "Command palette" },
  { keys: "⌘N", desc: "New chat" },
  { keys: "?", desc: "This cheat-sheet" },
  { keys: "⌘\\", desc: "Hide / show the side panel (chat only)" },
  { keys: "g p / f / c / t", desc: "Go to Preview / Files / Changes / Trust" },
  { keys: "g n", desc: "New chat" },
  { keys: "/", desc: "Slash commands" },
  { keys: "@", desc: "Include a file" },
  { keys: "↵", desc: "Send" },
  { keys: "Esc", desc: "Close · stop" },
];
function KeysOverlay({ onClose }: { onClose: () => void }) {
  function move(e: React.MouseEvent<HTMLDivElement>) {
    const r = e.currentTarget.getBoundingClientRect();
    e.currentTarget.style.setProperty("--mx", `${e.clientX - r.left}px`);
    e.currentTarget.style.setProperty("--my", `${e.clientY - r.top}px`);
  }
  return (
    <div className="fade-in fixed inset-0 z-[110] grid place-items-center bg-black/55 p-4 backdrop-blur-sm" onClick={onClose}>
      <div onMouseMove={move} onClick={(e) => e.stopPropagation()} className="sheen glass elev pop-in relative w-[560px] overflow-hidden rounded-2xl border border-[var(--line2)] p-6">
        <div className="relative z-10">
          <div className="mb-4 flex items-center gap-2 text-sm font-semibold"><Keyboard size={16} className="text-[var(--accent)]" /> Keyboard shortcuts</div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-1">
            {SHORTCUTS.map((s, i) => (
              <div key={i} className="flex items-center justify-between gap-3 py-1.5 text-[13px]">
                <span className="text-[var(--mut)]">{s.desc}</span><span className="kbd">{s.keys}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function Gauge({ frac }: { frac: number }) {
  const f = Math.max(0, Math.min(1, frac || 0));
  const color = f < 0.6 ? "var(--ok)" : f < 0.85 ? "var(--warn)" : "var(--accent)";
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-white/10">
      <div className="h-full rounded-full transition-all duration-500" style={{ width: `${f * 100}%`, background: color, boxShadow: `0 0 8px ${color}` }} />
    </div>
  );
}

function msgsToItems(msgs: Msg[]): Item[] {
  return msgs.map((m): Item =>
    m.role === "user" ? { t: "you", text: m.text } :
    m.role === "tool" ? { t: "tool", text: m.text } :
    { t: "assistant", text: m.text }
  );
}

function reduce(prev: Item[], e: Ev): Item[] {
  switch (e.kind) {
    case "delta": {
      const last = prev[prev.length - 1];
      if (last && last.t === "assistant") return [...prev.slice(0, -1), { t: "assistant", text: last.text + (e.text || "") }];
      return [...prev, { t: "assistant", text: e.text || "" }];
    }
    case "tool_call": return [...prev, { t: "tool", text: e.text || "" }];
    case "tool_result": return [...prev, { t: "result", text: e.text || "", images: e.data?.images }];
    case "approval": return [...prev, { t: "approval", id: e.data?.id, kind: e.data?.kind, target: e.data?.target }];
    case "error": return [...prev, { t: "error", text: e.text || "" }];
    case "end": return [...prev, { t: "end" }];
    default: return prev;
  }
}

function Setup({ onDone }: { onDone: () => void }) {
  const providers = [
    { id: "anthropic", label: "Anthropic (Claude)", keyUrl: "https://console.anthropic.com" },
    { id: "openai", label: "OpenAI", keyUrl: "https://platform.openai.com/api-keys" },
    { id: "openrouter", label: "OpenRouter — many models, one key", keyUrl: "https://openrouter.ai/keys" },
    { id: "google", label: "Google (Gemini)", keyUrl: "https://aistudio.google.com/apikey" },
    { id: "groq", label: "Groq — very fast", keyUrl: "https://console.groq.com/keys" },
    { id: "ollama", label: "Ollama — runs locally, no key", local: true },
  ];
  const [provider, setProvider] = useState("anthropic");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const p = providers.find((x) => x.id === provider)!;
  async function connect() {
    setBusy(true); setErr("");
    const r = await api("/api/setup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ provider, key }) });
    setBusy(false);
    if (r.status === 204) onDone(); else setErr((await r.text()) || "could not connect");
  }
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="surface elev fade-up w-[460px] rounded-3xl p-8">
        <div className="mb-4 flex items-center gap-3">
          <Mark size={38} />
          <div>
            <div className="text-[17px] font-semibold tracking-tight">Welcome to Cliché Studio</div>
            <div className="text-xs text-[var(--mut)]">Build anything · on your machine · your key</div>
          </div>
        </div>
        <p className="mb-7 text-sm text-[var(--mut)]">Connect a model to get started. Your key never leaves this computer.</p>
        <label className="mb-1.5 block text-xs font-medium text-[var(--mut)]">Provider</label>
        <select value={provider} onChange={(e) => setProvider(e.target.value)} className="field mb-4 w-full px-3.5 py-2.5 text-sm">
          {providers.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
        </select>
        {!p.local ? (
          <>
            <label className="mb-1.5 block text-xs font-medium text-[var(--mut)]">API key</label>
            <div className="field relative mb-2 flex items-center">
              <KeyRound size={15} className="absolute left-3 text-[var(--dim)]" />
              <input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="paste your key…" className="w-full bg-transparent py-2.5 pl-9 pr-3 font-mono text-sm outline-none" />
            </div>
            {p.keyUrl && <a href={p.keyUrl} target="_blank" className="text-xs text-[var(--accent)]">get a key →</a>}
          </>
        ) : <p className="mb-2 text-xs text-[var(--mut)]">Make sure Ollama is running on your machine — no key needed.</p>}
        {err && <div className="mt-3 flex items-center gap-1.5 text-xs text-[var(--accent)]"><CircleAlert size={13} /> {err}</div>}
        <button onClick={connect} disabled={busy || (!p.local && !key.trim())} className="btn-accent mt-6 w-full rounded-2xl py-3.5 text-[15px]">
          {busy ? "connecting…" : "Connect & start building"}
        </button>
      </div>
    </div>
  );
}

function Settings({ state, onClose, onApplied }: { state: State; onClose: () => void; onApplied: () => void }) {
  const [provider, setProvider] = useState(state.provider || "anthropic");
  const [key, setKey] = useState("");
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const p = PROVIDERS.find((x) => x.id === provider) || PROVIDERS[0];
  async function apply() {
    setBusy(true); setErr("");
    const r = await api("/api/provider", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ provider, key, model }) });
    setBusy(false);
    if (r.status === 204) { onApplied(); onClose(); } else setErr((await r.text()) || "could not switch provider");
  }
  return (
    <div className="fade-in fixed inset-0 z-[110] grid place-items-center bg-black/55 p-4 backdrop-blur-sm" onClick={onClose}>
      <div onClick={(e) => e.stopPropagation()} className="glass elev pop-in w-[460px] rounded-2xl border border-[var(--line2)] p-6">
        <div className="mb-1 flex items-center gap-2 text-sm font-semibold"><SlidersHorizontal size={16} className="text-[var(--accent)]" /> Settings</div>
        <div className="mb-5 text-xs text-[var(--mut)]">Currently: <b className="text-[var(--ink)]">{state.provider || "—"}</b> · <span className="font-mono">{state.model || "—"}</span>{state.mode ? ` · ${state.mode}` : ""}</div>
        <label className="mb-1.5 block text-xs font-medium text-[var(--mut)]">Provider</label>
        <select value={provider} onChange={(e) => setProvider(e.target.value)} className="field mb-3 w-full px-3 py-2 text-sm">
          {PROVIDERS.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
        </select>
        {!p.local && (
          <>
            <label className="mb-1.5 block text-xs font-medium text-[var(--mut)]">API key {state.provider === provider ? "(blank = keep current)" : ""}</label>
            <div className="field relative mb-1 flex items-center">
              <KeyRound size={15} className="absolute left-3 text-[var(--dim)]" />
              <input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="paste your key…" className="w-full bg-transparent py-2 pl-9 pr-3 font-mono text-sm outline-none" />
            </div>
            {p.keyUrl && <a href={p.keyUrl} target="_blank" className="text-xs text-[var(--accent)]">get a key →</a>}
          </>
        )}
        <label className="mb-1.5 mt-3 block text-xs font-medium text-[var(--mut)]">Model (optional)</label>
        <input value={model} onChange={(e) => setModel(e.target.value)} placeholder={p.local ? "e.g. llama3.2" : "blank = the provider's strong default"} className="field w-full bg-transparent px-3 py-2 font-mono text-sm outline-none" />
        {err && <div className="mt-3 flex items-center gap-1.5 text-xs text-[var(--accent)]"><CircleAlert size={13} /> {err}</div>}
        <button onClick={apply} disabled={busy} className="btn-accent mt-5 w-full rounded-xl py-3 text-sm">{busy ? "switching…" : "Connect & switch"}</button>
        <div className="mt-2 text-center text-[11px] text-[var(--dim)]">Keeps your current conversation. Your key stays on this computer.</div>
      </div>
    </div>
  );
}

function Sidebar({ sessions, state, audit, tasks, accent, inst, isMobile, mobileShow, onNew, onPick, onRename, onDelete, onToggleTask, onClearTasks, onAccent, onSearch, onSettings, onToggleInst, onMemory }: {
  sessions: SessionMeta[]; state: State; audit: Audit | null; tasks: Task[]; accent: string; inst: { substrate: boolean; sound: boolean; oracle: boolean }; isMobile?: boolean; mobileShow?: boolean;
  onNew: () => void; onPick: (id: string) => void; onRename: (id: string, title: string) => void; onDelete: (id: string) => void; onToggleTask: (id: number) => void; onClearTasks: () => void;
  onAccent: (id: string) => void; onSearch: () => void; onSettings: () => void; onToggleInst: (k: "substrate" | "sound" | "oracle") => void; onMemory: () => void;
}) {
  const cap = state.cap_usd || 0;
  const doneCount = tasks.filter((t) => t.done).length;
  const trust = trustOf(audit);
  const load = cap > 0 ? (state.spent_usd || 0) / cap : 0;
  const [quip, setQuip] = useState("");
  const [editId, setEditId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const startEdit = (id: string, title: string) => { setEditId(id); setDraft(title); };
  const commitEdit = () => { if (editId) onRename(editId, draft.trim()); setEditId(null); };
  const mood = trust === "tamper" ? "wary" : state.running ? "focused" : load > 0.85 ? "frugal" : (audit && audit.ok && audit.entries > 0 ? "proud" : "idle");
  return (
    <aside className={`flex flex-col border-r border-[var(--line)] ${isMobile ? (mobileShow ? "w-full" : "hidden") : "w-[244px] shrink-0"}`}>
      <div className="flex h-[52px] items-center gap-2.5 px-4">
        <span className="relative text-[#dcdce0]" onMouseEnter={() => inst.oracle && setQuip(pickQuip(mood))} onMouseLeave={() => setQuip("")}>
          <LogoMark size={28} />
          {quip && <div className="pop-in glass elev absolute left-0 top-9 z-30 w-52 rounded-xl border px-3 py-2 text-[12px] leading-snug text-[var(--mut)]" style={{ borderColor: trust === "tamper" ? "var(--danger)" : "var(--line2)" }}>{quip}</div>}
        </span>
        <span className="flex-1 text-[15px] font-semibold tracking-tight">Cliché <span className="text-[var(--dim)]">Studio</span></span>
        <button onClick={onSettings} className="icon-btn h-7 w-7" title="Settings — provider & model"><SlidersHorizontal size={15} /></button>
        <button onClick={onSearch} className="icon-btn h-7 w-7" title="Command palette (⌘K)"><Search size={15} /></button>
      </div>
      <div className="px-3 pb-1">
        <button onClick={onNew} className="btn-soft flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-sm font-medium">
          <Plus size={16} className="text-[var(--accent)]" /> New chat
          <span className="flex-1" /><span className="kbd">⌘N</span>
        </button>
      </div>
      <div className="mt-2 flex-1 overflow-auto px-2">
        <div className="px-2 pb-1 pt-1 text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--dim)]">Chats</div>
        {sessions.length === 0 && <div className="px-2 py-2 text-xs text-[var(--dim)]">No chats yet</div>}
        {sessions.map((s) => (
          <div key={s.id} onClick={() => editId !== s.id && onPick(s.id)} title={s.title}
            className={`group relative mb-0.5 flex w-full cursor-pointer items-center gap-2.5 rounded-lg py-2 pl-3 pr-1.5 text-left text-[13px] transition-colors ${s.active ? "bg-white/[0.06] text-[var(--ink)]" : "text-[var(--mut)] hover:bg-white/[0.035]"}`}>
            {s.active && <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-[var(--accent)]" />}
            <MessageSquare size={14} className={s.active ? "shrink-0 text-[var(--accent)]" : "shrink-0 text-[var(--dim)]"} />
            {editId === s.id ? (
              <input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => { if (e.key === "Enter") commitEdit(); if (e.key === "Escape") setEditId(null); }} onBlur={commitEdit}
                className="min-w-0 flex-1 rounded bg-black/30 px-1.5 py-0.5 text-[13px] outline-none ring-1 ring-[var(--accent)]/60" />
            ) : (
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{s.title || "New chat"}</span>
                <span className="block text-[11px] text-[var(--dim)]">{relTime(s.updated)} · {s.messages} msgs</span>
              </span>
            )}
            {editId !== s.id && (
              <span className="flex shrink-0 items-center opacity-0 transition-opacity group-hover:opacity-100">
                <button onClick={(e) => { e.stopPropagation(); startEdit(s.id, s.title || ""); }} className="icon-btn h-6 w-6" title="Rename"><Pencil size={12} /></button>
                <button onClick={(e) => { e.stopPropagation(); if (confirm("Delete this chat? This can't be undone.")) onDelete(s.id); }} className="icon-btn h-6 w-6 hover:text-[var(--danger)]" title="Delete"><Trash2 size={12} /></button>
              </span>
            )}
          </div>
        ))}
      </div>
      {tasks.length > 0 && (
        <div className="max-h-[42%] overflow-auto border-t border-[var(--line)] px-2 py-2">
          <div className="flex items-center gap-2 px-2 pb-1.5 text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--dim)]">
            <span>Plan</span>
            <span className="h-1 flex-1 overflow-hidden rounded-full bg-white/10"><span className="block h-full rounded-full bg-[var(--ok)] transition-all" style={{ width: `${(doneCount / tasks.length) * 100}%` }} /></span>
            <span className="tabular-nums">{doneCount}/{tasks.length}</span>
            <button onClick={onClearTasks} className="icon-btn h-5 w-5" title="Clear the plan"><Trash2 size={12} /></button>
          </div>
          {tasks.map((t) => (
            <button key={t.id} onClick={() => onToggleTask(t.id)} className="flex w-full items-start gap-2 rounded-md px-2 py-1 text-left text-[13px] hover:bg-white/[0.04]">
              <span className={`mt-0.5 grid h-[15px] w-[15px] shrink-0 place-items-center rounded-[5px] border transition-colors ${t.done ? "border-[var(--ok)] bg-[var(--ok)]/20 text-[var(--ok)]" : "border-[var(--line2)]"}`}>{t.done && <Check size={10} strokeWidth={3} />}</span>
              <span className={t.done ? "text-[var(--dim)] line-through" : "text-[var(--mut)]"}>{t.title}</span>
            </button>
          ))}
        </div>
      )}
      <div className="border-t border-[var(--line)] p-3">
        <div className="surface rounded-xl p-3">
          {audit && audit.entries > 0 && (
            <div className="mb-1 flex items-center gap-1.5 text-xs">
              {audit.ok ? <span className="flex items-center gap-1 text-[var(--ok)]"><ShieldCheck size={13} /> verified</span>
                        : <span className="flex items-center gap-1 text-[var(--accent)]"><ShieldAlert size={13} /> tamper</span>}
              <span className="text-[var(--dim)]">· {audit.entries} receipts</span>
            </div>
          )}
          <BudgetReactor frac={load} spent={state.spent_usd || 0} cap={cap} running={!!state.running} />
          <div className="mt-1 truncate text-center font-mono text-[11px] text-[var(--mut)]">{state.model || "—"}</div>
        </div>
        <div className="mt-2.5 flex items-center gap-2 px-1">
          <span className="text-[10.5px] uppercase tracking-[0.08em] text-[var(--faint)]">Theme</span>
          {ACCENTS.map((ac) => (
            <button key={ac.id} onClick={() => onAccent(ac.id)} title={ac.id}
              className={`h-3.5 w-3.5 rounded-full transition-transform hover:scale-110 ${accent === ac.id ? "ring-2 ring-white/70 ring-offset-1 ring-offset-[var(--bg)]" : ""}`}
              style={{ background: ac.a }} />
          ))}
        </div>
        <div className="mt-2 flex items-center gap-1 px-1">
          <span className="text-[10.5px] uppercase tracking-[0.08em] text-[var(--faint)]">Live</span>
          <span className="flex-1" />
          <button onClick={onMemory} title="Memory — fly through past sessions" className="icon-btn h-7 w-7"><Star size={14} /></button>
          <button onClick={() => onToggleInst("substrate")} title="Living substrate (WebGL)" className={`icon-btn h-7 w-7 ${inst.substrate ? "text-[var(--accent)]" : ""}`}><Sparkle size={14} /></button>
          <button onClick={() => onToggleInst("sound")} title="Sound — the agent's score" className={`icon-btn h-7 w-7 ${inst.sound ? "text-[var(--accent)]" : ""}`}><Volume2 size={14} /></button>
          <button onClick={() => onToggleInst("oracle")} title="Oracle — the Sigil speaks" className={`icon-btn h-7 w-7 ${inst.oracle ? "text-[var(--accent)]" : ""}`}><Eye size={14} /></button>
        </div>
      </div>
    </aside>
  );
}

function ApprovalCard({ it, onAnswer }: { it: Extract<Item, { t: "approval" }>; onAnswer: (id: string, allow: boolean) => void }) {
  // The approval target may carry a diff preview after the first line (writes/edits).
  const nl = it.target.indexOf("\n");
  const head = (nl >= 0 ? it.target.slice(0, nl) : it.target).trim();
  const diff = nl >= 0 ? it.target.slice(nl + 1).replace(ansiRe, "").replace(/^\s+\n/, "") : "";
  const diffLines = diff ? diff.split("\n").slice(0, 200) : [];
  return (
    <div className="fade-up my-3 overflow-hidden rounded-2xl border border-[var(--accent)]/35 bg-[var(--accent)]/[0.06]">
      <div className="flex items-center gap-2 px-4 py-3 text-sm">
        <span className="grid h-7 w-7 place-items-center rounded-lg bg-[var(--accent)]/15 text-[var(--accent)]"><CircleAlert size={15} /></span>
        <span>Cliche wants to <b className="text-[var(--accent)]">{it.kind}</b></span>
        <code className="ml-auto truncate rounded-md bg-black/30 px-2 py-0.5 font-mono text-[12.5px]">{head}</code>
      </div>
      {diffLines.length > 0 && (
        <pre className="max-h-56 overflow-auto border-t border-[var(--accent)]/15 px-3 py-2 font-mono text-[11.5px] leading-[1.5]">
          {diffLines.map((ln, i) => (
            <div key={i} className={/^\s*\+/.test(ln) ? "text-[var(--ok)]" : /^\s*-/.test(ln) ? "text-[var(--accent)]" : "text-[var(--mut)]"}>{ln || " "}</div>
          ))}
        </pre>
      )}
      <div className="border-t border-[var(--accent)]/20 px-4 py-2.5">
        {it.answered ? <span className="text-xs text-[var(--mut)]">{it.answered === "allowed" ? "✓ allowed" : "declined"}</span> : (
          <div className="flex gap-2">
            <button onClick={() => onAnswer(it.id, true)} className="rounded-lg bg-[var(--ok)] px-4 py-1.5 text-sm font-semibold text-[#06231a] transition-transform active:scale-95">Allow</button>
            <button onClick={() => onAnswer(it.id, false)} className="btn-soft rounded-lg px-4 py-1.5 text-sm">Not now</button>
          </div>
        )}
      </div>
    </div>
  );
}

function Row({ it, onAnswer }: { it: Item; onAnswer: (id: string, allow: boolean) => void }) {
  switch (it.t) {
    case "you": return <div className="fade-up my-4 flex justify-end"><div className="max-w-[82%] rounded-2xl rounded-br-md border border-[var(--line)] bg-white/[0.05] px-4 py-2.5 text-[14.5px] leading-relaxed">{it.text}</div></div>;
    case "assistant": return <div className="md fade-up my-3"><ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={MD_COMPONENTS}>{it.text}</ReactMarkdown></div>;
    case "tool": return <div className="fade-up my-1 flex items-center gap-2 text-[12.5px] text-[var(--mut)]"><Wrench size={12} className="text-[var(--accent)]" /> <span className="font-mono">{it.text}</span></div>;
    case "result": return (
      <div className="fade-up my-1">
        <div className="flex items-center gap-2 text-[12.5px] text-[var(--mut)]"><Check size={12} className="text-[var(--ok)]" strokeWidth={3} /> <span className="font-mono">{it.text}</span></div>
        {it.images && it.images.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-2 pl-5">
            {it.images.map((src, i) => (
              <a key={i} href={src} target="_blank" rel="noreferrer" className="block overflow-hidden rounded-xl border border-[var(--line2)] shadow-[var(--sh-md)] transition-transform hover:scale-[1.01]" title="What Cliché saw — click to open full size">
                <img src={src} alt="screenshot" className="max-h-72 max-w-full object-contain" />
              </a>
            ))}
          </div>
        )}
      </div>
    );
    case "error": return <div className="fade-up my-2 flex items-center gap-2 rounded-lg border border-[var(--accent)]/30 bg-[var(--accent)]/[0.06] px-3 py-2 text-[13px] text-[var(--accent)]"><CircleAlert size={14} /> {it.text}</div>;
    case "end": return <div className="my-4 flex items-center gap-3 text-[11px] uppercase tracking-wider text-[var(--faint)]"><span className="h-px flex-1 bg-[var(--line)]" /> done <span className="h-px flex-1 bg-[var(--line)]" /></div>;
    case "approval": return <ApprovalCard it={it} onAnswer={onAnswer} />;
  }
}

function Welcome({ templates, onPick }: { templates: Template[]; onPick: (p: string) => void }) {
  return (
    <div className="mx-auto mt-16 max-w-2xl px-4 text-center">
      <div className="fade-up mb-4 inline-flex items-center gap-2 rounded-full border border-[var(--line)] bg-white/[0.03] px-3 py-1 text-xs text-[var(--mut)]">
        <Sparkles size={13} className="text-[var(--accent)]" /> trust-first · local · your key
      </div>
      <h1 className="fade-up mb-2 text-[34px] font-semibold leading-tight tracking-tight">What do you want to <span className="bg-gradient-to-r from-[var(--accent2)] to-[var(--accent)] bg-clip-text text-transparent">build</span>?</h1>
      <p className="fade-up mb-10 text-[15px] text-[var(--mut)]">Describe it, or start from one of these. Cliche makes it — you watch, and approve each step.</p>
      <div className="grid grid-cols-2 gap-3 text-left">
        {templates.map((t) => {
          const Icon = TEMPLATE_ICONS[t.name] || Sparkles;
          return (
            <button key={t.name} onClick={() => onPick(t.prompt)} className="surface card-hover group flex items-start gap-3 rounded-2xl p-4">
              <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-[var(--line)] bg-white/[0.03] text-[var(--accent)] transition-colors group-hover:bg-[var(--accent)]/12"><Icon size={18} /></span>
              <span><span className="block font-medium">{t.name}</span><span className="mt-0.5 block text-[13px] text-[var(--mut)]">{t.desc}</span></span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function Tree({ nodes, depth, expanded, onToggle, onOpen, active }: { nodes: FileNode[]; depth: number; expanded: Set<string>; onToggle: (p: string) => void; onOpen: (p: string) => void; active: string }) {
  return (
    <>
      {nodes.map((n) => n.dir ? (
        <div key={n.path}>
          <button onClick={() => onToggle(n.path)} className="flex w-full items-center gap-1.5 rounded-md py-[5px] pr-2 text-left text-[13px] text-[var(--mut)] transition-colors hover:bg-white/[0.04]" style={{ paddingLeft: 8 + depth * 13 }}>
            <ChevronRight size={13} className={`shrink-0 text-[var(--dim)] transition-transform ${expanded.has(n.path) ? "rotate-90" : ""}`} />
            {expanded.has(n.path) ? <FolderOpen size={14} className="text-[var(--accent)]" /> : <Folder size={14} className="text-[var(--dim)]" />}
            <span className="truncate">{n.name}</span>
          </button>
          {expanded.has(n.path) && n.children && <Tree nodes={n.children} depth={depth + 1} expanded={expanded} onToggle={onToggle} onOpen={onOpen} active={active} />}
        </div>
      ) : (
        <button key={n.path} onClick={() => onOpen(n.path)} className={`flex w-full items-center gap-1.5 rounded-md py-[5px] pr-2 text-left text-[13px] transition-colors hover:bg-white/[0.04] ${active === n.path ? "bg-white/[0.06] text-[var(--ink)]" : "text-[var(--mut)]"}`} style={{ paddingLeft: 8 + depth * 13 + 14 }}>
          <FileText size={13} className="shrink-0 text-[var(--dim)]" /><span className="truncate">{n.name}</span>
        </button>
      ))}
    </>
  );
}

function RuleList({ label, items, empty }: { label: string; items: string[]; empty: string }) {
  return (
    <div className="flex gap-3 border-t border-[var(--line)] py-2.5 text-[13px] first:border-0">
      <span className="w-16 shrink-0 text-[var(--dim)]">{label}</span>
      {items.length === 0 ? <span className="text-[var(--dim)]">{empty}</span> :
        <span className="flex flex-wrap gap-1.5">{items.map((x, i) => <code key={i} className="rounded-md bg-white/[0.06] px-1.5 py-0.5 font-mono text-[12px]">{x}</code>)}</span>}
    </div>
  );
}

function TrustPanel({ a, rules }: { a: Audit | null; rules: Rules | null }) {
  const tiles = a ? [
    { label: "receipts", value: a.entries }, { label: "turns", value: a.turns },
    { label: "spent", value: `$${(a.usd || 0).toFixed(4)}` },
    { label: "tokens", value: `${(((a.input_tokens || 0) + (a.output_tokens || 0)) / 1000).toFixed(1)}k` },
  ] : [];
  return (
    <div className="overflow-auto p-5">
      {a && a.entries > 0 ? (
        <>
          <div className={`mb-4 flex items-center gap-2.5 rounded-xl border p-3.5 ${a.ok ? "border-[var(--ok)]/30 bg-[var(--ok)]/[0.06] text-[var(--ok)]" : "border-[var(--accent)]/40 bg-[var(--accent)]/[0.06] text-[var(--accent)]"}`}>
            {a.ok ? <ShieldCheck size={18} /> : <ShieldAlert size={18} />}
            <span className="text-[13px] font-medium leading-snug">{a.ok ? "Ledger intact — every action is a signed, hash-chained receipt." : `Tamper detected${a.reason ? `: ${a.reason}` : ""}`}</span>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {tiles.map((t) => (
              <div key={t.label} className="surface rounded-xl p-4">
                <div className="text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--dim)]">{t.label}</div>
                <div className="mt-1 font-mono text-2xl tabular-nums">{t.value}</div>
              </div>
            ))}
          </div>
          {a.verdicts && Object.keys(a.verdicts).length > 0 && (
            <div className="mt-5">
              <div className="mb-2 text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--dim)]">Verifier verdicts</div>
              <div className="flex flex-wrap gap-2">
                {Object.entries(a.verdicts).map(([k, v]) => (
                  <span key={k} className="chip">{k}: <b className="text-[var(--ink)]">{v}</b></span>
                ))}
              </div>
            </div>
          )}
        </>
      ) : <div className="mb-2 text-sm text-[var(--mut)]">No receipts yet — the trust ledger fills in as Cliche works.</div>}
      {rules && (
        <div className="mt-6">
          <div className="mb-2 text-[10.5px] font-medium uppercase tracking-[0.08em] text-[var(--dim)]">Rules in force</div>
          <div className="surface rounded-xl px-4 py-1.5">
            <RuleList label="mode" items={[`${rules.mode} — ${rules.mode_desc}`]} empty="" />
            <RuleList label="allow" items={rules.allow || []} empty="nothing pre-allowed (mode + prompts govern)" />
            <RuleList label="deny" items={rules.deny || []} empty="no hard denies" />
            <RuleList label="egress" items={rules.egress || []} empty="unrestricted (the web gate still applies)" />
            <RuleList label="hooks" items={rules.hooks || []} empty="none" />
            <RuleList label="guards" items={[`${rules.max_turns} turns · ${rules.max_wall_sec}s wall · ${rules.max_failed_edits} failed-edits`]} empty="" />
          </div>
        </div>
      )}
    </div>
  );
}

function ChangesPanel({ changes, onUndo, onRevertAll }: { changes: Change[]; onUndo: () => void; onRevertAll: () => void }) {
  if (changes.length === 0) return <div className="grid h-full place-items-center p-6 text-center text-sm text-[var(--dim)]"><span><FileDiff size={26} className="mx-auto mb-2 opacity-50" />No file changes yet — edits show here as diffs you can undo.</span></div>;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b border-[var(--line)] px-4 py-2.5 text-xs">
        <span className="text-[var(--mut)]">{changes.length} file{changes.length > 1 ? "s" : ""} changed</span>
        <span className="flex-1" />
        <button onClick={onUndo} className="btn-soft rounded-lg px-2.5 py-1 text-[var(--mut)]">Undo last</button>
        <button onClick={onRevertAll} className="rounded-lg border border-[var(--accent)]/40 px-2.5 py-1 text-[var(--accent)] transition-colors hover:bg-[var(--accent)]/10">Revert all</button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {changes.map((c) => {
          const badge = c.was_new ? "new" : c.deleted ? "deleted" : "modified";
          const rows = lineDiff(c.before, c.after);
          const shown = rows.slice(0, 400);
          const adds = rows.filter((r) => r.type === "add").length, dels = rows.filter((r) => r.type === "del").length;
          return (
            <div key={c.path} className="surface mb-3 overflow-hidden rounded-xl">
              <div className="flex items-center gap-2 border-b border-[var(--line)] px-3 py-2 font-mono text-[12px]">
                <FileText size={13} className="shrink-0 text-[var(--dim)]" />
                <span className="min-w-0 flex-1 truncate">{c.path}</span>
                <span className="text-[var(--ok)]">+{adds}</span><span className="text-[var(--accent)]">−{dels}</span>
                <span className={`rounded-full px-1.5 py-0.5 text-[10px] ${c.deleted ? "bg-[var(--accent)]/15 text-[var(--accent)]" : c.was_new ? "bg-[var(--ok)]/15 text-[var(--ok)]" : "bg-white/10 text-[var(--mut)]"}`}>{badge}</span>
              </div>
              <pre className="overflow-auto py-1 text-[12px] leading-[1.55]">
                {shown.map((r, i) => (
                  <div key={i} className={`flex ${r.type === "add" ? "bg-[var(--ok)]/[0.09]" : r.type === "del" ? "bg-[var(--accent)]/[0.09]" : ""}`}>
                    <span className={`w-6 shrink-0 select-none text-center ${r.type === "add" ? "text-[var(--ok)]" : r.type === "del" ? "text-[var(--accent)]" : "text-[var(--faint)]"}`}>{r.type === "add" ? "+" : r.type === "del" ? "−" : ""}</span>
                    <span className={`whitespace-pre-wrap pr-3 ${r.type === "add" ? "text-[var(--ok)]" : r.type === "del" ? "text-[var(--accent)]" : "text-[var(--mut)]"}`}>{r.text || " "}</span>
                  </div>
                ))}
                {rows.length > shown.length && <div className="pl-6 text-[var(--dim)]">… {rows.length - shown.length} more lines</div>}
              </pre>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function GitPanel({ onAsk, onChanged }: { onAsk: (p: string) => void; onChanged: () => void }) {
  const [g, setG] = useState<GitStatus | null>(null);
  const [msg, setMsg] = useState("");
  const [branch, setBranch] = useState("");
  const [busy, setBusy] = useState("");
  const [note, setNote] = useState<{ ok: boolean; text: string; url?: string } | null>(null);
  const refresh = () => api("/api/git").then((r) => r.json()).then(setG).catch(() => {});
  useEffect(() => { refresh(); }, []);
  async function commit() {
    if (!msg.trim()) return; setBusy("commit"); setNote(null);
    const r = await api("/api/git/commit", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ msg }) });
    setBusy("");
    if (r.ok) { const d = await r.json(); setNote({ ok: true, text: "Committed " + d.result }); setMsg(""); refresh(); onChanged(); }
    else setNote({ ok: false, text: await r.text() });
  }
  async function makeBranch() {
    if (!branch.trim()) return; setBusy("branch"); setNote(null);
    const r = await api("/api/git/branch", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: branch }) });
    setBusy("");
    if (r.status === 204) { setNote({ ok: true, text: "Switched to branch " + branch }); setBranch(""); refresh(); }
    else setNote({ ok: false, text: await r.text() });
  }
  async function pr() {
    setBusy("pr"); setNote(null);
    const r = await api("/api/git/pr", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ title: msg.split("\n")[0], body: msg }) });
    setBusy("");
    if (r.ok) { const d = await r.json(); setNote({ ok: true, text: "Pull request opened", url: d.url }); }
    else setNote({ ok: false, text: await r.text() });
  }
  async function deploy() {
    setBusy("deploy"); setNote(null);
    const r = await api("/api/deploy", { method: "POST" });
    setBusy("");
    if (r.ok) { const d = await r.json(); setNote({ ok: true, text: "Live! First publish can take ~1 min to go live.", url: d.url }); refresh(); }
    else setNote({ ok: false, text: await r.text() });
  }
  const NoteLine = note && (
    <div className={`mt-3 flex items-center gap-1.5 text-xs ${note.ok ? "text-[var(--ok)]" : "text-[var(--accent)]"}`}>
      {note.ok ? <Check size={13} /> : <CircleAlert size={13} />}
      <span className="break-all">{note.text}{note.url && <a href={note.url} target="_blank" className="ml-1 underline">open ↗</a>}</span>
    </div>
  );
  if (!g) return <div className="p-6 text-sm text-[var(--dim)]">Loading git…</div>;
  if (!g.repo) return (
    <div className="grid h-full place-items-center p-6 text-center text-sm text-[var(--dim)]">
      <div className="max-w-xs">
        <Rocket size={26} className="mx-auto mb-2 text-[var(--accent)] opacity-80" />
        Not a git repository — but you can still ship.<br />
        <span className="text-xs">Publish this project to a public URL (GitHub Pages):</span>
        <button onClick={deploy} disabled={busy === "deploy"} className="btn-accent mx-auto mt-3 flex items-center gap-2 rounded-lg px-3.5 py-2 text-sm">
          <Rocket size={14} /> {busy === "deploy" ? "Deploying…" : "Deploy to a live URL"}
        </button>
        {NoteLine}
        <div className="mt-3 text-[11px] text-[var(--faint)]">Or run <code className="rounded bg-white/10 px-1">git init</code> to enable commits & PRs.</div>
      </div>
    </div>
  );
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-auto p-4">
      <div className="mb-3 flex items-center gap-2 text-sm">
        <GitBranch size={15} className="text-[var(--accent)]" /> <b className="font-mono">{g.branch}</b>
        <span className="text-[var(--mut)]">· {g.dirty ? (g.stat || "uncommitted changes") : "clean"}</span>
        <span className="flex-1" />
        <button onClick={refresh} className="icon-btn h-7 w-7" title="Refresh"><RefreshCw size={14} /></button>
      </div>
      {g.files.length > 0 && (
        <div className="surface mb-4 max-h-40 overflow-auto rounded-xl p-1.5 font-mono text-[12px]">
          {g.files.map((f, i) => <div key={i} className="truncate px-2 py-0.5 text-[var(--mut)]">{f}</div>)}
        </div>
      )}
      <label className="mb-1.5 text-xs font-medium text-[var(--mut)]">Commit message</label>
      <textarea value={msg} onChange={(e) => setMsg(e.target.value)} rows={3} placeholder="feat: describe what you built…" className="field mb-2 w-full resize-none bg-transparent px-3 py-2 text-sm outline-none" />
      <div className="mb-5 flex items-center gap-2">
        <button onClick={commit} disabled={!g.dirty || !msg.trim() || busy === "commit"} className="btn-accent rounded-lg px-3.5 py-1.5 text-sm">{busy === "commit" ? "committing…" : "Commit"}</button>
        <button onClick={() => onAsk("Write a concise Conventional Commits message for the current uncommitted changes. Reply with ONLY the message.")} className="btn-soft rounded-lg px-3 py-1.5 text-sm">Draft in chat ↗</button>
        {g.gh && <button onClick={pr} disabled={busy === "pr"} className="btn-soft ml-auto rounded-lg px-3 py-1.5 text-sm">{busy === "pr" ? "opening…" : "Open PR"}</button>}
      </div>
      <label className="mb-1.5 text-xs font-medium text-[var(--mut)]">New branch</label>
      <div className="flex gap-2">
        <input value={branch} onChange={(e) => setBranch(e.target.value)} placeholder="feature/my-change" className="field flex-1 bg-transparent px-3 py-1.5 font-mono text-sm outline-none" />
        <button onClick={makeBranch} disabled={!branch.trim() || busy === "branch"} className="btn-soft rounded-lg px-3 py-1.5 text-sm">Create</button>
      </div>
      <div className="mt-5 border-t border-[var(--line)] pt-4">
        <label className="mb-1.5 block text-xs font-medium text-[var(--mut)]">Ship it</label>
        <button onClick={deploy} disabled={busy === "deploy"} className="btn-accent flex w-full items-center justify-center gap-2 rounded-lg px-3.5 py-2 text-sm">
          <Rocket size={15} /> {busy === "deploy" ? "Deploying…" : "Deploy to a live URL"}
        </button>
        <div className="mt-1.5 text-[11px] text-[var(--faint)]">Publishes the project to a public GitHub Pages URL (needs the gh CLI, signed in).</div>
      </div>
      {NoteLine}
    </div>
  );
}

// Connect — the mobile app's first-run screen: point it at a remote Cliché backend
// (the cloud gateway / a networked `cliche serve`) with an access token. Only shown
// inside the native shell; the browser/desktop talk to their own same-origin server.
function Connect({ onConnected }: { onConnected: () => void }) {
  const [url, setUrl] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  async function connect() {
    const base = url.trim().replace(/\/+$/, "");
    if (!base) { setErr("Enter your Cliché server URL"); return; }
    setBusy(true); setErr("");
    setServer(base, token.trim());
    try {
      const r = await api("/api/state");
      if (!r.ok) throw new Error(r.status === 401 ? "Token rejected" : `Server returned ${r.status}`);
      onConnected();
    } catch (e: any) {
      clearServer();
      setErr(String(e?.message || "").includes("fetch") ? "Couldn't reach that server" : (e?.message || "Connection failed"));
      setBusy(false);
    }
  }
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="surface w-full max-w-sm rounded-2xl p-6 fade-up">
        <div className="mb-5 flex items-center gap-3">
          <span className="text-[#e8e8ea]"><LogoMark size={40} /></span>
          <div>
            <div className="text-lg font-semibold tracking-tight">Connect to Cliché</div>
            <div className="text-xs text-[var(--mut)]">Point the app at your workspace</div>
          </div>
        </div>
        <label className="mb-1 block text-xs font-medium text-[var(--mut)]">Server URL</label>
        <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://your-cliche-server" autoCapitalize="off" autoCorrect="off" spellCheck={false}
          className="field mb-3 w-full bg-transparent px-3 py-2.5 text-sm outline-none" />
        <label className="mb-1 block text-xs font-medium text-[var(--mut)]">Access token</label>
        <input value={token} onChange={(e) => setToken(e.target.value)} type="password" placeholder="paste the token" autoCapitalize="off"
          className="field mb-4 w-full bg-transparent px-3 py-2.5 text-sm outline-none" />
        {err && <div className="mb-3 flex items-center gap-1.5 text-xs text-[var(--accent)]"><CircleAlert size={13} /> {err}</div>}
        <button onClick={connect} disabled={busy} className="btn-accent w-full rounded-xl py-2.5 text-sm">{busy ? "Connecting…" : "Connect"}</button>
        <div className="mt-3 text-center text-[11px] text-[var(--faint)]">The token authenticates the agent in your private sandbox.</div>
      </div>
    </div>
  );
}

// notifyDone fires a native "build finished" notification when a run completes
// while the app is backgrounded. It reaches Capacitor's LocalNotifications via the
// runtime bridge (window.Capacitor) rather than an npm import, so the shared web
// build pulls in zero Capacitor dependencies and this is a no-op in the browser.
function notifyDone() {
  if (typeof document === "undefined" || !document.hidden) return;
  const ln = (window as any).Capacitor?.Plugins?.LocalNotifications;
  if (!ln?.schedule) return;
  try {
    ln.schedule({ notifications: [{ title: "Cliché Studio", body: "Your build finished.", id: Math.floor(Date.now() % 100000) }] });
  } catch { /* ignore */ }
}

type CronJob = { id: string; spec: string; prompt: string; enabled: boolean; next: string; last_status: string };

// ScheduledPanel manages cron jobs from the web (the "Scheduled" tab). Jobs are
// fired by `cliche cron run`; each fire is Trust-Kernel-bounded — the GUI face of
// "leave it running."
function ScheduledPanel({ onRun }: { onRun: (prompt: string) => void }) {
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [spec, setSpec] = useState("@daily");
  const [prompt, setPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const refresh = () => api("/api/cron").then((r) => r.json()).then(setJobs).catch(() => {});
  useEffect(() => { refresh(); }, []);
  const post = (b: any) => api("/api/cron", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(b) });
  async function add() {
    if (!spec.trim() || !prompt.trim()) return;
    setBusy(true); setErr("");
    const r = await post({ action: "add", spec, prompt });
    setBusy(false);
    if (r.ok) { setPrompt(""); refresh(); } else setErr((await r.text()).trim());
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-auto p-4">
      <div className="mb-1 flex items-center gap-2 text-sm"><Clock size={15} className="text-[var(--accent)]" /> <b>Scheduled jobs</b></div>
      <div className="mb-4 text-[11px] leading-relaxed text-[var(--faint)]">Each fire runs through the Trust Kernel (budget cap + governor) — it can't run away. Start the scheduler with <code className="rounded bg-white/10 px-1">cliche cron run</code>.</div>
      <label className="mb-1 block text-xs font-medium text-[var(--mut)]">Schedule</label>
      <input value={spec} onChange={(e) => setSpec(e.target.value)} placeholder="@daily · @hourly · @every 30m · 0 9 * * 1-5" className="field mb-2 w-full bg-transparent px-3 py-2 font-mono text-xs outline-none" />
      <label className="mb-1 block text-xs font-medium text-[var(--mut)]">Prompt</label>
      <textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={2} placeholder="what to do on each run…" className="field mb-2 w-full resize-none bg-transparent px-3 py-2 text-sm outline-none" />
      <div className="mb-5 flex items-center gap-2">
        <button onClick={add} disabled={!spec.trim() || !prompt.trim() || busy} className="btn-accent rounded-lg px-3.5 py-1.5 text-sm">{busy ? "adding…" : "Schedule"}</button>
        {err && <span className="flex items-center gap-1 text-xs text-[var(--accent)]"><CircleAlert size={12} /> {err}</span>}
      </div>
      {jobs.length === 0 ? (
        <div className="text-sm text-[var(--dim)]">No scheduled jobs yet — add one above.</div>
      ) : (
        <div className="space-y-2">
          {jobs.map((j) => (
            <div key={j.id} className={`surface rounded-xl p-3 ${j.enabled ? "" : "opacity-55"}`}>
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs text-[var(--accent)]">{j.spec}</span>
                <span className="text-[11px] text-[var(--dim)]">next {j.next}</span>
                {j.last_status && <span className="chip text-[10px]">{j.last_status}</span>}
                <span className="flex-1" />
                <button onClick={() => onRun(j.prompt)} className="icon-btn h-6 w-6 hover:text-[var(--accent)]" title="Run now (in chat)"><ArrowUp size={13} className="rotate-90" /></button>
                <button onClick={() => post({ action: "toggle", id: j.id, enabled: !j.enabled }).then(refresh)} className="icon-btn h-6 w-6" title={j.enabled ? "Disable" : "Enable"} style={{ color: j.enabled ? "var(--ok)" : "var(--dim)" }}>{j.enabled ? <Check size={13} strokeWidth={3} /> : <Square size={12} />}</button>
                <button onClick={() => post({ action: "remove", id: j.id }).then(refresh)} className="icon-btn h-6 w-6 hover:text-[var(--danger)]" title="Remove"><Trash2 size={13} /></button>
              </div>
              <div className="mt-1.5 line-clamp-2 text-[13px] text-[var(--mut)]">{j.prompt}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default function App() {
  const [items, setItems] = useState<Item[]>([]);
  const [state, setState] = useState<State>({});
  const [prompt, setPrompt] = useState("");
  const [templates, setTemplates] = useState<Template[]>([]);
  const [audit, setAudit] = useState<Audit | null>(null);
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [changes, setChanges] = useState<Change[]>([]);
  const [rules, setRules] = useState<Rules | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [commands, setCommands] = useState<CommandInfo[]>([]);
  const [imgCount, setImgCount] = useState(0);
  const [dragOver, setDragOver] = useState(false);
  const [listening, setListening] = useState(false);
  const recogRef = useRef<any>(null);
  const speechOK = typeof window !== "undefined" && !!((window as any).SpeechRecognition || (window as any).webkitSpeechRecognition);
  // Skip the in-page intro when launched from the desktop shell — its native
  // splash already covered the boot (so the hand-off isn't a double animation).
  const [booting, setBooting] = useState(() => { try { return !new URLSearchParams(location.search).has("desktop"); } catch { return true; } });
  // loaded gates the main render on the first /api/state result, so a fresh install
  // never flashes the empty workspace for a frame before the Setup screen.
  const [loaded, setLoaded] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [showKeys, setShowKeys] = useState(false);
  const [leader, setLeader] = useState(false);
  const [awaiting, setAwaiting] = useState(false);
  const [ritual, setRitual] = useState(false);
  const [accent, setAccent] = useState("coral");
  const [inst, setInst] = useState({ substrate: false, sound: false, oracle: false });
  const [showMemory, setShowMemory] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [celebrate, setCelebrate] = useState(false);
  const [workspaceOpen, setWorkspaceOpen] = useState(() => { try { return localStorage.getItem("cliche-workspace") !== "off"; } catch { return true; } });
  const isMobile = useIsMobile();
  const [mobileView, setMobileView] = useState<MobileView>("chat");
  const [previewKey, setPreviewKey] = useState(0);
  const [tab, setTab] = useState<"preview" | "files" | "changes" | "git" | "schedule" | "trust">("preview");
  const [tree, setTree] = useState<FileNode[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [openFile, setOpenFile] = useState<{ path: string; html: string } | null>(null);
  const [pill, setPill] = useState({ left: 0, width: 0 });
  const feedRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const tabBarRef = useRef<HTMLDivElement>(null);
  const activityRef = useRef(0);
  const tracerRef = useRef<(kind: string) => void>(() => {});
  const depthRef = useRef<(kind: string) => void>(() => {});
  const leaderRef = useRef(false);
  const lastCelebRef = useRef(0);

  const refreshAudit = () => api("/api/audit").then((r) => r.json()).then((a) => { setAudit(a); if (lastCelebRef.current === 0) lastCelebRef.current = a.entries || 0; }).catch(() => {});
  const refreshSessions = () => api("/api/sessions").then((r) => r.json()).then(setSessions).catch(() => {});
  const refreshFiles = () => api("/api/files").then((r) => r.json()).then(setTree).catch(() => {});
  const refreshChanges = () => api("/api/changes").then((r) => r.json()).then(setChanges).catch(() => {});
  const refreshRules = () => api("/api/rules").then((r) => r.json()).then(setRules).catch(() => {});
  const refreshTasks = () => api("/api/tasks").then((r) => r.json()).then(setTasks).catch(() => {});
  useEffect(() => { feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight, behavior: "smooth" }); }, [items]);

  useEffect(() => {
    let saved = "coral";
    try { saved = localStorage.getItem("cliche-accent") || "coral"; } catch { /* ignore */ }
    setAccent(saved); applyAccent(saved);
    const sub = flag("cliche-substrate"), orc = flag("cliche-oracle");
    setInst({ substrate: sub, sound: false, oracle: orc });
    document.documentElement.setAttribute("data-substrate", sub ? "on" : "off");
    const t = setTimeout(() => setBooting(false), 1150);
    let leaderTimer = 0;
    const isTyping = () => { const el = document.activeElement as HTMLElement | null; const tag = el?.tagName; return tag === "INPUT" || tag === "TEXTAREA" || !!el?.isContentEditable; };
    const onKey = (e: KeyboardEvent) => {
      if (leaderRef.current) {
        leaderRef.current = false; setLeader(false); window.clearTimeout(leaderTimer);
        const map: Record<string, "preview" | "files" | "changes" | "trust"> = { p: "preview", f: "files", c: "changes", t: "trust" };
        if (map[e.key]) { e.preventDefault(); setTab(map[e.key]); return; }
        if (e.key === "n") { e.preventDefault(); newChat(); return; }
        return;
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") { e.preventDefault(); setPaletteOpen((o) => !o); return; }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "n") { e.preventDefault(); newChat(); return; }
      if ((e.metaKey || e.ctrlKey) && e.key === "\\") { e.preventDefault(); setWorkspaceOpen((v) => { try { localStorage.setItem("cliche-workspace", v ? "off" : "on"); } catch { /* */ } return !v; }); return; }
      if (e.key === "Escape") { setBooting(false); setShowKeys(false); setPaletteOpen(false); setShowMemory(false); setShowSettings(false); return; }
      if (isTyping() || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "?") { e.preventDefault(); setShowKeys(true); }
      else if (e.key === "g") { leaderRef.current = true; setLeader(true); leaderTimer = window.setTimeout(() => { leaderRef.current = false; setLeader(false); }, 800); }
    };
    window.addEventListener("keydown", onKey);
    return () => { clearTimeout(t); window.clearTimeout(leaderTimer); window.removeEventListener("keydown", onKey); };
  }, []);
  function setAccentTheme(id: string) { setAccent(id); applyAccent(id); }
  const persistWs = (on: boolean) => { try { localStorage.setItem("cliche-workspace", on ? "on" : "off"); } catch { /* ignore */ } };
  const toggleWs = () => setWorkspaceOpen((v) => { persistWs(!v); return !v; });
  const showWs = () => { setWorkspaceOpen(true); persistWs(true); };
  function toggleInst(k: "substrate" | "sound" | "oracle") {
    const on = !inst[k];
    if (k === "substrate") { setFlag("cliche-substrate", on); document.documentElement.setAttribute("data-substrate", on ? "on" : "off"); }
    if (k === "oracle") setFlag("cliche-oracle", on);
    if (k === "sound") { if (on) { if (!enableAudio()) return; } else disableAudio(); setFlag("cliche-sound", on); }
    setInst((p) => ({ ...p, [k]: on }));
  }

  useEffect(() => {
    api("/api/state").then((r) => r.json()).then(setState).catch(() => {}).finally(() => setLoaded(true));
    api("/api/templates").then((r) => r.json()).then(setTemplates).catch(() => {});
    api("/api/session").then((r) => r.json()).then((d) => setItems(msgsToItems(d.messages || []))).catch(() => {});
    api("/api/models").then((r) => r.json()).then(setModels).catch(() => {});
    api("/api/commands").then((r) => r.json()).then(setCommands).catch(() => {});
    refreshSessions(); refreshAudit(); refreshFiles(); refreshChanges(); refreshRules(); refreshTasks();
    const es = new EventSource(sseUrl("/api/events"));
    es.onmessage = (m) => {
      const e: Ev = JSON.parse(m.data);
      setItems((prev) => reduce(prev, e));
      if (e.kind === "delta") activityRef.current = Math.min(1, activityRef.current + 0.06);
      if (e.kind === "tool_call" || e.kind === "tool_result") { activityRef.current = Math.min(1, activityRef.current + 0.2); tracerRef.current(e.kind); }
      if (e.kind === "tool_result") depthRef.current("tool_result");
      if (e.kind === "error") tracerRef.current("error");
      if (e.kind === "approval") setAwaiting(true);
      if (e.kind === "end" || e.kind === "error") setAwaiting(false);
      if (e.kind === "delta" || e.kind === "tool_call" || e.kind === "tool_result" || e.kind === "approval" || e.kind === "error" || e.kind === "end") ping(e.kind);
      if (e.kind === "state" && e.data) setState(e.data);
      if (e.kind === "end") {
        notifyDone();
        setPreviewKey((k) => k + 1); refreshSessions(); refreshFiles(); refreshChanges(); refreshTasks();
        api("/api/audit").then((r) => r.json()).then((fresh) => {
          setAudit(fresh);
          if (fresh.ok && lastCelebRef.current > 0 && fresh.entries > lastCelebRef.current && !REDUCE.matches) setCelebrate(true);
          lastCelebRef.current = fresh.entries || 0;
        }).catch(() => {});
      }
    };
    return () => es.close();
  }, []);

  // activity spine: decay the signal each frame into a CSS var (no React re-render)
  useEffect(() => {
    let raf = 0;
    const tick = () => { activityRef.current *= 0.94; document.documentElement.style.setProperty("--activity", activityRef.current.toFixed(3)); scoreActivity(activityRef.current); raf = requestAnimationFrame(tick); };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, []);
  useEffect(() => { document.body.classList.toggle("awaiting", awaiting); }, [awaiting]);
  useEffect(() => { document.documentElement.setAttribute("data-sigtrust", trustOf(audit)); }, [audit]);
  useEffect(() => {
    let q = 0;
    const onMove = (e: PointerEvent) => {
      if (q) return;
      q = requestAnimationFrame(() => { q = 0; const dx = Math.max(-1, Math.min(1, (e.clientX / innerWidth - 0.5) * 2)), dy = Math.max(-1, Math.min(1, (e.clientY / innerHeight - 0.5) * 2)); const r = document.documentElement.style; r.setProperty("--eye-x", `${dx * 2}px`); r.setProperty("--eye-y", `${dy * 2}px`); });
    };
    window.addEventListener("pointermove", onMove);
    return () => window.removeEventListener("pointermove", onMove);
  }, []);
  useEffect(() => {
    if (!state.running) { setRitual(false); return; }
    const t = setTimeout(() => setRitual(true), 400);
    return () => clearTimeout(t);
  }, [state.running]);
  useEffect(() => {
    const c = tabBarRef.current; if (!c) return;
    const el = c.querySelector('[data-on="true"]') as HTMLElement | null;
    if (el) { const cb = c.getBoundingClientRect(), bb = el.getBoundingClientRect(); setPill({ left: bb.left - cb.left, width: bb.width }); }
  }, [tab, changes.length]);

  function answer(id: string, allow: boolean) {
    api("/api/approve", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, allow }) });
    setItems((prev) => prev.map((it) => (it.t === "approval" && it.id === id ? { ...it, answered: allow ? "allowed" : "declined" } : it)));
  }
  async function run(p: string) {
    if (!p.trim() || state.running) return;
    setMobileView("chat");
    setItems((prev) => [...prev, { t: "you", text: p }]); setPrompt("");
    const r = await api("/api/prompt", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt: p }) });
    if (!r.ok) setItems((prev) => [...prev, { t: "error", text: r.status === 409 ? "a run is already in progress" : `request failed (${r.status})` }]);
    else setImgCount(0);
  }
  async function addTask(title: string) { const r = await api("/api/tasks", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ title }) }); setTasks(await r.json()); }
  async function toggleTask(id: number) { const r = await api("/api/tasks/done", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) }); setTasks(await r.json()); }
  async function clearTasks() { const r = await api("/api/tasks/clear", { method: "POST" }); setTasks(await r.json()); }
  async function uploadImage(file: File) { const fd = new FormData(); fd.append("file", file); const r = await api("/api/image", { method: "POST", body: fd }); if (r.ok) setImgCount((await r.json()).count); }
  async function uploadImages(files: File[]) { for (const f of files) { if (f.type.startsWith("image/")) await uploadImage(f); } }
  function onComposerPaste(e: React.ClipboardEvent) { const imgs = Array.from(e.clipboardData.files).filter((f) => f.type.startsWith("image/")); if (imgs.length) { e.preventDefault(); uploadImages(imgs); } }
  function onComposerDrop(e: React.DragEvent) { e.preventDefault(); setDragOver(false); const imgs = Array.from(e.dataTransfer.files).filter((f) => f.type.startsWith("image/")); if (imgs.length) uploadImages(imgs); }
  // Voice input: dictate the prompt via the browser Web Speech API (no deps; a
  // no-op where unsupported). Interim results stream into the composer.
  function toggleDictation() {
    if (!speechOK) return;
    if (listening) { recogRef.current?.stop(); return; }
    const SR = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    const r = new SR();
    r.lang = navigator.language || "en-US";
    r.interimResults = true;
    r.continuous = false;
    const base = prompt ? prompt.replace(/\s*$/, "") + " " : "";
    r.onresult = (e: any) => {
      let txt = "";
      for (let i = e.resultIndex; i < e.results.length; i++) txt += e.results[i][0].transcript;
      setPrompt(base + txt);
    };
    r.onend = () => setListening(false);
    r.onerror = () => setListening(false);
    recogRef.current = r;
    setListening(true);
    r.start();
  }
  async function newChat() {
    await api("/api/sessions/new", { method: "POST" });
    setItems([]); refreshSessions(); setMobileView("chat");
  }
  async function pickSession(id: string) {
    const r = await api("/api/sessions/select", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) });
    const d = await r.json(); setItems(msgsToItems(d.messages || [])); refreshSessions(); setMobileView("chat");
  }
  async function renameSession(id: string, title: string) {
    await api("/api/sessions/rename", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, title }) });
    refreshSessions();
  }
  async function deleteSession(id: string) {
    const r = await api("/api/sessions/delete", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) });
    if (r.ok) { const wasActive = sessions.find((s) => s.id === id)?.active; if (wasActive) setItems([]); refreshSessions(); }
  }
  const refreshState = () => api("/api/state").then((r) => r.json()).then(setState).catch(() => {});
  async function setMode(mode: string) {
    await api("/api/mode", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ mode }) });
    refreshState(); refreshRules();
  }
  async function undo() { await api("/api/undo", { method: "POST" }); refreshChanges(); refreshFiles(); setPreviewKey((k) => k + 1); }
  async function rewind() { await api("/api/rewind", { method: "POST" }); refreshChanges(); refreshFiles(); setPreviewKey((k) => k + 1); }
  async function setModel(model: string) {
    await api("/api/model", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ model }) });
    refreshState();
  }
  function stop() { api("/api/stop", { method: "POST" }); }

  function runCommand(line: string): boolean {
    const [cmd, ...rest] = line.slice(1).split(/\s+/);
    const arg = rest.join(" ").trim();
    switch (cmd) {
      case "new": case "clear": newChat(); return true;
      case "stop": stop(); return true;
      case "plan": if (arg) { addTask(arg); return true; } return false;
      case "tasks": return true;
      case "done": { const id = parseInt(arg, 10); if (!isNaN(id)) { toggleTask(id); return true; } return false; }
      case "image": fileRef.current?.click(); return true;
      case "suggest": case "full": setMode(cmd); return true;
      case "auto": case "auto-edit": setMode("auto-edit"); return true;
      case "mode": if (arg) { setMode(arg === "auto" ? "auto-edit" : arg); return true; } return false;
      case "model": if (arg) { setModel(arg); return true; } return false;
      case "undo": undo(); return true;
      case "rewind": rewind(); return true;
      case "changes": case "diff": showWs(); setTab("changes"); return true;
      case "git": case "commit": case "pr": showWs(); setTab("git"); return true;
      case "preview": showWs(); setTab("preview"); return true;
      case "files": showWs(); setTab("files"); return true;
      case "trust": case "rules": case "status": showWs(); setTab("trust"); return true;
      case "focus": case "chat": setWorkspaceOpen(false); persistWs(false); return true;
      case "split": case "panel": showWs(); return true;
      case "export": window.location.href = sseUrl("/api/export"); return true;
      default: return false;
    }
  }
  function submit() {
    const p = prompt.trim();
    if (p.startsWith("/") && runCommand(p)) { setPrompt(""); return; }
    run(p);
  }

  async function openFileAt(path: string) {
    const r = await api(`/api/file?path=${encodeURIComponent(path)}`);
    if (!r.ok) return;
    const text = await r.text();
    setOpenFile({ path, html: hljs.highlightAuto(text).value });
    setTab("files");
  }
  function toggleDir(p: string) {
    setExpanded((prev) => { const n = new Set(prev); if (n.has(p)) n.delete(p); else n.add(p); return n; });
  }

  if (isApp() && !apiBase()) return <Connect onConnected={() => location.reload()} />;
  if (!loaded) return <Boot />;
  if (state.needs_setup) return <Setup onDone={() => api("/api/state").then((r) => r.json()).then(setState)} />;

  const activeTitle = sessions.find((s) => s.active)?.title;
  const allCommands = [...COMMANDS, ...commands.map((c) => ({ cmd: c.name, desc: c.desc, arg: true }))];
  const tabs: { id: typeof tab; label: string; icon: any }[] = [
    { id: "preview", label: "Preview", icon: Eye },
    { id: "files", label: "Files", icon: ListTree },
    { id: "changes", label: changes.length ? `Changes · ${changes.length}` : "Changes", icon: FileDiff },
    { id: "git", label: "Git", icon: GitBranch },
    { id: "schedule", label: "Scheduled", icon: Clock },
    { id: "trust", label: "Trust", icon: ShieldCheck },
  ];
  const palette = allCommands.filter((c) => ("/" + c.cmd).startsWith(prompt.split(/\s+/)[0])).slice(0, 8);
  // @-file mention autocomplete: the token under the caret (at the end of the prompt).
  const atTok = (() => { const m = /(?:^|\s)@([^\s@]*)$/.exec(prompt); return m ? m[1] : null; })();
  const mentions = atTok === null ? [] : flattenTree(tree)
    .map((p) => ({ p, s: fuzzy(atTok, p) })).filter((x) => x.s !== null)
    .sort((a, b) => (b.s as number) - (a.s as number)).slice(0, 7).map((x) => x.p);
  const ctxPct = Math.round((state.ctx_frac || 0) * 100);
  const paletteItems: PItem[] = [
    ...allCommands.map((c) => ({ id: "cmd-" + c.cmd, group: "Cmd", label: "/" + c.cmd + " — " + c.desc, run: () => { if (c.arg) setPrompt("/" + c.cmd + " "); else runCommand("/" + c.cmd); } })),
    ...sessions.map((s) => ({ id: "sess-" + s.id, group: "Chat", label: s.title || "New chat", hint: relTime(s.updated), run: () => pickSession(s.id) })),
    ...tabs.map((t) => ({ id: "tab-" + t.id, group: "View", label: String(t.label).replace(/ ·.*/, ""), run: () => setTab(t.id) })),
    { id: "mem", group: "Go", label: "Memory — fly through past sessions", run: () => setShowMemory(true) },
    { id: "settings", group: "Go", label: "Settings — switch provider / model / key", run: () => setShowSettings(true) },
    { id: "i-sub", group: "Live", label: `Living substrate — ${inst.substrate ? "on" : "off"}`, run: () => toggleInst("substrate") },
    { id: "i-snd", group: "Live", label: `Sound — the agent's score — ${inst.sound ? "on" : "off"}`, run: () => toggleInst("sound") },
    { id: "i-ora", group: "Live", label: `Oracle — the Sigil speaks — ${inst.oracle ? "on" : "off"}`, run: () => toggleInst("oracle") },
    ...ACCENTS.map((ac) => ({ id: "acc-" + ac.id, group: "Theme", label: "Accent · " + ac.id, run: () => setAccentTheme(ac.id) })),
  ];

  const lastActText = (() => { for (let i = items.length - 1; i >= 0; i--) { const it = items[i]; if (it.t === "tool" || it.t === "result") return it.text; } return ""; })();

  return (
    <>
      {inst.substrate && <ShaderField />}
      <DepthField bind={(fn) => { depthRef.current = fn; }} />
      <div className="aurora" />
      <div className="aurora-pulse" />
      <TracerField bind={(fn) => { tracerRef.current = fn; }} />
      <div className="grain" />
      {booting && <Boot />}
      {celebrate && <Sparks origin={{ x: 32, y: 26 }} onDone={() => setCelebrate(false)} />}
      {showMemory && <MemoryConstellation sessions={sessions} relTime={relTime} onPick={pickSession} onClose={() => setShowMemory(false)} />}
      {showSettings && <Settings state={state} onClose={() => setShowSettings(false)} onApplied={() => { refreshState(); api("/api/models").then((r) => r.json()).then(setModels).catch(() => {}); }} />}
      {paletteOpen && <CommandPalette items={paletteItems} onClose={() => setPaletteOpen(false)} />}
      {showKeys && <KeysOverlay onClose={() => setShowKeys(false)} />}
      {leader && <div className="fade-up glass elev fixed bottom-6 left-1/2 z-[90] flex -translate-x-1/2 items-center gap-2 rounded-xl px-3 py-2 text-xs text-[var(--mut)]">go to <span className="kbd">p</span><span className="kbd">f</span><span className="kbd">c</span><span className="kbd">t</span><span className="kbd">n</span></div>}
      <div className="relative flex h-full flex-col">
        {state.running && <div className="loadbar absolute inset-x-0 top-0 z-50" />}
        <div className="flex min-h-0 flex-1">
        <Sidebar sessions={sessions} state={state} audit={audit} tasks={tasks} accent={accent} inst={inst} isMobile={isMobile} mobileShow={mobileView === "sessions"} onNew={newChat} onPick={pickSession} onRename={renameSession} onDelete={deleteSession} onToggleTask={toggleTask} onClearTasks={clearTasks} onAccent={setAccentTheme} onSearch={() => setPaletteOpen(true)} onSettings={() => setShowSettings(true)} onToggleInst={toggleInst} onMemory={() => setShowMemory(true)} />

      {/* conversation */}
      <section className={`cl-chat flex min-w-0 flex-1 flex-col ${isMobile && mobileView !== "chat" ? "hidden" : ""}`}>
        <header className="glass flex h-[52px] items-center gap-2 border-b border-[var(--line)] px-5">
          <span className="min-w-0 truncate text-sm font-medium">{activeTitle || "New chat"}</span>
          {state.running && <span className="flex items-center gap-2 text-xs text-[var(--accent)]"><span className="orb" /> working</span>}
          <span className="flex-1" />
          {state.running && (
            <button onClick={stop} className="flex items-center gap-1.5 rounded-lg border border-[var(--accent)]/40 bg-[var(--accent)]/[0.08] px-2.5 py-1 text-xs text-[var(--accent)] transition-colors hover:bg-[var(--accent)]/[0.16]" title="Stop the run">
              <Square size={11} strokeWidth={3} /> Stop
            </button>
          )}
          <div className="seg max-md:hidden">
            {MODES.map((m) => (
              <button key={m.id} data-on={(state.mode || "suggest") === m.id} onClick={() => setMode(m.id)} className="seg-item" title={`Permission mode: ${m.label}`}>{m.label}</button>
            ))}
          </div>
          {ctxPct > 0 && (
            <span className="chip tabular-nums max-md:hidden" title="Context window used" style={{ color: ctxPct < 60 ? "var(--ok)" : ctxPct < 85 ? "var(--warn)" : "var(--accent)" }}>
              {ctxPct}% ctx
            </span>
          )}
          <select value={state.model || ""} onChange={(e) => setModel(e.target.value)} title="Model"
            className="field max-w-[170px] px-2.5 py-1.5 font-mono text-xs text-[var(--mut)] outline-none max-md:max-w-[120px]">
            {state.model && !models.some((m) => m.model === state.model) && <option value={state.model}>{state.model}</option>}
            {models.map((m) => <option key={m.model} value={m.model}>{m.model}</option>)}
          </select>
          <button onClick={toggleWs} className={`icon-btn h-8 w-8 max-md:hidden ${workspaceOpen ? "text-[var(--accent)]" : ""}`} title={workspaceOpen ? "Hide the panel — chat only (⌘\\)" : "Show the panel (⌘\\)"}>
            <PanelRight size={16} />
          </button>
        </header>

        <div ref={feedRef} className="flex-1 overflow-auto">
          {items.length === 0 ? <Welcome templates={templates} onPick={run} /> : (
            <div className="mx-auto max-w-3xl px-6 py-7 text-[13.5px] leading-relaxed">
              {items.map((it, i) => <Row key={i} it={it} onAnswer={answer} />)}
            </div>
          )}
        </div>

        <div className="px-5 pb-5">
          <div className="mx-auto max-w-3xl">
            {prompt.startsWith("/") && palette.length > 0 && (
              <div className="surface elev slide-down mb-2 overflow-hidden rounded-xl">
                {palette.map((c, i) => (
                  <button key={c.cmd} type="button"
                    onClick={() => { if (c.arg) setPrompt("/" + c.cmd + " "); else { runCommand("/" + c.cmd); setPrompt(""); } }}
                    className={`flex w-full items-center gap-3 px-3 py-2 text-left text-[13px] ${i === 0 ? "bg-white/[0.04]" : ""} hover:bg-white/[0.06]`}>
                    <span className="w-28 shrink-0 font-mono text-[var(--accent)]">/{c.cmd}</span>
                    <span className="text-[var(--mut)]">{c.desc}</span>
                  </button>
                ))}
              </div>
            )}
            {mentions.length > 0 && (
              <div className="surface elev slide-down mb-2 overflow-hidden rounded-xl">
                {mentions.map((path, i) => (
                  <button key={path} type="button"
                    onClick={() => setPrompt(prompt.replace(/@([^\s@]*)$/, "@" + path + " "))}
                    className={`flex w-full items-center gap-2 px-3 py-2 text-left text-[13px] ${i === 0 ? "bg-white/[0.04]" : ""} hover:bg-white/[0.06]`}>
                    <AtSign size={12} className="shrink-0 text-[var(--accent)]" /><span className="truncate font-mono text-[var(--mut)]">{path}</span>
                  </button>
                ))}
              </div>
            )}
            <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadImage(f); e.target.value = ""; }} />
            <form onSubmit={(e) => { e.preventDefault(); submit(); }}
              onDrop={onComposerDrop} onDragOver={(e) => { e.preventDefault(); setDragOver(true); }} onDragLeave={() => setDragOver(false)}
              className={`surface field flex items-center gap-2 rounded-2xl p-2 pl-2.5 shadow-[var(--sh-md)] transition-shadow ${dragOver ? "ring-2 ring-[var(--accent)]/70" : ""}`}>
              <button type="button" onClick={() => fileRef.current?.click()} className="icon-btn h-9 w-9 shrink-0" title="Attach an image — or paste / drop one"><ImagePlus size={18} /></button>
              {speechOK && <button type="button" onClick={toggleDictation} className={`icon-btn h-9 w-9 shrink-0 ${listening ? "text-[var(--accent)]" : ""}`} title={listening ? "Stop dictation" : "Dictate (voice)"}><Mic size={18} className={listening ? "pulse-soft" : ""} /></button>}
              {imgCount > 0 && (
                <span className="flex shrink-0 items-center gap-1 rounded-full bg-[var(--accent)]/15 px-2 py-1 text-xs text-[var(--accent)]">
                  {imgCount} image{imgCount > 1 ? "s" : ""}
                  <button type="button" onClick={() => { api("/api/image/clear", { method: "POST" }); setImgCount(0); }} title="Remove"><X size={11} /></button>
                </span>
              )}
              <input value={prompt} onChange={(e) => setPrompt(e.target.value)} onPaste={onComposerPaste} placeholder={dragOver ? "Drop image to attach…" : "Describe what you want to build…"} autoFocus className="flex-1 bg-transparent py-2.5 text-[15px] outline-none placeholder:text-[var(--dim)]" />
              <button disabled={(state.running && !prompt.startsWith("/")) || !prompt.trim()} className="btn-accent grid h-9 w-9 place-items-center rounded-xl" title="Send"><ArrowUp size={18} /></button>
            </form>
            <div className="mt-2 flex items-center gap-3 px-1 text-[11px] text-[var(--faint)]">
              <span><span className="kbd">/</span> commands</span>
              <span><span className="kbd">@</span> files</span>
              <span className="flex-1" />
              <span className="flex items-center gap-1"><CornerDownLeft size={11} /> send</span>
            </div>
          </div>
        </div>
      </section>

      {/* workspace */}
      {(workspaceOpen || isMobile) && (
      <aside className={`cl-workspace flex flex-col border-l border-[var(--line)] ${isMobile ? (mobileView === "work" ? "w-full" : "hidden") : "w-[42%] min-w-[360px]"}`}>
        <div className="flex h-[52px] items-center gap-2 border-b border-[var(--line)] px-3">
          <div ref={tabBarRef} className="seg seg-morph">
            <span className="seg-pill" style={{ transform: `translateX(${pill.left}px)`, width: pill.width }} />
            {tabs.map((t) => (
              <button key={t.id} data-on={tab === t.id} onClick={() => setTab(t.id)} className="seg-item"><t.icon size={13} /> {t.label}</button>
            ))}
          </div>
          <span className="flex-1" />
          {tab === "preview" && (
            <>
              <a href={sseUrl("/api/export")} className="icon-btn h-8 w-8" title="Download (.zip)"><Download size={15} /></a>
              <button onClick={() => setPreviewKey((k) => k + 1)} className="icon-btn h-8 w-8" title="Refresh"><RefreshCw size={15} /></button>
              <a href="/preview/" target="_blank" className="icon-btn h-8 w-8" title="Open in a tab"><ExternalLink size={15} /></a>
            </>
          )}
        </div>

        {tab === "preview" && (
          <div className="min-h-0 flex-1 p-3">
            <div className="surface elev relative flex h-full flex-col overflow-hidden rounded-2xl">
              <div className="flex h-9 items-center gap-2 border-b border-[var(--line)] px-3.5">
                <span className="flex gap-1.5"><i className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" /><i className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" /><i className="h-2.5 w-2.5 rounded-full bg-[#28c840]" /></span>
                <span className="mx-2 flex-1 truncate rounded-md bg-black/30 px-2.5 py-1 text-center text-[11px] text-[var(--dim)]">localhost preview</span>
              </div>
              <iframe key={previewKey} src={`/preview/?k=${previewKey}`} title="preview" className="flex-1 border-0 bg-white" sandbox="allow-scripts allow-forms allow-same-origin" />
              {ritual && (
                <div className="fade-in pointer-events-none absolute inset-0 z-20 grid place-items-center" style={{ background: "rgba(8,8,11,.6)", backdropFilter: "blur(7px)", WebkitBackdropFilter: "blur(7px)" }}>
                  <Kiln />
                  <div className="relative z-10 text-center">
                    <div className="mx-auto mb-4 w-[88px]"><Sigil size={88} running trust="intact" load={1} /></div>
                    <div className="caret text-sm font-medium text-[var(--ink)]">forging</div>
                    {lastActText && <div className="mx-auto mt-2 max-w-[300px] truncate px-4 font-mono text-[12px] text-[var(--mut)]">{lastActText}</div>}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {tab === "files" && (
          <div className="flex min-h-0 flex-1">
            <div className="w-[46%] overflow-auto border-r border-[var(--line)] p-2">
              {tree.length === 0 ? <div className="p-3 text-xs text-[var(--dim)]">No files yet</div> :
                <Tree nodes={tree} depth={0} expanded={expanded} onToggle={toggleDir} onOpen={openFileAt} active={openFile?.path || ""} />}
            </div>
            <div className="min-w-0 flex-1 overflow-auto">
              {openFile ? (
                <>
                  <div className="glass sticky top-0 z-10 flex items-center gap-2 border-b border-[var(--line)] px-4 py-2 font-mono text-[11px] text-[var(--mut)]"><FileText size={12} className="text-[var(--dim)]" />{openFile.path}</div>
                  <pre className="hljs !bg-transparent p-4 text-[12.5px] leading-relaxed"><code dangerouslySetInnerHTML={{ __html: openFile.html }} /></pre>
                </>
              ) : <div className="grid h-full place-items-center p-6 text-sm text-[var(--dim)]"><span><FileText size={26} className="mx-auto mb-2 opacity-50" />Select a file to view it</span></div>}
            </div>
          </div>
        )}

        {tab === "changes" && <ChangesPanel changes={changes} onUndo={undo} onRevertAll={rewind} />}
        {tab === "git" && <GitPanel onAsk={run} onChanged={() => { refreshChanges(); refreshFiles(); }} />}
        {tab === "schedule" && <ScheduledPanel onRun={(p) => { setMobileView("chat"); run(p); }} />}
        {tab === "trust" && <TrustPanel a={audit} rules={rules} />}
      </aside>
      )}
        </div>
        {isMobile && <MobileTabBar view={mobileView} onView={setMobileView} />}
      </div>
    </>
  );
}
