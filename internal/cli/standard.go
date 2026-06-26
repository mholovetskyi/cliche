package cli

import "strings"

// baseStandard raises the floor for every run (it lands in the cached system
// block, so the token cost is largely one-time). It nudges toward production
// quality without forcing a heavyweight process onto small fixes.
const baseStandard = "\nStandard of work — aim for production quality, not throwaway demos:\n" +
	"- Match the project's existing stack, structure, and conventions. When starting something new, scaffold a real, modern setup rather than one giant single-file script.\n" +
	"- Prefer typed, modular, reusable code over one huge file; name things well and keep functions small and focused.\n" +
	"- For any UI, make it accessible (semantic HTML, labels, keyboard support, adequate contrast) and responsive, with a consistent visual system (a type scale, spacing, and color tokens) — never unstyled browser defaults.\n" +
	"- Add or update tests for behavior you change, and run the project's build / typecheck / linter / tests; read the output and fix what it reports before you call the work done.\n" +
	"- Handle errors and edge cases. Don't leave TODOs, stubs, or placeholder lorem-ipsum in the path you were asked to deliver."

// productStandard is the full senior-engineer bar applied in product mode
// (--pro / Studio). It pushes real planning, a real stack, vertical-slice
// iteration, design tokens, and a hard quality gate the agent must actually run.
const productStandard = "\n\nPRODUCT BUILD MODE — you are building a world-class product, not a sample. Hold a senior-engineer bar:\n" +
	"1. PLAN FIRST. State the goal, the stack you'll use, and a short milestone list before writing code. Pick a real, modern stack for the task (for a web app, e.g. Vite or Next + TypeScript + a styling system + a component approach) and scaffold it with proper tooling: a package manager, typechecking, linting, formatting, and a test runner.\n" +
	"2. BUILD IN VERTICAL SLICES. Get a thin end-to-end version working and verified first, then iterate. After each slice, run the build/typecheck/tests and fix failures before moving on.\n" +
	"3. DESIGN LIKE A PRODUCT. Define design tokens (color, type scale, spacing, radius) up front and use them everywhere. Aim for a polished, cohesive, responsive, accessible UI with real empty / loading / error states, sensible defaults, and thoughtful microcopy.\n" +
	"4. ENGINEER FOR REAL USE. Validate inputs, handle errors and edge cases, keep secrets out of code, and structure things so the codebase can grow. Write a short README and meaningful tests.\n" +
	"5. QUALITY GATE — do NOT call it done until every check below passes and you have RUN the commands to prove it: the build succeeds, typecheck is clean, the linter is clean, tests pass, and the app actually runs. Then self-review the result against this list and fix any gap. If a check genuinely cannot run in this environment, say so explicitly instead of assuming it passes.\n" +
	"Budget your turns and spend for a substantial build; if you genuinely need a higher cap to finish properly, ask rather than shipping something half-built."

// proStandard returns the full product-mode bar when pro is set, else nothing.
func proStandard(pro bool) string {
	if pro {
		return productStandard
	}
	return ""
}

// proModels is the strong default model per provider for product builds — used
// only to upgrade a weak/budget model the user hasn't explicitly pinned. Every id
// is a real entry in the pricing table.
var proModels = map[string]string{
	"anthropic":  "claude-sonnet-4-6",
	"openrouter": "anthropic/claude-sonnet-4.6",
	"openai":     "gpt-5",
	"google":     "gemini-2.5-pro",
}

// weakModel reports whether a model id looks like a small/budget tier that can't
// carry a world-class build. It matches DELIMITED tokens (so "gemini" isn't
// mistaken for "mini") plus a billions-parameter suffix like "8b".
var weakTokens = map[string]bool{"mini": true, "haiku": true, "flash": true, "small": true, "lite": true, "nano": true, "tiny": true}

func weakModel(m string) bool {
	m = strings.ToLower(m)
	if m == "" {
		return true
	}
	toks := strings.FieldsFunc(m, func(r rune) bool { return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') })
	for _, tk := range toks {
		if weakTokens[tk] {
			return true
		}
		if len(tk) >= 2 && tk[len(tk)-1] == 'b' { // size suffix: 7b, 8b, 13b, 70b…
			allDigit := true
			for _, c := range tk[:len(tk)-1] {
				if c < '0' || c > '9' {
					allDigit = false
					break
				}
			}
			if allDigit {
				return true
			}
		}
	}
	return false
}

// qualityModel upgrades a weak model to the provider's strong default for a
// product build. It only fires for a known cloud provider and only when the
// current model is a budget tier — a capable model (or a local/unknown provider,
// or an explicitly pinned model handled by the caller) is left untouched.
func qualityModel(provider, current string) (string, bool) {
	strong, ok := proModels[provider]
	if !ok || current == strong {
		return current, false
	}
	if weakModel(current) {
		return strong, true
	}
	return current, false
}
