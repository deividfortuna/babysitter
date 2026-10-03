import { mkdtempSync, readdirSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, test } from "vite-plus/test";

let built: Promise<string> | null = null;

function builtStyles(): Promise<string> {
  built ??= (async () => {
    const { build } = await import("vite");
    const outDir = mkdtempSync(join(tmpdir(), "babysitter-titlebar-"));
    await build({
      configFile: resolve(process.cwd(), "vite.renderer.config.mts"),
      root: process.cwd(),
      logLevel: "silent",
      build: { outDir, emptyOutDir: true },
    });
    const assets = join(outDir, "assets");
    return readdirSync(assets)
      .filter((file) => file.endsWith(".css"))
      .map((file) => readFileSync(join(assets, file), "utf8"))
      .join("\n");
  })();
  return built;
}

function rule(css: string, selector: string): string {
  const start = css.indexOf(`${selector}{`);
  if (start < 0) throw new Error(`the built styles carry no ${selector}`);
  return css.slice(start, css.indexOf("}", start));
}

test("the navigation buttons start after window buttons the system draws at the left", async () => {
  const css = await builtStyles();

  expect(rule(css, ".left-titlebar-nav-left")).toContain("env(titlebar-area-x");
}, 60_000);

test("a header under a collapsed sidebar clears window buttons the system draws at the left", async () => {
  const css = await builtStyles();

  expect(rule(css, ".pl-titlebar-nav-clearance")).toContain("env(titlebar-area-x");
}, 60_000);

test("horizontal scrollbars are as thin as vertical ones", async () => {
  const css = await builtStyles();
  const scrollbar = rule(css, "::-webkit-scrollbar");

  expect(scrollbar).toContain("width:var(--scrollbar-width)");
  expect(scrollbar).toContain("height:var(--scrollbar-width)");
}, 60_000);
