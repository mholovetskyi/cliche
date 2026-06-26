import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import hljs from "highlight.js";
import {
  ShieldCheck, ShieldAlert, Download, RefreshCw, ExternalLink, ArrowUp, Sparkles,
  Check, Wrench, Globe, Wand2, Hammer, FileSearch, KeyRound, CircleAlert, Plus,
  MessageSquare, Folder, FolderOpen, FileText, Eye, ListTree, ChevronRight, Square, FileDiff, ImagePlus, X,
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
  if (n * m > 400000) return b.map((t) => ({ type: "add", text: t })); // too big for LCS — show as added
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
  { id: "plan", label: "Plan", hint: "read-only" },
  { id: "suggest", label: "Suggest", hint: "ask each step" },
  { id: "auto-edit", label: "Auto-edit", hint: "auto-apply edits" },
  { id: "full", label: "Full", hint: "auto-approve all" },
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

function Gauge({ frac }: { frac: number }) {
  const f = Math.max(0, Math.min(1, frac || 0));
  const color = f < 0.6 ? "var(--ok)" : f < 0.85 ? "#ebb950" : "var(--accent)";
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
      <div className="surface fade-up w-[460px] rounded-3xl p-8 shadow-2xl">
        <div className="mb-1 flex items-center gap-2">
          <span className="grid h-9 w-9 place-items-center rounded-xl btn-accent"><Sparkles size={18} /></span>
          <span className="font-mono text-lg font-bold tracking-tight">welcome to cli<span className="text-[var(--accent)]">ché</span> studio</span>
        </div>
        <p className="mb-7 text-sm text-[var(--mut)]">Connect a model to get started. Your key stays on this computer.</p>
        <label className="mb-1.5 block text-xs text-[var(--mut)]">Provider</label>
        <select value={provider} onChange={(e) => setProvider(e.target.value)} className="ring-focus mb-4 w-full rounded-xl border border-[var(--line)] bg-black/30 px-3.5 py-2.5 text-sm">
          {providers.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
        </select>
        {!p.local ? (
          <>
            <label className="mb-1.5 block text-xs text-[var(--mut)]">API key</label>
            <div className="relative mb-2">
              <KeyRound size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--dim)]" />
              <input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="paste your key…" className="ring-focus w-full rounded-xl border border-[var(--line)] bg-black/30 py-2.5 pl-9 pr-3 font-mono text-sm" />
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
    <aside className="flex w-60 shrink-0 flex-col border-r border-[var(--line)]">
      <div className="flex h-14 items-center gap-2 px-4">
        <span className="grid h-7 w-7 place-items-center rounded-lg btn-accent"><Sparkles size={15} /></span>
        <span className="font-mono text-[15px] font-bold tracking-tight">cli<span className="text-[var(--accent)]">ché</span></span>
      </div>
      <div className="px-3">
        <button onClick={onNew} className="surface card-hover flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-sm font-medium">
          <Plus size={16} className="text-[var(--accent)]" /> New chat
        </button>
      </div>
      <div className="mt-3 flex-1 overflow-auto px-2">
        <div className="px-2 pb-1 text-[11px] uppercase tracking-wider text-[var(--dim)]">Chats</div>
        {sessions.length === 0 && <div className="px-2 py-2 text-xs text-[var(--dim)]">No chats yet</div>}
        {sessions.map((s) => (
          <button key={s.id} onClick={() => onPick(s.id)} title={s.title}
            className={`group mb-0.5 flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-[13px] transition-colors ${s.active ? "bg-white/[0.07] text-[var(--ink)]" : "text-[var(--mut)] hover:bg-white/[0.04]"}`}>
            <MessageSquare size={14} className={s.active ? "text-[var(--accent)]" : "text-[var(--dim)]"} />
            <span className="min-w-0 flex-1">
              <span className="block truncate">{s.title || "New chat"}</span>
              <span className="block text-[11px] text-[var(--dim)]">{relTime(s.updated)} · {s.messages} msgs</span>
            </span>
          </button>
        ))}
      </div>
      {tasks.length > 0 && (
        <div className="max-h-[40%] overflow-auto border-t border-[var(--line)] p-2">
          <div className="flex items-center px-2 pb-1 text-[11px] uppercase tracking-wider text-[var(--dim)]">
            <span className="flex-1">Plan · {doneCount}/{tasks.length}</span>
            <button onClick={onClearTasks} className="hover:text-[var(--mut)]" title="Clear the plan">clear</button>
          </div>
          {tasks.map((t) => (
            <button key={t.id} onClick={() => onToggleTask(t.id)} className="flex w-full items-start gap-2 rounded-md px-2 py-1 text-left text-[13px] hover:bg-white/[0.04]">
              <span className={`mt-0.5 grid h-3.5 w-3.5 shrink-0 place-items-center rounded border ${t.done ? "border-[var(--ok)] bg-[var(--ok)]/20 text-[var(--ok)]" : "border-[var(--line2)]"}`}>{t.done && <Check size={10} />}</span>
              <span className={t.done ? "text-[var(--dim)] line-through" : "text-[var(--mut)]"}>{t.title}</span>
            </button>
          ))}
        </div>
      )}
      <div className="border-t border-[var(--line)] p-3 text-xs">
        {audit && audit.entries > 0 && (
          <div className="mb-2 flex items-center gap-1.5">
            {audit.ok ? <span className="flex items-center gap-1 text-[var(--ok)]"><ShieldCheck size={13} /> verified</span>
                      : <span className="flex items-center gap-1 text-[var(--accent)]"><ShieldAlert size={13} /> tamper</span>}
            <span className="text-[var(--dim)]">· {audit.entries} receipts</span>
          </div>
        )}
        <div className="mb-1 flex items-center justify-between text-[var(--mut)]">
          <span className="truncate font-mono text-[11px]">{state.model || "—"}</span>
          <span className="font-mono">${(state.spent_usd || 0).toFixed(3)}</span>
        </div>
        {cap > 0 && <Gauge frac={(state.spent_usd || 0) / cap} />}
      </div>
    </aside>
  );
}

function ApprovalCard({ it, onAnswer }: { it: Extract<Item, { t: "approval" }>; onAnswer: (id: string, allow: boolean) => void }) {
  return (
    <div className="fade-up my-3 rounded-2xl border border-[var(--accent)]/40 bg-[var(--accent)]/[0.07] p-4">
      <div className="mb-3 flex items-center gap-2 text-sm">
        <CircleAlert size={16} className="text-[var(--accent)]" />
        Cliche wants to <b className="text-[var(--accent)]">{it.kind}</b>: <code className="rounded bg-black/30 px-1.5 py-0.5 font-mono text-[13px]">{it.target}</code>
      </div>
      {it.answered ? <span className="text-xs text-[var(--mut)]">{it.answered === "allowed" ? "✓ allowed" : "declined"}</span> : (
        <div className="flex gap-2">
          <button onClick={() => onAnswer(it.id, true)} className="rounded-xl bg-[var(--ok)] px-4 py-2 text-sm font-semibold text-[#06231a]">Allow</button>
          <button onClick={() => onAnswer(it.id, false)} className="rounded-xl px-4 py-2 text-sm text-[var(--mut)] hover:text-[var(--ink)]">Not now</button>
        </div>
      )}
    </div>
  );
}

function Row({ it, onAnswer }: { it: Item; onAnswer: (id: string, allow: boolean) => void }) {
  switch (it.t) {
    case "you": return <div className="fade-up my-3 flex justify-end"><div className="max-w-[80%] rounded-2xl rounded-br-md bg-white/[0.06] px-4 py-2 text-[14.5px]">{it.text}</div></div>;
    case "assistant": return <div className="md fade-up my-2"><ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>{it.text}</ReactMarkdown></div>;
    case "tool": return <div className="fade-up my-1 flex items-center gap-2 text-[13px] text-[var(--accent)]"><Wrench size={13} /> {it.text}</div>;
    case "result": return <div className="fade-up my-1 flex items-center gap-2 text-[13px] text-[var(--ok)]"><Check size={13} /> {it.text}</div>;
    case "error": return <div className="fade-up my-1 flex items-center gap-2 text-[13px] text-[var(--accent)]"><CircleAlert size={13} /> {it.text}</div>;
    case "end": return <div className="my-3 flex items-center gap-3 text-xs text-[var(--dim)]"><span className="h-px flex-1 bg-[var(--line)]" /> done <span className="h-px flex-1 bg-[var(--line)]" /></div>;
    case "approval": return <ApprovalCard it={it} onAnswer={onAnswer} />;
  }
}

function Welcome({ templates, onPick }: { templates: Template[]; onPick: (p: string) => void }) {
  return (
    <div className="mx-auto mt-14 max-w-2xl px-4 text-center">
      <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-[var(--line)] bg-white/[0.03] px-3 py-1 text-xs text-[var(--mut)]">
        <Sparkles size={13} className="text-[var(--accent)]" /> trust-first · local · your key
      </div>
      <h1 className="mb-2 text-3xl font-semibold tracking-tight">What do you want to build?</h1>
      <p className="mb-9 text-[var(--mut)]">Describe it, or start from one of these. Cliche makes it — you watch, and approve each step.</p>
      <div className="grid grid-cols-2 gap-3 text-left">
        {templates.map((t) => {
          const Icon = TEMPLATE_ICONS[t.name] || Sparkles;
          return (
            <button key={t.name} onClick={() => onPick(t.prompt)} className="surface card-hover group flex items-start gap-3 rounded-2xl p-4">
              <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl border border-[var(--line)] bg-white/[0.03] text-[var(--accent)] transition-colors group-hover:bg-[var(--accent)]/10"><Icon size={17} /></span>
              <span><span className="block font-medium">{t.name}</span><span className="mt-0.5 block text-xs text-[var(--mut)]">{t.desc}</span></span>
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
          <button onClick={() => onToggle(n.path)} className="flex w-full items-center gap-1.5 rounded-md py-1 pr-2 text-left text-[13px] text-[var(--mut)] hover:bg-white/[0.04]" style={{ paddingLeft: 8 + depth * 12 }}>
            <ChevronRight size={13} className={`shrink-0 transition-transform ${expanded.has(n.path) ? "rotate-90" : ""}`} />
            {expanded.has(n.path) ? <FolderOpen size={14} className="text-[var(--accent)]" /> : <Folder size={14} className="text-[var(--dim)]" />}
            <span className="truncate">{n.name}</span>
          </button>
          {expanded.has(n.path) && n.children && <Tree nodes={n.children} depth={depth + 1} expanded={expanded} onToggle={onToggle} onOpen={onOpen} active={active} />}
        </div>
      ) : (
        <button key={n.path} onClick={() => onOpen(n.path)} className={`flex w-full items-center gap-1.5 rounded-md py-1 pr-2 text-left text-[13px] hover:bg-white/[0.04] ${active === n.path ? "bg-white/[0.07] text-[var(--ink)]" : "text-[var(--mut)]"}`} style={{ paddingLeft: 8 + depth * 12 + 14 }}>
          <FileText size={13} className="shrink-0 text-[var(--dim)]" /><span className="truncate">{n.name}</span>
        </button>
      ))}
    </>
  );
}

function RuleList({ label, items, empty }: { label: string; items: string[]; empty: string }) {
  return (
    <div className="flex gap-2 border-t border-[var(--line)] py-2 text-[13px] first:border-0">
      <span className="w-16 shrink-0 text-[var(--dim)]">{label}</span>
      {items.length === 0 ? <span className="text-[var(--dim)]">{empty}</span> :
        <span className="flex flex-wrap gap-1.5">{items.map((x, i) => <code key={i} className="rounded bg-white/[0.06] px-1.5 py-0.5 font-mono text-[12px]">{x}</code>)}</span>}
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
          <div className={`mb-4 flex items-center gap-2 rounded-xl border p-3 ${a.ok ? "border-[var(--ok)]/30 bg-[var(--ok)]/[0.06] text-[var(--ok)]" : "border-[var(--accent)]/40 bg-[var(--accent)]/[0.06] text-[var(--accent)]"}`}>
            {a.ok ? <ShieldCheck size={18} /> : <ShieldAlert size={18} />}
            <span className="text-sm font-medium">{a.ok ? "Ledger intact — every action is a signed, hash-chained receipt." : `Tamper detected${a.reason ? `: ${a.reason}` : ""}`}</span>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {tiles.map((t) => (
              <div key={t.label} className="surface rounded-xl p-4">
                <div className="text-[11px] uppercase tracking-wider text-[var(--dim)]">{t.label}</div>
                <div className="mt-1 font-mono text-2xl">{t.value}</div>
              </div>
            ))}
          </div>
          {a.verdicts && Object.keys(a.verdicts).length > 0 && (
            <div className="mt-5">
              <div className="mb-2 text-[11px] uppercase tracking-wider text-[var(--dim)]">Verifier verdicts</div>
              <div className="flex flex-wrap gap-2">
                {Object.entries(a.verdicts).map(([k, v]) => (
                  <span key={k} className="rounded-full border border-[var(--line)] px-2.5 py-1 text-xs text-[var(--mut)]">{k}: <b className="text-[var(--ink)]">{v}</b></span>
                ))}
              </div>
            </div>
          )}
        </>
      ) : <div className="mb-2 text-sm text-[var(--mut)]">No receipts yet — the trust ledger fills in as Cliche works.</div>}
      {rules && (
        <div className="mt-6">
          <div className="mb-2 text-[11px] uppercase tracking-wider text-[var(--dim)]">Rules in force</div>
          <div className="surface rounded-xl px-4 py-2">
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
  if (changes.length === 0) return <div className="p-6 text-sm text-[var(--mut)]">No file changes yet — edits the agent makes show here as diffs you can undo.</div>;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b border-[var(--line)] px-4 py-2 text-xs">
        <span className="text-[var(--mut)]">{changes.length} file{changes.length > 1 ? "s" : ""} changed</span>
        <span className="flex-1" />
        <button onClick={onUndo} className="rounded-lg border border-[var(--line)] px-2.5 py-1 text-[var(--mut)] hover:text-[var(--ink)]">Undo last</button>
        <button onClick={onRevertAll} className="rounded-lg border border-[var(--accent)]/40 px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent)]/10">Revert all</button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {changes.map((c) => {
          const badge = c.was_new ? "new" : c.deleted ? "deleted" : "modified";
          const rows = lineDiff(c.before, c.after);
          const shown = rows.slice(0, 400);
          return (
            <div key={c.path} className="surface mb-3 overflow-hidden rounded-xl">
              <div className="flex items-center gap-2 border-b border-[var(--line)] px-3 py-2 font-mono text-[12px]">
                <FileText size={13} className="text-[var(--dim)]" />
                <span className="min-w-0 truncate">{c.path}</span>
                <span className={`rounded-full px-1.5 py-0.5 text-[10px] ${c.deleted ? "bg-[var(--accent)]/15 text-[var(--accent)]" : c.was_new ? "bg-[var(--ok)]/15 text-[var(--ok)]" : "bg-white/10 text-[var(--mut)]"}`}>{badge}</span>
              </div>
              <pre className="overflow-auto p-2 text-[12px] leading-[1.5]">
                {shown.map((r, i) => (
                  <div key={i} className={r.type === "add" ? "bg-[var(--ok)]/[0.10] text-[var(--ok)]" : r.type === "del" ? "bg-[var(--accent)]/[0.10] text-[var(--accent)]" : "text-[var(--mut)]"}>
                    <span className="select-none opacity-50">{r.type === "add" ? "+ " : r.type === "del" ? "- " : "  "}</span>{r.text || " "}
                  </div>
                ))}
                {rows.length > shown.length && <div className="text-[var(--dim)]">… {rows.length - shown.length} more lines</div>}
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
    else setImgCount(0); // images ride this turn, then the server clears them
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
  async function undo() {
    await fetch("/api/undo", { method: "POST" });
    refreshChanges(); refreshFiles(); setPreviewKey((k) => k + 1);
  }
  async function rewind() {
    await fetch("/api/rewind", { method: "POST" });
    refreshChanges(); refreshFiles(); setPreviewKey((k) => k + 1);
  }
  async function setModel(model: string) {
    await fetch("/api/model", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ model }) });
    refreshState();
  }
  function stop() { fetch("/api/stop", { method: "POST" }); }

  // runCommand interprets a /slash entry the way the CLI does. Returns true if it
  // was a recognized command (so it isn't sent to the model as a prompt).
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
    { id: "changes", label: changes.length ? `Changes (${changes.length})` : "Changes", icon: FileDiff },
    { id: "trust", label: "Trust", icon: ShieldCheck },
  ];

  return (
    <div className="flex h-full">
      <Sidebar sessions={sessions} state={state} audit={audit} tasks={tasks} onNew={newChat} onPick={pickSession} onToggleTask={toggleTask} onClearTasks={clearTasks} />

      {/* conversation */}
      <section className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center gap-2 border-b border-[var(--line)] px-5">
          <span className="min-w-0 truncate text-sm font-medium">{activeTitle || "New chat"}</span>
          {state.running && <span className="pulse-soft flex items-center gap-1 text-xs text-[var(--accent)]"><Sparkles size={13} /> working</span>}
          <span className="flex-1" />
          {state.running && (
            <button onClick={stop} className="flex items-center gap-1.5 rounded-lg border border-[var(--accent)]/40 bg-[var(--accent)]/[0.08] px-2.5 py-1 text-xs text-[var(--accent)] hover:bg-[var(--accent)]/[0.15]" title="Stop the run (Esc)">
              <Square size={11} /> Stop
            </button>
          )}
          <select value={state.mode || "suggest"} onChange={(e) => setMode(e.target.value)} title="Permission mode"
            className="rounded-lg border border-[var(--line)] bg-black/30 px-2 py-1 text-xs text-[var(--mut)] outline-none hover:text-[var(--ink)] focus:border-[var(--accent)]">
            {MODES.map((m) => <option key={m.id} value={m.id}>{m.label} · {m.hint}</option>)}
          </select>
          <select value={state.model || ""} onChange={(e) => setModel(e.target.value)} title="Model"
            className="max-w-[180px] rounded-lg border border-[var(--line)] bg-black/30 px-2 py-1 font-mono text-xs text-[var(--mut)] outline-none hover:text-[var(--ink)] focus:border-[var(--accent)]">
            {state.model && !models.some((m) => m.model === state.model) && <option value={state.model}>{state.model}</option>}
            {models.map((m) => <option key={m.model} value={m.model}>{m.model}</option>)}
          </select>
        </header>
        <div ref={feedRef} className="flex-1 overflow-auto">
          {items.length === 0 ? <Welcome templates={templates} onPick={run} /> : (
            <div className="mx-auto max-w-3xl px-5 py-6 font-mono text-[13.5px] leading-relaxed">
              {items.map((it, i) => <Row key={i} it={it} onAnswer={answer} />)}
            </div>
          )}
        </div>
        <div className="px-5 pb-5">
          <div className="mx-auto max-w-3xl">
            {prompt.startsWith("/") && (
              <div className="surface mb-2 overflow-hidden rounded-xl shadow-xl">
                {allCommands.filter((c) => ("/" + c.cmd).startsWith(prompt.split(/\s+/)[0])).slice(0, 8).map((c) => (
                  <button key={c.cmd} type="button"
                    onClick={() => { if (c.arg) setPrompt("/" + c.cmd + " "); else { runCommand("/" + c.cmd); setPrompt(""); } }}
                    className="flex w-full items-center gap-3 px-3 py-2 text-left text-[13px] hover:bg-white/[0.05]">
                    <span className="font-mono text-[var(--accent)]">/{c.cmd}</span>
                    <span className="text-[var(--mut)]">{c.desc}</span>
                  </button>
                ))}
              </div>
            )}
            <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadImage(f); e.target.value = ""; }} />
            <form onSubmit={(e) => { e.preventDefault(); submit(); }} className="surface flex items-center gap-2 rounded-2xl p-2 pl-2 shadow-xl focus-within:border-[var(--accent)]/60">
              <button type="button" onClick={() => fileRef.current?.click()} className="grid h-9 w-9 shrink-0 place-items-center rounded-xl text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Attach an image"><ImagePlus size={18} /></button>
              {imgCount > 0 && (
                <span className="flex shrink-0 items-center gap-1 rounded-full bg-[var(--accent)]/15 px-2 py-1 text-xs text-[var(--accent)]">
                  {imgCount} image{imgCount > 1 ? "s" : ""}
                  <button type="button" onClick={() => { fetch("/api/image/clear", { method: "POST" }); setImgCount(0); }} title="Remove"><X size={11} /></button>
                </span>
              )}
              <input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="Describe what you want to build…   ( / commands · @ files )" autoFocus className="flex-1 bg-transparent py-2.5 text-[15px] outline-none placeholder:text-[var(--dim)]" />
              <button disabled={(state.running && !prompt.startsWith("/")) || !prompt.trim()} className="btn-accent grid h-9 w-9 place-items-center rounded-xl" title="Build"><ArrowUp size={18} /></button>
            </form>
          </div>
        </div>
      </section>

      {/* workspace */}
      <aside className="flex w-[42%] min-w-[360px] flex-col border-l border-[var(--line)]">
        <div className="flex h-14 items-center gap-1 border-b border-[var(--line)] px-3">
          {tabs.map((t) => (
            <button key={t.id} onClick={() => setTab(t.id)} className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[13px] transition-colors ${tab === t.id ? "bg-white/[0.07] text-[var(--ink)]" : "text-[var(--mut)] hover:bg-white/[0.04]"}`}>
              <t.icon size={14} /> {t.label}
            </button>
          ))}
          <span className="flex-1" />
          {tab === "preview" && (
            <>
              <a href="/api/export" className="rounded-md p-1.5 text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Download (.zip)"><Download size={15} /></a>
              <button onClick={() => setPreviewKey((k) => k + 1)} className="rounded-md p-1.5 text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Refresh"><RefreshCw size={15} /></button>
              <a href="/preview/" target="_blank" className="rounded-md p-1.5 text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Open in a tab"><ExternalLink size={15} /></a>
            </>
          )}
        </div>

        {tab === "preview" && (
          <div className="min-h-0 flex-1 p-3">
            <div className="surface flex h-full flex-col overflow-hidden rounded-2xl">
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
            <div className="w-1/2 overflow-auto border-r border-[var(--line)] p-2">
              {tree.length === 0 ? <div className="p-3 text-xs text-[var(--dim)]">No files yet</div> :
                <Tree nodes={tree} depth={0} expanded={expanded} onToggle={toggleDir} onOpen={openFileAt} active={openFile?.path || ""} />}
            </div>
            <div className="min-w-0 flex-1 overflow-auto">
              {openFile ? (
                <>
                  <div className="sticky top-0 border-b border-[var(--line)] bg-[var(--bg)]/80 px-4 py-2 font-mono text-[11px] text-[var(--mut)] backdrop-blur">{openFile.path}</div>
                  <pre className="hljs !bg-transparent p-4 text-[12.5px] leading-relaxed"><code dangerouslySetInnerHTML={{ __html: openFile.html }} /></pre>
                </>
              ) : <div className="grid h-full place-items-center p-6 text-sm text-[var(--dim)]">Select a file to view it</div>}
            </div>
          </div>
        )}

        {tab === "changes" && <ChangesPanel changes={changes} onUndo={undo} onRevertAll={rewind} />}
        {tab === "trust" && <TrustPanel a={audit} rules={rules} />}
      </aside>
    </div>
  );
}
