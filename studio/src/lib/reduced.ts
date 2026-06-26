// Live reduced-motion signal. CSS handles declarative animations; JS-driven rAF /
// Web Audio loops must read REDUCE.matches at each entry point (not once) so a
// mid-session OS preference change is honored.
export const REDUCE = window.matchMedia("(prefers-reduced-motion: reduce)");
export function reduced(): boolean {
  return REDUCE.matches;
}

// flag reads/writes for the opt-in "instrument" toggles (mirror cliche-accent).
export function flag(key: string): boolean {
  try { return localStorage.getItem(key) === "on"; } catch { return false; }
}
export function setFlag(key: string, on: boolean) {
  try { localStorage.setItem(key, on ? "on" : "off"); } catch { /* ignore */ }
}
