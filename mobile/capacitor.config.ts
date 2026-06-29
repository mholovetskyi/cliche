import type { CapacitorConfig } from "@capacitor/cli";

// Cliché Studio — native app config. The app wraps the SAME React build that's
// embedded in the Go binary (internal/web/static); at runtime it points itself at
// a remote Cliché backend via the in-app Connect screen (studio/src/lib/api.ts).
const config: CapacitorConfig = {
  appId: "app.cliche.studio", // change to your reverse-DNS bundle id (must match your Apple/Play account)
  appName: "Cliché Studio",
  webDir: "../internal/web/static",
  backgroundColor: "#0a0a0c",
  ios: { backgroundColor: "#0a0a0c", contentInset: "always" },
  android: { backgroundColor: "#0a0a0c" },
};

export default config;
