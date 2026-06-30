import { useMemo } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { javascript } from "@codemirror/lang-javascript";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { json } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";

export interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  filename: string;
  readOnly?: boolean;
}

function languageFor(filename: string): Extension[] {
  const ext = filename.slice(filename.lastIndexOf(".") + 1).toLowerCase();
  switch (ext) {
    case "ts":
      return [javascript({ typescript: true })];
    case "tsx":
      return [javascript({ typescript: true, jsx: true })];
    case "js":
    case "mjs":
    case "cjs":
      return [javascript()];
    case "jsx":
      return [javascript({ jsx: true })];
    case "css":
      return [css()];
    case "html":
    case "htm":
      return [html()];
    case "json":
      return [json()];
    case "md":
    case "markdown":
      return [markdown()];
    default:
      return [];
  }
}

// Bridge CodeMirror to the app's CSS variables so the editor inherits the active
// (dark) theme. vscodeDark supplies the syntax token colors; this overlay re-skins
// the chrome (gutter, background, cursor, selection) to match Studio.
const studioChrome = EditorView.theme(
  {
    "&": {
      backgroundColor: "transparent",
      color: "var(--ink)",
      fontSize: "13px",
      height: "100%",
    },
    ".cm-content": {
      fontFamily: "var(--mono)",
      caretColor: "var(--accent)",
    },
    ".cm-gutters": {
      backgroundColor: "transparent",
      color: "var(--dim)",
      border: "none",
      borderRight: "1px solid var(--line)",
    },
    ".cm-activeLine": { backgroundColor: "rgba(255,255,255,0.03)" },
    ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--mut)" },
    "&.cm-focused .cm-cursor": { borderLeftColor: "var(--accent)" },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
      backgroundColor: "rgba(255,106,77,0.25)",
    },
  },
  { dark: true }
);

export default function CodeEditor({ value, onChange, filename, readOnly = false }: CodeEditorProps) {
  const extensions = useMemo(
    () => [...languageFor(filename), studioChrome, EditorView.lineWrapping],
    [filename]
  );

  return (
    <CodeMirror
      value={value}
      onChange={onChange}
      extensions={extensions}
      theme={vscodeDark}
      readOnly={readOnly}
      basicSetup={{ foldGutter: false, highlightActiveLine: true }}
      style={{ height: "100%", overflow: "auto" }}
    />
  );
}
