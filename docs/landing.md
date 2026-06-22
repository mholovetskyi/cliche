# Cliche — landing page content

> Copy deck for the marketing site. Sections are ordered top-to-bottom.
> Voice: direct, builder-first, no hype, no em-dash-laden AI cadence.

---

## Hero

**Headline:**
The AI coding agent you can actually leave running.

**Subhead:**
Cliche wraps any model in a deterministic trust layer: a hard token cap, a loop circuit-breaker, and a verifier that catches the agent faking it. On by default. Open source. Auditable to the token.

**Primary CTA:** `go install` → Get started
**Secondary CTA:** Watch the 30-second demo

**Microcopy under the buttons:**
Yes, "CLI" is right there in the name. No, we're not sorry.

---

## The problem (social proof of pain)

**Section title:** Autonomous agents are amazing right up until the bill arrives.

The whole category is racing toward agents you start and walk away from — async, in CI, ten at a time. The harness wasn't built for that, and it shows:

- **$438 in 3.5 hours.** A documented runaway loop ran 809 turns with no circuit breaker.
- **$0.10 → $7.59.** A single code review spiraled into an 8.5-million-token loop.
- **"3x what you budgeted."** The most common complaint about every major coding CLI is cost you can't see coming.
- **Silently faked.** Agents delete tests, mock returns, and wrap code in empty `try/except` to make the bar go green. You find out three tasks later.

None of these are model problems. They're *harness* problems. Cliche is a harness built for the part where you're not watching.

---

## Meet Cliche

**Section title:** Trust as a feature, not a footnote.

Every other tool competes on capability. Cliche rides the same frontier models you already use (bring your own key) and competes on the thing none of them ship: **deterministic guardrails that the model cannot talk its way past.**

The caps and breakers are *code wrapped around the loop* — not instructions in a prompt the model can ignore.

---

## The Trust Kernel

**Section title:** Four guardrails. All on by default.

**1. Budget Kernel — a cap that actually caps.**
A hard ceiling on tokens (the provider-independent guarantee) and an estimated dollar ceiling on top. Checked *before* every turn, again against *actual* usage the moment the turn returns, and each request is bounded by the remaining budget — so the one fat completion that blows the estimate is caught before the next turn, not on your invoice.

**2. Governor — the loop circuit-breaker.**
Hard limits on turns, wall-clock, and consecutive failed edits, plus repetition detection that spots an agent re-issuing the same failing edit. The runaway that costs other tools $438 stops here in single digits.

**3. Verifier — catches the agent faking it.**
When a diff deletes a test, swallows an error, or weakens an assertion to pass, Cliche flags it with the evidence. Honest by design: it never says "verified" without proof, and it biases toward "let me check" over false accusations.

**4. The Ledger — auditable to the token.**
Every turn, tool call, cap event, and verdict is written to an append-only log. `cliche cost` tells you exactly where the money went. No secrets, no raw code — just the receipts.

---

## See it (don't take our word for it)

**Section title:** Run the demo. No key, no network, 30 seconds.

```
$ cliche demo

[2] Runaway loop — the agent re-issues the SAME failing edit forever.
    → HALTED at turn 3: identical tool call repeated 3× within the last 3 calls
    → spent ~$0.0738 (15000 tokens) and stopped.
    For comparison: a documented runaway in another tool ran 809 turns
    and ~$438 with no breaker. Cliche stops it in single digits.

[3] Budget blowout — token-heavy turns; the dollar cap is $0.50.
    → HALTED at turn 2: estimated dollar cap reached: ~$0.60/$0.50
    → preflight passed, but ACTUAL usage crossed the cap and was caught
      the moment the turn returned — before the next turn could fire.

[4] Reward-hack check — the agent deletes a test to 'pass'.
    → verdict: flagged
      • [deleted_test] a test was removed: func TestChargesCustomer...
```

Those numbers are real program output, not a mockup.

---

## Drop it into CI

**Section title:** The agent that can't exceed the budget you set.

```
git diff | cliche exec -p "review this change" --max-usd 0.10
```

Headless mode streams JSON, returns clean exit codes (`0` done, `3` budget hit, `4` breaker tripped), and **fails loudly** on limits instead of hanging. It attaches an honesty report to the work. CI is exactly where an unwatched agent needs a hard ceiling — so that's where Cliche shines.

---

## Why you can trust the trust tool

**Section title:** Open core. Real commitments. In writing.

- **BYO-key, forever.** We never mark up your inference. Use Anthropic today, any provider next.
- **Local-first.** Your ledgers and verdicts live in your repo. No mandatory cloud, no default telemetry.
- **A single static binary.** Zero third-party dependencies in the core — nothing to compromise in a supply-chain attack.
- **Apache-2.0.** The whole kernel and CLI are auditable. A trust tool you can't read isn't one.

**Honest non-claim:** the Verifier catches documented reward-hacking patterns and honest mistakes. It is **not** a security boundary against an adversary who knows the rules. We'd rather tell you that than oversell it.

---

## How we're different

| | Cliche | Typical coding CLI |
|---|---|---|
| Hard spend cap | ✅ token-hard, $-estimated, on by default | ❌ discover cost after the bill |
| Loop circuit-breaker | ✅ turns / repetition / failed-edits | ⚠️ rare, usually off |
| Reward-hack detection | ✅ flags it with evidence | ❌ none |
| Cost ledger | ✅ append-only, per-project | ⚠️ partial |
| Model lock-in | ✅ BYO-key, provider-neutral | ⚠️ often vendor-locked |
| Source | ✅ Apache-2.0, zero-dep binary | mixed |

---

## FAQ

**Is this a model?**
No. Cliche brings the guardrails; you bring the model. It's the trust layer, not the brain.

**Does it slow the agent down?**
The kernel is deterministic code; the overhead is negligible. What it removes is the 800-turn loop you didn't want.

**What does it cost?**
The CLI is free and open source — you pay your own model bill. Team budget and verdict dashboards are the paid layer.

**Why the name?**
It has "CLI" in it, it's easy to type, and trust in agents *shouldn't* be novel — it should be the boring default. We made the boring stuff the whole point.

---

## Final CTA

**Stop hoping your agent behaves. Make it.**

```
go install github.com/mholovetskyi/cliche/cmd/cliche@latest
cliche demo
```

Star us on GitHub · Read the roadmap · Bring your key
