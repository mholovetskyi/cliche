# Control plane — architecture & build order

> Status: **spec**. The free CLI ships today. This document defines the
> commercial layer ([COMMERCIAL.md](../COMMERCIAL.md)) and the CLI-side hooks it
> needs. The backend is a **separate, closed-source service**; only the hooks
> described under "CLI side" live in this repo.

## The shape

```
        free, Apache-2.0, in this repo            commercial, separate repo
   ┌──────────────────────────────────────┐   ┌──────────────────────────────┐
   │  cliche CLI                           │   │  control plane (hosted /      │
   │  ─ Trust Kernel (local enforcement)   │   │  self-hosted)                 │
   │  ─ signed + hash-chained ledger       │   │  ─ policy store (signed)      │
   │                                       │   │  ─ ledger aggregation + search│
   │   --org hook:                         │   │  ─ spend dashboard            │
   │     1. fetch signed org policy  ◀─────┼───┼──  GET /policy (ed25519)      │
   │     2. verify vs pinned org key       │   │                               │
   │     3. TIGHTEN-ONLY merge into config │   │                               │
   │     4. ship signed ledger heads ──────┼───┼─▶  POST /ledger (verify chain)│
   └──────────────────────────────────────┘   └──────────────────────────────┘
```

The CLI never trusts the control plane to *loosen* anything: a policy can only
add restrictions, and the kernel's existing invariant holds — `--yolo` skips
approvals but never bypasses budget, governor, deny, plan mode, egress, or
hooks. A compromised/rogue control plane can at worst over-restrict (a DoS the
admin would notice), never silently disarm a guardrail.

## CLI side (this repo)

### Configuration

```jsonc
// .cliche/config.json  (or global config)
{
  "org": {
    "policy_url": "https://cp.example.com/policy",   // or "policy_file": "..."
    "key": "ed25519:BASE64PUBKEY"                     // pinned org public key
  }
}
```
Also settable via `CLICHE_ORG_POLICY_URL` / `CLICHE_ORG_KEY`. `cliche login --org`
walks an admin through pinning the key.

### Load + verify (fail-closed)

When an `org` source is configured, on every run:
1. Fetch the policy document (HTTP `policy_url`, or read `policy_file`).
2. Verify its ed25519 signature against the **pinned** `key`. Reuse
   `internal/seal` verification.
3. **Fail closed.** If a source is configured but the policy can't be fetched or
   the signature doesn't verify, **refuse to run** with a clear error. A
   governance tool must never run *unpoliced* when policy was expected. (A cached
   last-good policy with a max-age is a later refinement for offline use.)

### Tighten-only merge

The policy document carries only fields that can restrict. The merge is
**monotonic toward tighter** — applied to a looser local value it tightens;
applied to an already-tighter local value it is a no-op. This is the
security-critical core and gets adversarial tests ("no field can ever loosen").

| Policy field            | Local field            | Merge rule                                   | Tighter means |
|-------------------------|------------------------|----------------------------------------------|---------------|
| `deny: []`              | `permissions.deny`     | **union** (org ∪ local)                      | more denied   |
| `egress_allow: []`      | `egress.allow`         | **intersect**; if local empty (=all) → use org | fewer hosts   |
| `force_sandbox: bool`   | `policy.sandbox`       | OR (true wins)                               | sandbox on    |
| `max_usd`               | `budget.max_usd`       | **min** of the set (non-zero) values         | lower cap     |
| `max_tokens`            | `budget.max_tokens`    | **min** of the set values                    | lower cap     |
| `max_turns`             | `governor.max_turns`   | **min** of the set values                    | shorter       |
| `max_wallclock_seconds` | `governor.…`           | **min** of the set values                    | shorter       |
| `forbid_yolo: bool`     | the `--yolo` flag      | if true, reject `--yolo` (approvals forced)  | approvals on  |
| `require_hook` (later)  | `hooks.pre_tool_use`   | require a hook is present                    | gated         |

Notably the policy **cannot grant `allow` rules** — it only adds `deny`, which
already wins over allow and over `--yolo`. So a policy can never widen authority.

### Ledger shipping (telemetry)

After a run, `POST` the **signed ledger head** (and per-model token/$ totals)
to the control plane. Heads are already ed25519-signed (`ledger.seal.json`), so
the server verifies authenticity and chain continuity without trusting the
client. Opt-in, and a no-op when no `org` source is configured.

## Backend (separate repo — out of scope here)

MVP endpoints: `GET /policy` (returns the signed doc), `POST /ledger` (verify +
store heads), a minimal web dashboard for **spend** and **audit search**
("what touched repo X"), and SSO. Pricing/tiers in
[COMMERCIAL.md](../COMMERCIAL.md).

## Build order

1. **`internal/orgpolicy`** — the document type + the tighten-only `Apply(cfg,
   policy) cfg` merge, with adversarial "never loosens" tests. *(Pure, no
   network — buildable and fully testable in this repo today.)*
2. **Fetch + verify** wiring in `buildAgent`/`buildSwarmRunner`, fail-closed.
3. **`cliche org`** — `cliche org show` (effective policy + what it tightened),
   `cliche login --org` (pin the key).
4. **Ledger head POST** (opt-in).
5. Backend MVP (separate repo): policy store, ledger ingest, dashboard.

Step 1 is the seed: it is useful **even without the backend** — a team lead can
commit a signed `policy.json` and every `cliche` enforces it.
