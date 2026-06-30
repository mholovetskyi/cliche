package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// scaffoldBackend writes a real backend into the app being built — by default a
// Supabase (hosted Postgres + auth) integration: a typed client, an environment
// template, a starter schema with row-level security, and a connect guide. It
// turns a static frontend into an app that can persist data and authenticate
// users — the piece that was missing for "build apps like Lovable". One approval
// covers the whole scaffold and existing files are never clobbered. It pairs with
// the Supabase MCP connector, which can then create the project and apply the
// schema without leaving Cliché.
func (e OSExecutor) scaffoldBackend(args map[string]string) Result {
	kind := strings.ToLower(strings.TrimSpace(firstNonEmpty(args["kind"], args["provider"], "supabase")))
	dir := strings.TrimSpace(firstNonEmpty(args["dir"], args["path"], "."))
	if kind != "supabase" {
		return Result{Output: "scaffold_backend: only kind=\"supabase\" is supported for now (hosted Postgres + auth).", Success: false}
	}

	ts := e.looksTypeScript(dir)
	files := supabaseScaffold(ts)
	names := make([]string, 0, len(files))
	for rel := range files {
		names = append(names, rel)
	}
	sort.Strings(names)

	if e.ruleDecision("write", dir) == ruleDeny {
		return Result{Output: "blocked by deny rule: write " + dir, IsEdit: true, Success: false}
	}
	if !e.permit("write", dir, "write", "scaffold_backend (supabase) — creates: "+strings.Join(names, ", ")) {
		return Result{Output: "permission denied: scaffold_backend", IsEdit: true, Success: false}
	}

	var wrote, skipped []string
	for _, rel := range names {
		p, err := e.resolve(filepath.Join(dir, rel))
		if err != nil {
			return Result{Output: "scaffold_backend denied: " + err.Error(), IsEdit: true, Success: false}
		}
		if _, err := os.Stat(p); err == nil {
			skipped = append(skipped, rel) // never clobber an existing file (e.g. a real .env.local)
			continue
		}
		if d := filepath.Dir(p); d != "" {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return Result{Output: "scaffold_backend error: " + err.Error(), IsEdit: true, Success: false}
			}
		}
		if err := os.WriteFile(p, []byte(files[rel]), 0o644); err != nil {
			return Result{Output: "scaffold_backend error: " + err.Error(), IsEdit: true, Success: false}
		}
		e.Journal.record(p, "", false)
		wrote = append(wrote, rel)
	}

	lang := "JavaScript"
	if ts {
		lang = "TypeScript"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Scaffolded a Supabase backend (%s) in %q.\n", lang, dir)
	if len(wrote) > 0 {
		fmt.Fprintf(&b, "Created: %s\n", strings.Join(wrote, ", "))
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&b, "Left untouched (already exist): %s\n", strings.Join(skipped, ", "))
	}
	b.WriteString("\nNext steps (do these now):\n")
	b.WriteString("1. Install the client:  npm install @supabase/supabase-js\n")
	b.WriteString("2. Import { supabase } from the new client module and use it for data/auth in your components.\n")
	b.WriteString("3. Connect a project: create one at supabase.com (or use the Supabase MCP if it's connected), then put its URL + anon key in .env.local (copy from .env.example). Apply supabase/schema.sql in the SQL editor or via the Supabase MCP's apply_migration.\n")
	b.WriteString("Note: the client reads Vite-style import.meta.env.VITE_SUPABASE_*. For Next.js, switch to process.env.NEXT_PUBLIC_SUPABASE_* (see BACKEND.md).\n")
	return Result{Output: b.String(), IsEdit: true, Success: true}
}

// looksTypeScript reports whether the app at dir (relative to the root) is a
// TypeScript project, so the scaffold matches its language.
func (e OSExecutor) looksTypeScript(dir string) bool {
	if p, err := e.resolve(filepath.Join(dir, "tsconfig.json")); err == nil {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	if p, err := e.resolve(filepath.Join(dir, "package.json")); err == nil {
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "typescript") {
			return true
		}
	}
	return false
}

// supabaseScaffold returns the file set for a Supabase integration, keyed by path
// relative to the app dir. The client extension follows the project's language.
func supabaseScaffold(ts bool) map[string]string {
	ext := "js"
	annUrl, annKey := "", ""
	if ts {
		ext = "ts"
		annUrl, annKey = " as string", " as string"
	}
	client := "import { createClient } from '@supabase/supabase-js'\n\n" +
		"// Configured from environment so keys never live in source. Set these in\n" +
		"// .env.local (see .env.example); the anon key is safe for the browser.\n" +
		fmt.Sprintf("const url = import.meta.env.VITE_SUPABASE_URL%s\n", annUrl) +
		fmt.Sprintf("const anonKey = import.meta.env.VITE_SUPABASE_ANON_KEY%s\n\n", annKey) +
		"if (!url || !anonKey) {\n" +
		"  console.warn('Supabase env vars missing — set VITE_SUPABASE_URL and VITE_SUPABASE_ANON_KEY in .env.local')\n" +
		"}\n\n" +
		"export const supabase = createClient(url ?? '', anonKey ?? '')\n"

	return map[string]string{
		"src/lib/supabaseClient." + ext: client,
		".env.example": "# Supabase — copy to .env.local and fill in from your project's API settings\n" +
			"VITE_SUPABASE_URL=your-project-url\n" +
			"VITE_SUPABASE_ANON_KEY=your-anon-public-key\n",
		"supabase/schema.sql": "-- Starter schema. Apply in the Supabase SQL editor, or via the Supabase\n" +
			"-- MCP connector's apply_migration. Adjust to your app's data model.\n" +
			"create table if not exists items (\n" +
			"  id uuid primary key default gen_random_uuid(),\n" +
			"  title text not null,\n" +
			"  created_at timestamptz not null default now()\n" +
			");\n\n" +
			"-- Row-level security on by default; open up only what each role needs.\n" +
			"alter table items enable row level security;\n" +
			"create policy \"items are readable by everyone\" on items\n" +
			"  for select using (true);\n" +
			"create policy \"authenticated users can insert items\" on items\n" +
			"  for insert with check (auth.role() = 'authenticated');\n",
		"BACKEND.md": "# Backend (Supabase)\n\n" +
			"This app uses [Supabase](https://supabase.com) for its database and auth.\n\n" +
			"## Connect\n" +
			"1. Create a project at supabase.com (free tier is fine), or use the Supabase MCP connector from Cliché.\n" +
			"2. Copy `.env.example` to `.env.local` and fill in the project URL and anon key (Project Settings → API).\n" +
			"3. Apply `supabase/schema.sql` in the SQL editor (or via the MCP's `apply_migration`).\n\n" +
			"## Use\n" +
			"```ts\n" +
			"import { supabase } from './lib/supabaseClient'\n" +
			"const { data, error } = await supabase.from('items').select('*')\n" +
			"```\n\n" +
			"## Next.js\n" +
			"This client reads Vite's `import.meta.env`. On Next.js, read `process.env.NEXT_PUBLIC_SUPABASE_URL` / `...ANON_KEY` instead and prefix the env vars with `NEXT_PUBLIC_`.\n",
	}
}
