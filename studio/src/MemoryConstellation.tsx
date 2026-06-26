import { useEffect, useRef, useState } from "react";
import { REDUCE } from "./lib/reduced";

// Memory Constellation: your past sessions as a drifting star-map you fly through
// and click to revisit. Deterministic polar layout (hash(id)→angle, recency→ring),
// star size ← message count, chronological links. Hit-test runs against the actual
// drawn screen positions (no DPR/transform guesswork). Data = /api/sessions.
type S = { id: string; title: string; model: string; updated: string; messages: number };
function hash(s: string) { let h = 2166136261; for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 16777619); } return h >>> 0; }

export default function MemoryConstellation({ sessions, relTime, onPick, onClose }: { sessions: S[]; relTime: (s: string) => string; onPick: (id: string) => void; onClose: () => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const [hover, setHover] = useState<{ x: number; y: number; s: S } | null>(null);
  const pickRef = useRef<S | null>(null);
  useEffect(() => {
    const cv = ref.current; if (!cv) return;
    const ctx = cv.getContext("2d"); if (!ctx) return;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const root = document.documentElement;
    const accent = () => getComputedStyle(root).getPropertyValue("--accent").trim() || "#ff6a4d";
    let W = 0, H = 0;
    const ro = new ResizeObserver(() => { W = cv.clientWidth; H = cv.clientHeight; cv.width = W * dpr; cv.height = H * dpr; ctx.setTransform(dpr, 0, 0, dpr, 0, 0); });
    ro.observe(cv);
    const n = sessions.length;
    const nodes = sessions.map((s, i) => { const hh = hash(s.id); return { s, ang: (hh % 360) * Math.PI / 180, ring: 0.16 + (n > 1 ? i / (n - 1) : 0) * 0.74, size: 1.6 + Math.min(5, (s.messages || 1) * 0.5), tw: (hh % 628) / 100 }; });
    let mx = -1, my = -1, hoverId: string | null = null;
    const onMove = (e: PointerEvent) => { const r = cv.getBoundingClientRect(); mx = e.clientX - r.left; my = e.clientY - r.top; };
    cv.addEventListener("pointermove", onMove);
    cv.addEventListener("pointerleave", () => { mx = -1; my = -1; });
    let raf = 0, last = 0;
    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (!REDUCE.matches && t - last < 32) return; last = t;
      ctx.clearRect(0, 0, W, H);
      const cx = W / 2, cy = H / 2, R = Math.min(W, H) * 0.42, col = accent();
      const drift = REDUCE.matches ? 0 : t * 0.00004;
      const pts = nodes.map((nd) => ({ x: cx + Math.cos(nd.ang + drift) * nd.ring * R, y: cy + Math.sin(nd.ang + drift) * nd.ring * R, r: nd.size, s: nd.s, tw: nd.tw }));
      ctx.strokeStyle = "rgba(255,255,255,.05)"; ctx.lineWidth = 1;
      for (let i = 0; i < pts.length - 1; i++) { ctx.beginPath(); ctx.moveTo(pts[i].x, pts[i].y); ctx.lineTo(pts[i + 1].x, pts[i + 1].y); ctx.stroke(); }
      let near: typeof pts[0] | null = null, best = 24 * 24;
      ctx.globalCompositeOperation = "lighter";
      for (const p of pts) {
        const tw = REDUCE.matches ? 1 : 0.6 + 0.4 * Math.sin(t * 0.002 + p.tw);
        ctx.fillStyle = col;
        ctx.globalAlpha = 0.55 * tw; ctx.beginPath(); ctx.arc(p.x, p.y, p.r, 0, 6.283); ctx.fill();
        ctx.globalAlpha = 0.16 * tw; ctx.beginPath(); ctx.arc(p.x, p.y, p.r * 3.2, 0, 6.283); ctx.fill();
        if (mx >= 0) { const dd = (p.x - mx) ** 2 + (p.y - my) ** 2; if (dd < best) { best = dd; near = p; } }
      }
      ctx.globalAlpha = 1; ctx.globalCompositeOperation = "source-over";
      const id = near ? near.s.id : null;
      pickRef.current = near ? near.s : null;
      if (id !== hoverId) { hoverId = id; setHover(near ? { x: near.x, y: near.y, s: near.s } : null); }
    };
    raf = requestAnimationFrame(frame);
    return () => { cancelAnimationFrame(raf); ro.disconnect(); cv.removeEventListener("pointermove", onMove); };
  }, [sessions]);
  return (
    <div className="fade-in fixed inset-0 z-[120] bg-black/70 backdrop-blur-sm" onClick={onClose}>
      <canvas ref={ref} className="absolute inset-0 h-full w-full" onClick={(e) => { e.stopPropagation(); if (pickRef.current) { onPick(pickRef.current.id); onClose(); } }} />
      <div className="pointer-events-none absolute left-1/2 top-8 -translate-x-1/2 text-center">
        <div className="text-sm font-semibold tracking-tight">Memory</div>
        <div className="text-xs text-[var(--mut)]">your past sessions · click a star to revisit · esc to close</div>
      </div>
      {hover && (
        <div className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full" style={{ left: hover.x, top: hover.y - 12 }}>
          <div className="glass elev rounded-lg border border-[var(--line2)] px-3 py-2 text-[12px]">
            <div className="font-medium">{hover.s.title || "New chat"}</div>
            <div className="text-[var(--dim)]">{relTime(hover.s.updated)} · {hover.s.messages} msgs · {hover.s.model}</div>
          </div>
        </div>
      )}
    </div>
  );
}
