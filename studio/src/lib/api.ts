// api — the backend boundary. In the browser/desktop the UI is served by the same
// `cliche serve` it talks to, so the base is "" (same-origin, no token) and nothing
// changes. In the Capacitor mobile app the UI is loaded from capacitor://localhost,
// so it must target a REMOTE backend (the cloud gateway / a networked `cliche serve`)
// with a bearer token. Configure it once via the Connect screen; every request and
// the SSE stream then route through here.

const SRV = "cliche-server"; // remote backend base URL, e.g. https://gw.cliche.app
const TOK = "cliche-token";  // bearer token for that backend

export function apiBase(): string {
  try { return localStorage.getItem(SRV) || ""; } catch { return ""; }
}
export function apiToken(): string {
  try { return localStorage.getItem(TOK) || ""; } catch { return ""; }
}
export function setServer(base: string, token: string) {
  try {
    localStorage.setItem(SRV, base.replace(/\/+$/, ""));
    localStorage.setItem(TOK, token);
  } catch { /* ignore */ }
}
export function clearServer() {
  try { localStorage.removeItem(SRV); localStorage.removeItem(TOK); } catch { /* ignore */ }
}

// isApp reports whether we're running inside the native Capacitor shell.
export function isApp(): boolean {
  return typeof window !== "undefined" && !!(window as any).Capacitor;
}

// api is a drop-in for fetch: it prepends the configured backend base and attaches
// the bearer token. With no backend configured it's identical to a same-origin fetch.
export function api(path: string, opts: RequestInit = {}): Promise<Response> {
  const base = apiBase(), token = apiToken();
  const headers = new Headers(opts.headers || {});
  if (token) headers.set("Authorization", "Bearer " + token);
  return fetch(base + path, { ...opts, headers, credentials: base ? "include" : "same-origin" });
}

// saveFile persists an edit from the in-Studio editor. Throws with the server's
// message on rejection (e.g. a sensitive file or a path that escapes the project).
export async function saveFile(path: string, content: string): Promise<void> {
  const r = await api("/api/file/save", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, content }),
  });
  if (!r.ok) throw new Error((await r.text()).trim() || "save failed");
}

// sseUrl builds a URL for EventSource / direct navigation (downloads): these can't
// set an Authorization header, so the token rides as a query param.
export function sseUrl(path: string): string {
  const base = apiBase(), token = apiToken();
  if (!token) return base + path;
  return base + path + (path.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(token);
}
