import { useEffect, useRef, useState } from "react";

type Ev = { kind: string; text?: string; data?: any };
type State = {
  model?: string; provider?: string; mode?: string;
  spent_usd?: number; cap_usd?: number; ctx_frac?: number; running?: boolean;
};
type Item =
  | { t: "you"; text: string }
  | { t: "assistant"; text: string }
  | { t: "tool"; text: string }
  | { t: "result"; text: string }
  | { t: "error"; text: string }
  | { t: "end" }
  | { t: "approval"; id: string; kind: string; target: string; answered?: "allowed" | "declined" };

// Gauge: a semantic bar (green → amber → coral) that fills by level — the budget
// and context meters turn red as they approach a cap, mirroring the CLI HUD.
function Gauge({ frac }: { frac: number }) {
  const f = Math.max(0, Math.min(1, frac || 0));
  const color = f < 0.6 ? "#36c08a" : f < 0.85 ? "#ebb950" : "#ff5a4d";
  return (
    <div className="h-1.5 w-16 rounded-full bg-line overflow-hidden">
      <div className="h-full rounded-full" style={{ width: `${f * 100}%`, background: color }} />
    </div>
  );
}

function Hud({ s }: { s: State }) {
  const cap = s.cap_usd || 0;
  return (
    <header className="flex items-center gap-4 px-4 h-12 border-b border-line text-sm">
      <span className="font-mono font-bold">cli<span className="text-accent">che</span> studio</span>
      <span className="flex-1" />
      {s.mode && <span className="text-mut font-mono text-xs uppercase tracking-wide">{s.mode}</span>}
      {s.model && <span className="text-mut font-mono text-xs">{s.model}</span>}
      <span className="font-mono text-xs">${(s.spent_usd || 0).toFixed(4)}</span>
      {cap > 0 && (
        <span className="flex items-center gap-2">
          <Gauge frac={(s.spent_usd || 0) / cap} />
          <span className="text-mut font-mono text-xs">{Math.round((100 * (s.spent_usd || 0)) / cap)}%</span>
        </span>
      )}
      {(s.ctx_frac || 0) > 0 && (
        <span className="flex items-center gap-2">
          <span className="text-mut font-mono text-xs">ctx</span>
          <Gauge frac={s.ctx_frac || 0} />
        </span>
      )}
      {s.running && <span className="text-accent text-xs animate-pulse">● working</span>}
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
        <span className="text-mut text-xs">{it.answered}</span>
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
  const feedRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight });
  }, [items]);

  useEffect(() => {
    fetch("/api/state").then((r) => r.json()).then(setState).catch(() => {});
    const es = new EventSource("/api/events");
    es.onmessage = (m) => {
      const e: Ev = JSON.parse(m.data);
      setItems((prev) => reduce(prev, e));
      if (e.kind === "state" && e.data) setState(e.data);
    };
    return () => es.close();
  }, []);

  function answer(id: string, allow: boolean) {
    fetch("/api/approve", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, allow }) });
    setItems((prev) => prev.map((it) => (it.t === "approval" && it.id === id ? { ...it, answered: allow ? "allowed" : "declined" } : it)));
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    const p = prompt.trim();
    if (!p || state.running) return;
    setItems((prev) => [...prev, { t: "you", text: p }]);
    setPrompt("");
    const r = await fetch("/api/prompt", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt: p }) });
    if (!r.ok) setItems((prev) => [...prev, { t: "error", text: r.status === 409 ? "a run is already in progress" : `request failed (${r.status})` }]);
  }

  return (
    <div className="flex h-full flex-col">
      <Hud s={state} />
      <div ref={feedRef} className="flex-1 overflow-auto px-4 py-4 font-mono text-[13.5px] leading-relaxed">
        {items.length === 0 && (
          <div className="mt-20 text-center text-mut">
            <div className="text-2xl mb-2">Build something.</div>
            <div className="text-sm">Describe what you want — a website, a script, a tool — and watch Cliche make it, safely.</div>
          </div>
        )}
        {items.map((it, i) => <Row key={i} it={it} onAnswer={answer} />)}
      </div>
      <form onSubmit={send} className="flex gap-2 border-t border-line p-3">
        <input
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Describe what you want to build…"
          autoFocus
          className="flex-1 rounded-xl border border-line bg-panel px-4 py-3 text-[15px] outline-none focus:border-accent"
        />
        <button disabled={state.running} className="rounded-xl bg-accent px-5 font-bold text-[#1a0c0a] disabled:opacity-50">Build</button>
      </form>
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

// reduce folds an SSE event into the item list — streamed deltas append to the
// current assistant block; everything else is its own row.
function reduce(prev: Item[], e: Ev): Item[] {
  switch (e.kind) {
    case "delta": {
      const last = prev[prev.length - 1];
      if (last && last.t === "assistant") {
        const copy = prev.slice(0, -1);
        return [...copy, { t: "assistant", text: last.text + (e.text || "") }];
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
