import { useEffect, useRef } from "react";
import { REDUCE } from "./lib/reduced";

// Depth Aurora: a parallax starfield the whole UI floats above. ~140 motes with
// a z depth, pinhole-projected; the cursor shifts deep motes more than near ones
// → real 3D. On a tool_result a bright mote rushes from the deep toward the
// camera — a receipt surfacing. Canvas-2D, additive, ~30fps, capped, hidden-safe.
export default function DepthField({ bind }: { bind: (fn: (kind: string) => void) => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const cv = ref.current; if (!cv) return;
    const ctx = cv.getContext("2d"); if (!ctx) return;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const root = document.documentElement;
    const accent = () => (getComputedStyle(root).getPropertyValue("--accent").trim() || "#ff6a4d");
    let W = innerWidth, H = innerHeight;
    const resize = () => { W = innerWidth; H = innerHeight; cv.width = W * dpr; cv.height = H * dpr; ctx.setTransform(dpr, 0, 0, dpr, 0, 0); };
    resize(); window.addEventListener("resize", resize);
    const N = REDUCE.matches ? 90 : 140;
    const motes = Array.from({ length: N }, () => ({ x: Math.random(), y: Math.random(), z: Math.random(), tw: Math.random() * 6.28, surge: 0 }));
    let px = 0, py = 0;
    const onMove = (e: PointerEvent) => { px = (e.clientX / W - 0.5); py = (e.clientY / H - 0.5); };
    window.addEventListener("pointermove", onMove);
    bind((kind) => { if (kind !== "tool_result") return; const m = motes[Math.floor(Math.random() * motes.length)]; m.z = 0.02; m.surge = 1; });
    const focal = 1.0, spread = 0.85;
    let raf = 0, last = 0;
    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (document.hidden) { ctx.clearRect(0, 0, W, H); return; }
      if (!REDUCE.matches && t - last < 32) return;
      last = t;
      const act = parseFloat(getComputedStyle(root).getPropertyValue("--activity")) || 0;
      ctx.clearRect(0, 0, W, H);
      ctx.globalCompositeOperation = "lighter";
      const col = accent();
      for (const m of motes) {
        if (!REDUCE.matches) { m.z += (0.0006 + act * 0.0016) + (m.surge ? 0.02 : 0); if (m.z > 1) m.z -= 1; if (m.surge) m.surge = Math.max(0, m.surge - 0.02); }
        const scale = focal / (focal - m.z * spread);
        const sx = W / 2 + (m.x - 0.5) * W * scale + px * m.z * 90;
        const sy = H / 2 + (m.y - 0.5) * H * scale + py * m.z * 90;
        const r = (0.5 + m.z * 2.2) * scale + m.surge * 3;
        const tw = REDUCE.matches ? 0.6 : 0.45 + 0.25 * Math.sin(t * 0.002 + m.tw);
        ctx.globalAlpha = Math.min(0.7, (0.12 + m.z * 0.5) * tw + m.surge * 0.5);
        ctx.fillStyle = m.surge > 0.3 ? col : (m.z > 0.6 ? col : "#6f5cff");
        ctx.beginPath(); ctx.arc(sx, sy, r, 0, 6.283); ctx.fill();
      }
      ctx.globalAlpha = 1; ctx.globalCompositeOperation = "source-over";
    };
    raf = requestAnimationFrame(frame);
    return () => { cancelAnimationFrame(raf); window.removeEventListener("resize", resize); window.removeEventListener("pointermove", onMove); };
  }, []);
  return <canvas ref={ref} className="depth" aria-hidden="true" />;
}
