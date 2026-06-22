# Setting up Cliche

Cliche is **bring-your-own-key**: you point it at any model provider you already
pay for, and it wraps that model in a deterministic Trust Kernel (hard spend
caps, a loop breaker, an audit ledger, a reward-hack verifier). This page gets
you from zero to a running session in a couple of minutes.

---

## 1. Get the binary

Download a release for your platform from
[Releases](https://github.com/mholovetskyi/cliche/releases), or build from
source (Go 1.23+):

```sh
go build -o cliche ./cmd/cliche      # Windows: -o cliche.exe
```

Confirm it runs — this works offline, with no key:

```sh
cliche demo      # runs the Trust Kernel against four scenarios
```

---

## 2. Connect a provider (once)

Run the guided wizard — pick a provider, paste your key (hidden), Cliche
verifies it works and saves it to your per-user config (`0600`, never in the
repo):

```sh
cliche login
```

Built-in providers: **Anthropic, OpenRouter, OpenAI, Groq, DeepSeek, Mistral,
Together, xAI**. (Running `cliche chat` with no key configured drops you
straight into this wizard.)

**Where to get a key**

| Provider   | Get a key at                         |
|------------|--------------------------------------|
| Anthropic  | console.anthropic.com → API keys     |
| OpenRouter | openrouter.ai/keys (one key, many models) |
| OpenAI     | platform.openai.com/api-keys         |
| Groq       | console.groq.com/keys                |
| DeepSeek   | platform.deepseek.com/api_keys       |
| Mistral    | console.mistral.ai/api-keys          |

**Scriptable / CI alternative** (no prompt):

```sh
cliche auth openrouter --from-file path/to/key.txt   # or --key, or pipe on stdin
export OPENROUTER_API_KEY=sk-or-...                  # env always overrides a saved key
```

Check what's configured anytime with `cliche auth`.

### Any other provider (including local models)

Anything that speaks the **OpenAI-compatible** Chat Completions API works — add
it under `providers` in `.cliche/config.json` (see step 3), or point at it
ad-hoc:

```sh
# A local model via Ollama / LM Studio / vLLM:
cliche chat --provider local \
  --base-url http://localhost:11434/v1/chat/completions --model llama3.1
```

(For a local server the key is usually ignored; save any placeholder with
`cliche auth local --key local`.)

---

## 3. Scaffold your project (optional but recommended)

In your project root:

```sh
cliche init
```

This writes a default `.cliche/config.json` (the conservative caps, ready to
edit) and an `AGENTS.md` template. Wire up your real test command in the
`## verify` section of `AGENTS.md` so a "verified" verdict means **your** tests
actually passed.

A custom provider lives in the config like this:

```json
{
  "providers": [
    { "name": "ollama", "base_url": "http://localhost:11434/v1/chat/completions", "default_model": "llama3.1" }
  ]
}
```

---

## 4. Go

```sh
cliche chat                 # interactive session — type a task, watch it cook
```

Inside a session: `/model` switch models, `/mode` change permission mode,
`/diff` see changes, `/undo` revert the last edit, `/rewind` undo all edits this
session, `/commit` commit the work, `/cost` spend, `/verify` re-run tests,
`/help` for the rest. Resume later with `cliche chat --continue` (or `--resume
<id>`; list them with `cliche sessions`).

One-shot and headless:

```sh
cliche run --max-usd 0.50 --allow-write --verify "fix the failing test in ./api"
cliche run --branch --commit --allow-write "..."   # isolate on a cliche/<id> branch, commit the result
git diff | cliche exec -p "review this change" --max-usd 0.10   # JSON out, CI exit codes
```

Every run is capped, breaker-guarded, and logged to an append-only ledger. See
`cliche cost`, or `cliche report` for a Markdown verdict (task, cost, changes)
you can paste into a PR — `cliche report --pr 42` posts it via the GitHub CLI.

---

## 5. Tighten the leash (optional)

Everything below is opt-in, in `.cliche/config.json`. Defaults stay conservative.

**Permission mode** — `--mode plan` (read-only: it proposes, never touches the
disk), `suggest` (ask before each action, the default), `auto-edit` (auto-apply
edits, ask before commands), `full` (auto everything). `--yolo` skips approvals
but **never** the spend cap or the loop breaker.

**Allow/deny rules** — deterministic policy-as-code. Deny wins over allow and
overrides even `--yolo`:

```json
{
  "permissions": {
    "allow": ["Bash(go test *)", "Edit(src/**)"],
    "deny":  ["Read(**/.env)", "Bash(rm -rf *)"]
  }
}
```

**Network** — the agent can pull docs with `web_fetch` (gated by `--allow-web`).
Constrain which hosts it may reach:

```json
{ "egress": { "allow": ["pkg.go.dev", "*.github.com"] } }
```

**`--sandbox`** — one strict switch for a task you don't fully trust: confines to
the project root (overriding `--allow-outside-root`), denies network unless an
egress allowlist is set, and scrubs your provider API keys from any shell command
the agent runs. (Application-level isolation; kernel sandboxing is a roadmap item.)

**Hooks** — run your own command at lifecycle points; a non-zero `pre_tool_use`
exit *blocks* the tool (its output is the reason), making policy externally
enforceable:

```json
{ "hooks": { "pre_tool_use": "sh ./gate.sh", "stop": "make notify" } }
```

**Cheaper delegation** — let the strong model plan and route grunt work to a
cheaper one:

```json
{ "subagents": { "max_depth": 2, "model": "openai/gpt-4o-mini" } }
```

**Remote tools (MCP)** — stdio or Streamable-HTTP servers:

```json
{ "mcp": [
  { "name": "fs",  "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "."] },
  { "name": "docs", "url": "https://mcp.example.com/mcp" }
] }
```

Run `cliche config` to print and validate your effective configuration, and
`cliche map` to see the project overview the agent starts with.
