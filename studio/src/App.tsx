import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import {
  ShieldCheck, ShieldAlert, Download, RefreshCw, ExternalLink, ArrowUp,
  Sparkles, Check, Wrench, Globe, Wand2, Hammer, FileSearch, KeyRound, CircleAlert,
} from "lucide-react";

type Ev = { kind: string; text?: string; data?: any };
type State = {
  model?: string; provider?: string; mode?: string;
  spent_usd?: number; cap_usd?: number; ctx_frac?: number; running?: boolean; needs_setup?: boolean;
};
type Template = { name: string; desc: string; prompt: string };
type Audit = { ok: boolean; entries: number; verified: number; usd: number; turns: number; reason?: string };
type Item =
  | { t: "you"; text: string }
  | { t: "assistant"; text: string }
  | { t: "tool"; text: string }
  | { t: "result"; text: string }
  | { t: "error"; text: string }
  | { t: "end" }
  | { t: "approval"; id: string; kind: string; target: string; answered?: "allowed" | "declined" };

const TEMPLATE_ICONS: Record<string, any> = { Website: Globe, "Automate a task": Wand2, "Small tool": Hammer, "Explain this project": FileSearch };

function Gauge({ frac }: { frac: number }) {
  const f = Math.max(0, Math.min(1, frac || 0));
  const color = f < 0.6 ? "var(--ok)" : f < 0.85 ? "#ebb950" : "var(--accent)";
  return (
    <div className="h-1.5 w-14 overflow-hidden rounded-full bg-white/10">
      <div className="h-full rounded-full transition-all duration-500" style={{ width: `${f * 100}%`, background: color, boxShadow: `0 0 8px ${color}` }} />
    </div>
  );
}

function TrustBadge({ a }: { a: Audit | null }) {
  if (!a || a.entries === 0) return null;
  const title = `${a.entries} receipts · ${a.turns} turns · $${(a.usd || 0).toFixed(4)}${a.reason ? ` · ${a.reason}` : ""}`;
  return a.ok ? (
    <span title={title} className="flex items-center gap-1 rounded-full border border-[var(--ok)]/30 bg-[var(--ok)]/10 px-2.5 py-1 text-xs text-[var(--ok)]">
      <ShieldCheck size={13} /> verified · {a.entries}
    </span>
  ) : (
    <span title={title} className="flex items-center gap-1 rounded-full border border-[var(--accent)]/40 bg-[var(--accent)]/10 px-2.5 py-1 text-xs text-[var(--accent)]">
      <ShieldAlert size={13} /> tamper
    </span>
  );
}

function Hud({ s, audit }: { s: State; audit: Audit | null }) {
  const cap = s.cap_usd || 0;
  return (
    <header className="glass sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-[var(--line)] px-5">
      <span className="font-mono text-[15px] font-bold tracking-tight">cli<span className="text-[var(--accent)]">ché</span></span>
      <span className="text-xs text-[var(--dim)]">studio</span>
      <TrustBadge a={audit} />
      <span className="flex-1" />
      {s.mode && <span className="rounded-full border border-[var(--line)] px-2 py-0.5 font-mono text-[11px] uppercase tracking-wider text-[var(--mut)]">{s.mode}</span>}
      {s.model && <span className="font-mono text-xs text-[var(--mut)]">{s.model}</span>}
      <span className="font-mono text-xs text-[var(--ink)]">${(s.spent_usd || 0).toFixed(4)}</span>
      {cap > 0 && (
        <span className="flex items-center gap-2">
          <Gauge frac={(s.spent_usd || 0) / cap} />
          <span className="font-mono text-[11px] text-[var(--mut)]">{Math.round((100 * (s.spent_usd || 0)) / cap)}%</span>
        </span>
      )}
      {(s.ctx_frac || 0) > 0 && (
        <span className="flex items-center gap-1.5">
          <span className="font-mono text-[11px] text-[var(--dim)]">ctx</span>
          <Gauge frac={s.ctx_frac || 0} />
        </span>
      )}
      {s.running && <span className="pulse-soft flex items-center gap-1 text-xs text-[var(--accent)]"><Sparkles size={13} /> working</span>}
    </header>
  );
}

function ApprovalCard({ it, onAnswer }: { it: Extract<Item, { t: "approval" }>; onAnswer: (id: string, allow: boolean) => void }) {
  return (
    <div className="fade-up my-3 rounded-2xl border border-[var(--accent)]/40 bg-[var(--accent)]/[0.07] p-4">
      <div className="mb-3 flex items-center gap-2 text-sm">
        <CircleAlert size={16} className="text-[var(--accent)]" />
        Cliche wants to <b className="text-[var(--accent)]">{it.kind}</b>: <code className="rounded bg-black/30 px-1.5 py-0.5 font-mono text-[13px]">{it.target}</code>
      </div>
      {it.answered ? (
        <span className="text-xs text-[var(--mut)]">{it.answered === "allowed" ? "✓ allowed" : "declined"}</span>
      ) : (
        <div className="flex gap-2">
          <button onClick={() => onAnswer(it.id, true)} className="rounded-xl bg-[var(--ok)] px-4 py-2 text-sm font-semibold text-[#06231a]">Allow</button>
          <button onClick={() => onAnswer(it.id, false)} className="rounded-xl px-4 py-2 text-sm text-[var(--mut)] hover:text-[var(--ink)]">Not now</button>
        </div>
      )}
    </div>
  );
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
        ) : (
          <p className="mb-2 text-xs text-[var(--mut)]">Make sure Ollama is running on your machine — no key needed.</p>
        )}

        {err && <div className="mt-3 flex items-center gap-1.5 text-xs text-[var(--accent)]"><CircleAlert size={13} /> {err}</div>}
        <button onClick={connect} disabled={busy || (!p.local && !key.trim())} className="btn-accent mt-6 w-full rounded-2xl py-3.5 text-[15px]">
          {busy ? "connecting…" : "Connect & start building"}
        </button>
      </div>
    </div>
  );
}

function Welcome({ templates, onPick }: { templates: Template[]; onPick: (p: string) => void }) {
  return (
    <div className="mx-auto mt-16 max-w-2xl px-4 text-center">
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
              <span>
                <span className="block font-medium">{t.name}</span>
                <span className="mt-0.5 block text-xs text-[var(--mut)]">{t.desc}</span>
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function Row({ it, onAnswer }: { it: Item; onAnswer: (id: string, allow: boolean) => void }) {
  switch (it.t) {
    case "you":
      return <div className="fade-up my-3 flex justify-end"><div className="max-w-[80%] rounded-2xl rounded-br-md bg-white/[0.06] px-4 py-2 text-[14.5px]">{it.text}</div></div>;
    case "assistant":
      return <div className="md fade-up my-2"><ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>{it.text}</ReactMarkdown></div>;
    case "tool":
      return <div className="fade-up my-1 flex items-center gap-2 text-[13px] text-[var(--accent)]"><Wrench size={13} /> {it.text}</div>;
    case "result":
      return <div className="fade-up my-1 flex items-center gap-2 text-[13px] text-[var(--ok)]"><Check size={13} /> {it.text}</div>;
    case "error":
      return <div className="fade-up my-1 flex items-center gap-2 text-[13px] text-[var(--accent)]"><CircleAlert size={13} /> {it.text}</div>;
    case "end":
      return <div className="my-3 flex items-center gap-3 text-xs text-[var(--dim)]"><span className="h-px flex-1 bg-[var(--line)]" /> done <span className="h-px flex-1 bg-[var(--line)]" /></div>;
    case "approval":
      return <ApprovalCard it={it} onAnswer={onAnswer} />;
  }
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

export default function App() {
  const [items, setItems] = useState<Item[]>([]);
  const [state, setState] = useState<State>({});
  const [prompt, setPrompt] = useState("");
  const [templates, setTemplates] = useState<Template[]>([]);
  const [audit, setAudit] = useState<Audit | null>(null);
  const [previewKey, setPreviewKey] = useState(0);
  const feedRef = useRef<HTMLDivElement>(null);

  const refreshAudit = () => fetch("/api/audit").then((r) => r.json()).then(setAudit).catch(() => {});
  useEffect(() => { feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight, behavior: "smooth" }); }, [items]);

  useEffect(() => {
    fetch("/api/state").then((r) => r.json()).then(setState).catch(() => {});
    fetch("/api/templates").then((r) => r.json()).then(setTemplates).catch(() => {});
    refreshAudit();
    const es = new EventSource("/api/events");
    es.onmessage = (m) => {
      const e: Ev = JSON.parse(m.data);
      setItems((prev) => reduce(prev, e));
      if (e.kind === "state" && e.data) setState(e.data);
      if (e.kind === "end") { setPreviewKey((k) => k + 1); refreshAudit(); }
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
  }

  if (state.needs_setup) return <Setup onDone={() => fetch("/api/state").then((r) => r.json()).then(setState)} />;

  return (
    <div className="flex h-full flex-col">
      <Hud s={state} audit={audit} />
      <div className="flex min-h-0 flex-1">
        {/* conversation + composer */}
        <section className="flex min-w-0 flex-1 flex-col">
          <div ref={feedRef} className="flex-1 overflow-auto">
            {items.length === 0 ? (
              <Welcome templates={templates} onPick={run} />
            ) : (
              <div className="mx-auto max-w-3xl px-5 py-6 font-mono text-[13.5px] leading-relaxed">
                {items.map((it, i) => <Row key={i} it={it} onAnswer={answer} />)}
              </div>
            )}
          </div>
          <div className="px-5 pb-5">
            <form onSubmit={(e) => { e.preventDefault(); run(prompt); }} className="surface mx-auto flex max-w-3xl items-center gap-2 rounded-2xl p-2 pl-4 shadow-xl focus-within:border-[var(--accent)]/60">
              <input
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="Describe what you want to build…"
                autoFocus
                className="flex-1 bg-transparent py-2.5 text-[15px] outline-none placeholder:text-[var(--dim)]"
              />
              <button disabled={state.running || !prompt.trim()} className="btn-accent grid h-9 w-9 place-items-center rounded-xl" title="Build">
                <ArrowUp size={18} />
              </button>
            </form>
          </div>
        </section>

        {/* live preview, in a browser frame */}
        <aside className="flex w-[46%] min-w-[340px] flex-col border-l border-[var(--line)] p-4 pl-0">
          <div className="surface flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl">
            <div className="flex h-10 items-center gap-2 border-b border-[var(--line)] px-3.5">
              <span className="flex gap-1.5"><i className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" /><i className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" /><i className="h-2.5 w-2.5 rounded-full bg-[#28c840]" /></span>
              <span className="mx-2 flex-1 truncate rounded-md bg-black/30 px-2.5 py-1 text-center text-[11px] text-[var(--dim)]">localhost preview</span>
              <a href="/api/export" className="rounded-md p-1.5 text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Download (.zip)"><Download size={15} /></a>
              <button onClick={() => setPreviewKey((k) => k + 1)} className="rounded-md p-1.5 text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Refresh"><RefreshCw size={15} /></button>
              <a href="/preview/" target="_blank" className="rounded-md p-1.5 text-[var(--mut)] hover:bg-white/5 hover:text-[var(--ink)]" title="Open in a tab"><ExternalLink size={15} /></a>
            </div>
            <iframe key={previewKey} src={`/preview/?k=${previewKey}`} title="preview" className="flex-1 border-0 bg-white" sandbox="allow-scripts allow-forms allow-same-origin" />
          </div>
        </aside>
      </div>
    </div>
  );
}
