import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import hljs from "highlight.js";
import {
  ShieldCheck, ShieldAlert, Download, RefreshCw, ExternalLink, ArrowUp, Sparkles,
  Check, Wrench, Globe, Wand2, Hammer, FileSearch, KeyRound, CircleAlert, Plus,
  MessageSquare, Folder, FolderOpen, FileText, Eye, ListTree, ChevronRight, Square,
  FileDiff, ImagePlus, X, CornerDownLeft, Trash2,
} from "lucide-react";

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
  { cmd: "preview", desc: "Show the live preview" },
  { cmd: "files", desc: "Show the file tree" },
  { cmd: "trust", desc: "Show the trust ledger + rules" },
  { cmd: "export", desc: "Download the project (.zip)" },
];
type Item =
  | { t: "you"; text: string }
  | { t: "assistant"; text: string }
  | { t: "tool"; text: string }
  | { t: "result"; text: string }
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

function Mark({ size = 28 }: { size?: number }) {
  return (
    <span className="grid shrink-0 place-items-center rounded-[10px] btn-accent" style={{ width: size, height: size }}>
      <Sparkles size={size * 0.55} />
    </span>
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
    case "tool_result": return [...prev, { t: "result", text: e.text || "" }];
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
    const r = await fetch("/api/setup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ provider, key }) });
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

function Sidebar({ sessions, state, audit, tasks, onNew, onPick, onToggleTask, onClearTasks }: {
  sessions: SessionMeta[]; state: State; audit: Audit | null; tasks: Task[];
  onNew: () => void; onPick: (id: string) => void; onToggleTask: (id: number) => void; onClearTasks: () => void;
}) {
  const cap = state.cap_usd || 0;
  const doneCount = tasks.filter((t) => t.done).length;
  return (
    <aside className="flex w-[244px] shrink-0 flex-col border-r border-[var(--line)]">
      <div className="flex h-[52px] items-center gap-2.5 px-4">
        <Mark size={26} />
        <span className="text-[15px] font-semibold tracking-tight">Cliché <span className="text-[var(--dim)]">Studio</span></span>
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
          <button key={s.id} onClick={() => onPick(s.id)} title={s.title}
            className={`group relative mb-0.5 flex w-full items-center gap-2.5 rounded-lg py-2 pl-3 pr-2 text-left text-[13px] transition-colors ${s.active ? "bg-white/[0.06] text-[var(--ink)]" : "text-[var(--mut)] hover:bg-white/[0.035]"}`}>
            {s.active && <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-[var(--accent)]" />}
            <MessageSquare size={14} className={s.active ? "shrink-0 text-[var(--accent)]" : "shrink-0 text-[var(--dim)]"} />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">{s.title || "New chat"}</span>
              <span className="block text-[11px] text-[var(--dim)]">{relTime(s.updated)} · {s.messages} msgs</span>
            </span>
          </button>
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
            <div className="mb-2 flex items-center gap-1.5 text-xs">
              {audit.ok ? <span className="flex items-center gap-1 text-[var(--ok)]"><ShieldCheck size={13} /> verified</span>
                        : <span className="flex items-center gap-1 text-[var(--accent)]"><ShieldAlert size={13} /> tamper</span>}
              <span className="text-[var(--dim)]">· {audit.entries} receipts</span>
            </div>
          )}
          <div className="mb-1.5 flex items-center justify-between text-xs text-[var(--mut)]">
            <span className="truncate font-mono text-[11px]">{state.model || "—"}</span>
            <span className="font-mono tabular-nums">${(state.spent_usd || 0).toFixed(3)}</span>
          </div>
          {cap > 0 && <Gauge frac={(state.spent_usd || 0) / cap} />}
        </div>
      </div>
    </aside>
  );
}

function ApprovalCard({ it, onAnswer }: { it: Extract<Item, { t: "approval" }>; onAnswer: (id: string, allow: boolean) => void }) {
  return (
    <div className="fade-up my-3 overflow-hidden rounded-2xl border border-[var(--accent)]/35 bg-[var(--accent)]/[0.06]">
      <div className="flex items-center gap-2 px-4 py-3 text-sm">
        <span className="grid h-7 w-7 place-items-center rounded-lg bg-[var(--accent)]/15 text-[var(--accent)]"><CircleAlert size={15} /></span>
        <span>Cliche wants to <b className="text-[var(--accent)]">{it.kind}</b></span>
        <code className="ml-auto truncate rounded-md bg-black/30 px-2 py-0.5 font-mono text-[12.5px]">{it.target}</code>
      </div>
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
    case "assistant": return <div className="md fade-up my-3"><ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>{it.text}</ReactMarkdown></div>;
    case "tool": return <div className="fade-up my-1 flex items-center gap-2 text-[12.5px] text-[var(--mut)]"><Wrench size={12} className="text-[var(--accent)]" /> <span className="font-mono">{it.text}</span></div>;
    case "result": return <div className="fade-up my-1 flex items-center gap-2 text-[12.5px] text-[var(--mut)]"><Check size={12} className="text-[var(--ok)]" strokeWidth={3} /> <span className="font-mono">{it.text}</span></div>;
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
  const [previewKey, setPreviewKey] = useState(0);
  const [tab, setTab] = useState<"preview" | "files" | "changes" | "trust">("preview");
  const [tree, setTree] = useState<FileNode[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [openFile, setOpenFile] = useState<{ path: string; html: string } | null>(null);
  const feedRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const refreshAudit = () => fetch("/api/audit").then((r) => r.json()).then(setAudit).catch(() => {});
  const refreshSessions = () => fetch("/api/sessions").then((r) => r.json()).then(setSessions).catch(() => {});
  const refreshFiles = () => fetch("/api/files").then((r) => r.json()).then(setTree).catch(() => {});
  const refreshChanges = () => fetch("/api/changes").then((r) => r.json()).then(setChanges).catch(() => {});
  const refreshRules = () => fetch("/api/rules").then((r) => r.json()).then(setRules).catch(() => {});
  const refreshTasks = () => fetch("/api/tasks").then((r) => r.json()).then(setTasks).catch(() => {});
  useEffect(() => { feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight, behavior: "smooth" }); }, [items]);

  useEffect(() => {
    fetch("/api/state").then((r) => r.json()).then(setState).catch(() => {});
    fetch("/api/templates").then((r) => r.json()).then(setTemplates).catch(() => {});
    fetch("/api/session").then((r) => r.json()).then((d) => setItems(msgsToItems(d.messages || []))).catch(() => {});
    fetch("/api/models").then((r) => r.json()).then(setModels).catch(() => {});
    fetch("/api/commands").then((r) => r.json()).then(setCommands).catch(() => {});
    refreshSessions(); refreshAudit(); refreshFiles(); refreshChanges(); refreshRules(); refreshTasks();
    const es = new EventSource("/api/events");
    es.onmessage = (m) => {
      const e: Ev = JSON.parse(m.data);
      setItems((prev) => reduce(prev, e));
      if (e.kind === "state" && e.data) setState(e.data);
      if (e.kind === "end") { setPreviewKey((k) => k + 1); refreshAudit(); refreshSessions(); refreshFiles(); refreshChanges(); refreshTasks(); }
    };
    return () => es.close();
  }, []);

  function answer(id: string, allow: boolean) {
    fetch("/api/approve", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, allow }) });
    setItems((prev) => prev.map((it) => (it.t === "approval" && it.id === id ? { ...it, answered: allow ? "allowed" : "declined" } : it)));
  }
  async function run(p: string) {
    if (!p.trim() || state.running) return;
    setItems((prev) => [...prev, { t: "you", text: p }]); setPrompt("");
    const r = await fetch("/api/prompt", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt: p }) });
    if (!r.ok) setItems((prev) => [...prev, { t: "error", text: r.status === 409 ? "a run is already in progress" : `request failed (${r.status})` }]);
    else setImgCount(0);
  }
  async function addTask(title: string) { const r = await fetch("/api/tasks", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ title }) }); setTasks(await r.json()); }
  async function toggleTask(id: number) { const r = await fetch("/api/tasks/done", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) }); setTasks(await r.json()); }
  async function clearTasks() { const r = await fetch("/api/tasks/clear", { method: "POST" }); setTasks(await r.json()); }
  async function uploadImage(file: File) { const fd = new FormData(); fd.append("file", file); const r = await fetch("/api/image", { method: "POST", body: fd }); if (r.ok) setImgCount((await r.json()).count); }
  async function newChat() {
    await fetch("/api/sessions/new", { method: "POST" });
    setItems([]); refreshSessions();
  }
  async function pickSession(id: string) {
    const r = await fetch("/api/sessions/select", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) });
    const d = await r.json(); setItems(msgsToItems(d.messages || [])); refreshSessions();
  }
  const refreshState = () => fetch("/api/state").then((r) => r.json()).then(setState).catch(() => {});
  async function setMode(mode: string) {
    await fetch("/api/mode", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ mode }) });
    refreshState(); refreshRules();
  }
  async function undo() { await fetch("/api/undo", { method: "POST" }); refreshChanges(); refreshFiles(); setPreviewKey((k) => k + 1); }
  async function rewind() { await fetch("/api/rewind", { method: "POST" }); refreshChanges(); refreshFiles(); setPreviewKey((k) => k + 1); }
  async function setModel(model: string) {
    await fetch("/api/model", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ model }) });
    refreshState();
  }
  function stop() { fetch("/api/stop", { method: "POST" }); }

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
      case "changes": case "diff": setTab("changes"); return true;
      case "preview": setTab("preview"); return true;
      case "files": setTab("files"); return true;
      case "trust": case "rules": case "status": setTab("trust"); return true;
      case "export": window.location.href = "/api/export"; return true;
      default: return false;
    }
  }
  function submit() {
    const p = prompt.trim();
    if (p.startsWith("/") && runCommand(p)) { setPrompt(""); return; }
    run(p);
  }

  async function openFileAt(path: string) {
    const r = await fetch(`/api/file?path=${encodeURIComponent(path)}`);
    if (!r.ok) return;
    const text = await r.text();
    setOpenFile({ path, html: hljs.highlightAuto(text).value });
    setTab("files");
  }
  function toggleDir(p: string) {
    setExpanded((prev) => { const n = new Set(prev); if (n.has(p)) n.delete(p); else n.add(p); return n; });
  }

  if (state.needs_setup) return <Setup onDone={() => fetch("/api/state").then((r) => r.json()).then(setState)} />;

  const activeTitle = sessions.find((s) => s.active)?.title;
  const allCommands = [...COMMANDS, ...commands.map((c) => ({ cmd: c.name, desc: c.desc, arg: true }))];
  const tabs: { id: typeof tab; label: string; icon: any }[] = [
    { id: "preview", label: "Preview", icon: Eye },
    { id: "files", label: "Files", icon: ListTree },
    { id: "changes", label: changes.length ? `Changes · ${changes.length}` : "Changes", icon: FileDiff },
    { id: "trust", label: "Trust", icon: ShieldCheck },
  ];
  const palette = allCommands.filter((c) => ("/" + c.cmd).startsWith(prompt.split(/\s+/)[0])).slice(0, 8);

  return (
    <div className="relative flex h-full">
      {state.running && <div className="loadbar absolute inset-x-0 top-0 z-50" />}
      <Sidebar sessions={sessions} state={state} audit={audit} tasks={tasks} onNew={newChat} onPick={pickSession} onToggleTask={toggleTask} onClearTasks={clearTasks} />

      {/* conversation */}
      <section className="flex min-w-0 flex-1 flex-col">
        <header className="glass flex h-[52px] items-center gap-2 border-b border-[var(--line)] px-5">
          <span className="min-w-0 truncate text-sm font-medium">{activeTitle || "New chat"}</span>
          {state.running && <span className="pulse-soft flex items-center gap-1 text-xs text-[var(--accent)]"><Sparkles size={13} /> working</span>}
          <span className="flex-1" />
          {state.running && (
            <button onClick={stop} className="flex items-center gap-1.5 rounded-lg border border-[var(--accent)]/40 bg-[var(--accent)]/[0.08] px-2.5 py-1 text-xs text-[var(--accent)] transition-colors hover:bg-[var(--accent)]/[0.16]" title="Stop the run">
              <Square size={11} strokeWidth={3} /> Stop
            </button>
          )}
          <div className="seg">
            {MODES.map((m) => (
              <button key={m.id} data-on={(state.mode || "suggest") === m.id} onClick={() => setMode(m.id)} className="seg-item" title={`Permission mode: ${m.label}`}>{m.label}</button>
            ))}
          </div>
          <select value={state.model || ""} onChange={(e) => setModel(e.target.value)} title="Model"
            className="field max-w-[170px] px-2.5 py-1.5 font-mono text-xs text-[var(--mut)] outline-none">
            {state.model && !models.some((m) => m.model === state.model) && <option value={state.model}>{state.model}</option>}
            {models.map((m) => <option key={m.model} value={m.model}>{m.model}</option>)}
          </select>
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
            <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadImage(f); e.target.value = ""; }} />
            <form onSubmit={(e) => { e.preventDefault(); submit(); }} className="surface field flex items-center gap-2 rounded-2xl p-2 pl-2.5 shadow-[var(--sh-md)]">
              <button type="button" onClick={() => fileRef.current?.click()} className="icon-btn h-9 w-9 shrink-0" title="Attach an image"><ImagePlus size={18} /></button>
              {imgCount > 0 && (
                <span className="flex shrink-0 items-center gap-1 rounded-full bg-[var(--accent)]/15 px-2 py-1 text-xs text-[var(--accent)]">
                  {imgCount} image{imgCount > 1 ? "s" : ""}
                  <button type="button" onClick={() => { fetch("/api/image/clear", { method: "POST" }); setImgCount(0); }} title="Remove"><X size={11} /></button>
                </span>
              )}
              <input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="Describe what you want to build…" autoFocus className="flex-1 bg-transparent py-2.5 text-[15px] outline-none placeholder:text-[var(--dim)]" />
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
      <aside className="flex w-[42%] min-w-[360px] flex-col border-l border-[var(--line)]">
        <div className="flex h-[52px] items-center gap-2 border-b border-[var(--line)] px-3">
          <div className="seg">
            {tabs.map((t) => (
              <button key={t.id} data-on={tab === t.id} onClick={() => setTab(t.id)} className="seg-item"><t.icon size={13} /> {t.label}</button>
            ))}
          </div>
          <span className="flex-1" />
          {tab === "preview" && (
            <>
              <a href="/api/export" className="icon-btn h-8 w-8" title="Download (.zip)"><Download size={15} /></a>
              <button onClick={() => setPreviewKey((k) => k + 1)} className="icon-btn h-8 w-8" title="Refresh"><RefreshCw size={15} /></button>
              <a href="/preview/" target="_blank" className="icon-btn h-8 w-8" title="Open in a tab"><ExternalLink size={15} /></a>
            </>
          )}
        </div>

        {tab === "preview" && (
          <div className="min-h-0 flex-1 p-3">
            <div className="surface elev flex h-full flex-col overflow-hidden rounded-2xl">
              <div className="flex h-9 items-center gap-2 border-b border-[var(--line)] px-3.5">
                <span className="flex gap-1.5"><i className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" /><i className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" /><i className="h-2.5 w-2.5 rounded-full bg-[#28c840]" /></span>
                <span className="mx-2 flex-1 truncate rounded-md bg-black/30 px-2.5 py-1 text-center text-[11px] text-[var(--dim)]">localhost preview</span>
              </div>
              <iframe key={previewKey} src={`/preview/?k=${previewKey}`} title="preview" className="flex-1 border-0 bg-white" sandbox="allow-scripts allow-forms allow-same-origin" />
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
        {tab === "trust" && <TrustPanel a={audit} rules={rules} />}
      </aside>
    </div>
  );
}
