import { useEffect, useRef, useState } from "react";

type Ev = { kind: string; text?: string; data?: any };
type State = {
  model?: string; provider?: string; mode?: string;
  spent_usd?: number; cap_usd?: number; ctx_frac?: number; running?: boolean;
};
type Template = { name: string; desc: string; prompt: string };
type Audit = { ok: boolean; entries: number; verified: number; usd: number; turns: number; reason?: string; verdicts?: Record<string, number> };
type Item =
  | { t: "you"; text: string }
  | { t: "assistant"; text: string }
  | { t: "tool"; text: string }
  | { t: "result"; text: string }
  | { t: "error"; text: string }
  | { t: "end" }
  | { t: "approval"; id: string; kind: string; target: string; answered?: "allowed" | "declined" };

// Gauge fills by level (green → amber → coral), so the budget/context meters turn
// red as a cap nears — the visual embodiment of the Trust Kernel.
function Gauge({ frac }: { frac: number }) {
  const f = Math.max(0, Math.min(1, frac || 0));
  const color = f < 0.6 ? "#36c08a" : f < 0.85 ? "#ebb950" : "#ff5a4d";
  return (
    <div className="h-1.5 w-16 overflow-hidden rounded-full bg-line">
      <div className="h-full rounded-full" style={{ width: `${f * 100}%`, background: color }} />
    </div>
  );
}

function TrustBadge({ a }: { a: Audit | null }) {
  if (!a || a.entries === 0) return null;
  const title = `${a.entries} receipts · ${a.turns} turns · $${(a.usd || 0).toFixed(4)}` + (a.reason ? ` · ${a.reason}` : "");
  return a.ok ? (
    <span title={title} className="rounded-full border border-ok/40 px-2 py-0.5 text-xs text-ok">✓ verified · {a.entries}</span>
  ) : (
    <span title={title} className="rounded-full border border-[#ff5a4d]/50 px-2 py-0.5 text-xs text-[#ff5a4d]">⚠ tamper</span>
  );
}

function Hud({ s, audit }: { s: State; audit: Audit | null }) {
  const cap = s.cap_usd || 0;
  return (
    <header className="flex h-12 items-center gap-4 border-b border-line px-4 text-sm">
      <span className="font-mono font-bold">cli<span className="text-accent">che</span> studio</span>
      <TrustBadge a={audit} />
      <span className="flex-1" />
      {s.mode && <span className="font-mono text-xs uppercase tracking-wide text-mut">{s.mode}</span>}
      {s.model && <span className="font-mono text-xs text-mut">{s.model}</span>}
      <span className="font-mono text-xs">${(s.spent_usd || 0).toFixed(4)}</span>
      {cap > 0 && (
        <span className="flex items-center gap-2">
          <Gauge frac={(s.spent_usd || 0) / cap} />
          <span className="font-mono text-xs text-mut">{Math.round((100 * (s.spent_usd || 0)) / cap)}%</span>
        </span>
      )}
      {(s.ctx_frac || 0) > 0 && (
        <span className="flex items-center gap-2">
          <span className="font-mono text-xs text-mut">ctx</span>
          <Gauge frac={s.ctx_frac || 0} />
        </span>
      )}
      {s.running && <span className="animate-pulse text-xs text-accent">● working</span>}
    </header>
  );
}

function ApprovalCard({ it, onAnswer }: { it: Extract<Item, { t: "approval" }>; onAnswer: (id: string, allow: boolean) => void }) {
  return (
    <div className="my-2 rounded-xl border border-accent bg-accent/10 p-3">
      <div className="mb-2 text-sm">
        Cliche wants to <b className="text-accent">{it.kind}</b>: <code className="font-mono text-ink">{it.target}</code>
      </div>
      {it.answered ? (
        <span className="text-xs text-mut">{it.answered}</span>
      ) : (
        <div className="flex gap-2">
          <button onClick={() => onAnswer(it.id, true)} className="rounded-lg bg-ok px-4 py-1.5 text-sm font-semibold text-[#06231a]">Allow</button>
          <button onClick={() => onAnswer(it.id, false)} className="rounded-lg px-4 py-1.5 text-sm text-mut hover:text-ink">Not now</button>
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
  const [previewKey, setPreviewKey] = useState(0); // bump to reload the preview iframe
  const feedRef = useRef<HTMLDivElement>(null);

  const refreshAudit = () => fetch("/api/audit").then((r) => r.json()).then(setAudit).catch(() => {});

  useEffect(() => { feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight }); }, [items]);

  useEffect(() => {
    fetch("/api/state").then((r) => r.json()).then(setState).catch(() => {});
    fetch("/api/templates").then((r) => r.json()).then(setTemplates).catch(() => {});
    refreshAudit();
    const es = new EventSource("/api/events");
    es.onmessage = (m) => {
      const e: Ev = JSON.parse(m.data);
      setItems((prev) => reduce(prev, e));
      if (e.kind === "state" && e.data) setState(e.data);
      if (e.kind === "end") { setPreviewKey((k) => k + 1); refreshAudit(); } // build done → refresh preview + receipts
    };
    return () => es.close();
  }, []);

  function answer(id: string, allow: boolean) {
    fetch("/api/approve", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, allow }) });
    setItems((prev) => prev.map((it) => (it.t === "approval" && it.id === id ? { ...it, answered: allow ? "allowed" : "declined" } : it)));
  }

  async function run(p: string) {
    if (!p.trim() || state.running) return;
    setItems((prev) => [...prev, { t: "you", text: p }]);
    setPrompt("");
    const r = await fetch("/api/prompt", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt: p }) });
    if (!r.ok) setItems((prev) => [...prev, { t: "error", text: r.status === 409 ? "a run is already in progress" : `request failed (${r.status})` }]);
  }

  return (
    <div className="flex h-full flex-col">
      <Hud s={state} audit={audit} />
      <div className="flex min-h-0 flex-1">
        {/* Conversation + build prompt */}
        <section className="flex min-w-0 flex-1 flex-col border-r border-line">
          <div ref={feedRef} className="flex-1 overflow-auto px-4 py-4 font-mono text-[13.5px] leading-relaxed">
            {items.length === 0 ? (
              <Welcome templates={templates} onPick={run} />
            ) : (
              items.map((it, i) => <Row key={i} it={it} onAnswer={answer} />)
            )}
          </div>
          <form
            onSubmit={(e) => { e.preventDefault(); run(prompt); }}
            className="flex gap-2 border-t border-line p-3"
          >
            <input
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="Describe what you want to build…"
              autoFocus
              className="flex-1 rounded-xl border border-line bg-panel px-4 py-3 text-[15px] outline-none focus:border-accent"
            />
            <button disabled={state.running} className="rounded-xl bg-accent px-5 font-bold text-[#1a0c0a] disabled:opacity-50">Build</button>
          </form>
        </section>

        {/* Live preview of what's being built */}
        <aside className="flex w-[44%] min-w-[320px] flex-col bg-panel">
          <div className="flex h-9 items-center gap-2 border-b border-line px-3 text-xs text-mut">
            <span>Live preview</span>
            <span className="flex-1" />
            <a href="/api/export" className="rounded px-2 py-0.5 hover:text-ink" title="Download the project (.zip)">⬇ Export</a>
            <button onClick={() => setPreviewKey((k) => k + 1)} className="rounded px-2 py-0.5 hover:text-ink" title="Refresh">↻</button>
            <a href="/preview/" target="_blank" className="rounded px-2 py-0.5 hover:text-ink" title="Open in a tab">↗</a>
          </div>
          <iframe
            key={previewKey}
            src={`/preview/?k=${previewKey}`}
            title="preview"
            className="flex-1 border-0 bg-white"
            sandbox="allow-scripts allow-forms allow-same-origin"
          />
        </aside>
      </div>
    </div>
  );
}

function Welcome({ templates, onPick }: { templates: Template[]; onPick: (p: string) => void }) {
  return (
    <div className="mx-auto mt-12 max-w-2xl text-center font-sans">
      <div className="mb-2 text-2xl font-semibold">Build something.</div>
      <div className="mb-8 text-sm text-mut">Describe it, or start from one of these — Cliche makes it, you watch, and approve each step.</div>
      <div className="grid grid-cols-2 gap-3 text-left">
        {templates.map((t) => (
          <button
            key={t.name}
            onClick={() => onPick(t.prompt)}
            className="rounded-xl border border-line bg-panel p-4 transition-colors hover:border-accent"
          >
            <div className="font-semibold">{t.name}</div>
            <div className="mt-1 text-xs text-mut">{t.desc}</div>
          </button>
        ))}
      </div>
    </div>
  );
}

function Row({ it, onAnswer }: { it: Item; onAnswer: (id: string, allow: boolean) => void }) {
  switch (it.t) {
    case "you": return <div className="my-1 text-mut">❯ {it.text}</div>;
    case "assistant": return <div className="my-1 whitespace-pre-wrap text-ink">{it.text}</div>;
    case "tool": return <div className="my-0.5 text-accent">▸ {it.text}</div>;
    case "result": return <div className="my-0.5 text-ok">✓ {it.text}</div>;
    case "error": return <div className="my-0.5 text-[#ff5a4d]">✗ {it.text}</div>;
    case "end": return <div className="my-1 text-mut">— done —</div>;
    case "approval": return <ApprovalCard it={it} onAnswer={onAnswer} />;
  }
}

function reduce(prev: Item[], e: Ev): Item[] {
  switch (e.kind) {
    case "delta": {
      const last = prev[prev.length - 1];
      if (last && last.t === "assistant") {
        return [...prev.slice(0, -1), { t: "assistant", text: last.text + (e.text || "") }];
      }
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
