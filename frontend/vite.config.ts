import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: [
      { find: /^cn$/, replacement: fileURLToPath(new URL("./src/renderer/lib/utils.ts", import.meta.url)) },
      { find: "@", replacement: fileURLToPath(new URL("./src/renderer", import.meta.url)) },
      {
        find: /^@pierre\/diffs\/react$/,
        replacement: fileURLToPath(new URL("./src/test/diffs-react.tsx", import.meta.url)),
      },
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
  lint: {
    plugins: ["eslint", "typescript", "unicorn", "oxc", "react", "jsx-a11y", "import", "vitest"],
    jsPlugins: [
      "@shadcn/lint",
      "eslint-plugin-better-tailwindcss",
      {
        name: "vite-plus",
        specifier: "vite-plus/oxlint-plugin",
      },
    ],
    categories: {
      correctness: "error",
    },
    settings: {
      "better-tailwindcss": {
        entryPoint: "src/renderer/styles.css",
      },
    },
    rules: {
      "shadcn/no-raw-colors": "error",
      "shadcn/no-arbitrary-values": ["error", { allow: ["layout"] }],
      "shadcn/no-inline-styles": "error",
      "shadcn/no-unknown-classes": "error",
      "shadcn/require-static-classes": "error",
      "better-tailwindcss/enforce-canonical-classes": "error",
      "better-tailwindcss/enforce-shorthand-classes": "error",
      "better-tailwindcss/no-conflicting-classes": "error",
      "better-tailwindcss/no-duplicate-classes": "error",
      "better-tailwindcss/no-deprecated-classes": "error",
      "vitest/require-mock-type-parameters": "off",
      "jsx-a11y/prefer-tag-over-role": "off",
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
      "typescript/no-floating-promises": "error",
      "typescript/no-misused-promises": "error",
      "vite-plus/prefer-vite-plus-imports": "error",
    },
    ignorePatterns: [".vite/", "coverage/", "daemon/", "out/", "src/api/schema.ts", "src/renderer/components/ui/"],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  fmt: {
    printWidth: 120,
    sortPackageJson: false,
    sortTailwindcss: {
      stylesheet: "./src/renderer/styles.css",
      functions: ["cn", "cva"],
    },
    ignorePatterns: [
      ".vite/",
      "components.json",
      "coverage/",
      "daemon/",
      "out/",
      "package-lock.json",
      "src/api/schema.ts",
      "src/renderer/components/ui/",
    ],
  },
});
