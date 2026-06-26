/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "#0c0c0e",
        panel: "#141417",
        line: "#26262b",
        ink: "#f4f3f0",
        mut: "#9a9aa2",
        accent: "#ff6a5a",
        accent2: "#e8503f",
        ok: "#36c08a",
        warn: "#ebb950",
      },
      fontFamily: {
        mono: ["ui-monospace", "Cascadia Code", "JetBrains Mono", "Menlo", "Consolas", "monospace"],
      },
    },
  },
  plugins: [],
};
