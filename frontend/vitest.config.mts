import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: [
      { find: /^cn$/, replacement: fileURLToPath(new URL("./src/renderer/lib/utils.ts", import.meta.url)) },
      { find: "@", replacement: fileURLToPath(new URL("./src/renderer", import.meta.url)) },
      { find: "@test", replacement: fileURLToPath(new URL("./src/test", import.meta.url)) },
    ],
  },
  test: {
    environment: "jsdom",
    environmentOptions: {
      jsdom: {
        url: "http://localhost/",
      },
    },
    setupFiles: "./src/test/setup.ts",
    testTimeout: 15_000,
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      include: [
        "src/renderer/components/**/*.{ts,tsx}",
        "src/renderer/hooks/**/*.{ts,tsx}",
        "src/renderer/lib/{api-client,navigation,query-keys}.ts",
      ],
      exclude: ["src/renderer/components/ui/**", "src/test/**", "**/*.test.{ts,tsx}", "**/*.spec.{ts,tsx}"],
      thresholds: {
        branches: 47,
        functions: 62,
        lines: 70,
        statements: 66,
      },
    },
  },
});
