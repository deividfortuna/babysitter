import { createHash } from "node:crypto";
import { defineConfig } from "vite";
import type { Plugin } from "vite";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

function contentSecurityPolicy(inlineScriptHashes: string[]): string {
  return [
    "default-src 'self'",
    ["script-src 'self' 'wasm-unsafe-eval'", ...inlineScriptHashes.map((hash) => `'sha256-${hash}'`)].join(" "),
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: https:",
    "font-src 'self' data:",
    "connect-src 'self' http: https:",
    "object-src 'none'",
    "base-uri 'self'",
    "frame-src 'none'",
  ].join("; ");
}

function inlineScriptHashes(html: string): string[] {
  return [...html.matchAll(/<script(?![^>]*\ssrc=)[^>]*>([\s\S]*?)<\/script>/g)].map((match) =>
    createHash("sha256").update(match[1], "utf8").digest("base64"),
  );
}

const injectCspMeta: Plugin = {
  name: "inject-csp-meta",
  apply: "build",
  transformIndexHtml(html) {
    return [
      {
        tag: "meta",
        attrs: { "http-equiv": "Content-Security-Policy", content: contentSecurityPolicy(inlineScriptHashes(html)) },
        injectTo: "head-prepend",
      },
    ];
  },
};

export default defineConfig({
  assetsInclude: ["**/*.wasm"],
  resolve: {
    alias: [
      { find: /^cn$/, replacement: fileURLToPath(new URL("./src/renderer/lib/utils.ts", import.meta.url)) },
      { find: "@", replacement: fileURLToPath(new URL("./src/renderer", import.meta.url)) },
    ],
  },
  plugins: [react(), tailwindcss(), injectCspMeta],
});
