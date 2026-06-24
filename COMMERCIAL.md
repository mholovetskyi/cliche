# Cliche — open core, commercial trust layer

**The CLI is free and Apache-2.0, forever.** Every guardrail in the Trust
Kernel — hard spend caps, the runaway governor, the signed audit ledger, the
egress allowlist, deny rules, plan mode, hooks — is in the open-source core and
always will be. Bring your own key; we never touch your tokens or your code.

What we sell is the **multi-user expression of that trust layer**: the things a
*team* needs once more than one person is running agents against a shared
codebase, and someone is accountable for what those agents do.

> **🚧 Pre-order — the Team control plane is in development.** The free CLI and the
> open policy client (`cliche org`) ship today; the hosted backend is being built.
> If your team would use this, **[register interest](https://github.com/mholovetskyi/cliche/issues/new?template=team-interest.yml)**
> (or email below). Founding customers get **locked-in early pricing** and shape
> the roadmap. No payment is taken now — this is to gauge interest before launch.

> Every other coding CLI competes on capability. Cliche competes on
> **guardrails the model cannot argue its way past.** The commercial product is
> those same guardrails, enforced and proven **across your whole org.**

---

## Tiers

### Free — the CLI (Apache-2.0)

The complete agent and Trust Kernel, for individuals and open-source work:

- all ~25 providers, BYO key
- budget kernel, governor, verifier, sandbox, egress, hooks, deny rules
- **local** signed + hash-chained audit ledger (`cliche audit`)
- sessions, memory, MCP + connectors, skills, plugins, swarm, the TUI

This is the adoption engine. It is never crippled to push the paid tiers.

### Team — the control plane (per-seat)

A hosted (or self-hosted) control plane that turns the local Trust Kernel into
an org-wide one:

- **Policy push** — an admin defines deny rules, egress allowlists, budget
  caps, and required permission mode once; every developer's CLI fetches the
  signed policy and the Trust Kernel enforces it. It can only *tighten* local
  config, never loosen it, and `--yolo` still can't bypass it.
- **Audit aggregation** — every developer's already-signed ledger streams to one
  tamper-evident store. Ask "what did any agent do to repo X / file Y", verify
  the chain, export the answer.
- **Spend governance** — aggregate the Budget Kernel telemetry: spend by
  developer, repo, and model; cap alerts before the bill, not after.
- **Shared catalog** — distribute org-approved providers, MCP connectors,
  skills, and plugins to the whole team.
- SSO for the dashboard.

### Enterprise — (custom)

Everything in Team, plus what regulated and large orgs require:

- self-hosted / air-gapped control plane
- **compliance evidence** exported from the signed ledger (SOC 2 / ISO: prove
  what your AI agents did, cryptographically)
- SAML / SCIM, RBAC, retention policies
- egress-proxy and secret-scanning hook integration, custom verifier policies
- support SLA

---

## Why this split is honest

The Trust Kernel is **local-first and verifiable** on purpose. The paid product
doesn't gate that — it *aggregates and enforces* it across people. You can audit
your own runs for free, forever. You pay when you need to audit and govern
*everyone's*, and prove it to an auditor.

The design that makes this possible is already shipped: the ledger is
hash-chained and ed25519-signed (`internal/seal`, `cliche audit`), the kernel's
guardrails are code around the loop (not prompt text), and the open `orgpolicy`
client enforces a signed, tighten-only org policy (`cliche org`). The control
plane — the policy/billing/aggregation backend — is a separate hosted service.

---

## Pre-order / register interest

No payment is taken yet — we're gauging demand before opening billing. Founding
customers lock in early pricing and shape what ships first.

- **Register interest (1 min):** open a
  [Team-interest issue](https://github.com/mholovetskyi/cliche/issues/new?template=team-interest.yml)
  — team size, what you'd use it for, what you'd pay. Public, so it also signals
  demand to others.
- **Prefer not to post publicly?** Email **devawemykola@gmail.com** for a
  founding-customer slot (companies / Enterprise).
- **Sponsor the open-source core meanwhile:** GitHub Sponsors (Sponsor button).

Cliche is built by [@mholovetskyi](https://github.com/mholovetskyi).
