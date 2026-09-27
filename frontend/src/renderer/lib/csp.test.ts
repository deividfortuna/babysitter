import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, test } from "vite-plus/test";

let built: Promise<string> | null = null;

function builtPage(): Promise<string> {
  built ??= (async () => {
    const { build } = await import("vite");
    const outDir = mkdtempSync(join(tmpdir(), "babysitter-csp-"));
    await build({
      configFile: resolve(process.cwd(), "vite.renderer.config.mts"),
      root: process.cwd(),
      logLevel: "silent",
      build: { outDir, emptyOutDir: true },
    });
    return readFileSync(join(outDir, "index.html"), "utf8");
  })();
  return built;
}

function policyOf(html: string): string {
  const meta = html.match(/<meta http-equiv="Content-Security-Policy" content="([^"]*)"/)?.[1];
  if (!meta) throw new Error("the built page carries no Content-Security-Policy");
  return meta.replaceAll("&#39;", "'").replaceAll("&amp;", "&").replaceAll("&quot;", '"');
}

function directive(policy: string, name: string): string {
  return (
    policy
      .split(";")
      .map((part) => part.trim())
      .find((part) => part.startsWith(name)) ?? ""
  );
}

function inlineScripts(html: string): string[] {
  return [...html.matchAll(/<script(?![^>]*\ssrc=)[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]);
}

test("script-src names every inline script of the built page", async () => {
  const html = await builtPage();
  const allowed = directive(policyOf(html), "script-src");

  for (const script of inlineScripts(html)) {
    const hash = createHash("sha256").update(script, "utf8").digest("base64");
    expect(allowed, `the page carries an inline script Chromium answers for with 'sha256-${hash}'`).toContain(
      `'sha256-${hash}'`,
    );
  }
  expect(inlineScripts(html).length).toBeGreaterThan(0);
});

test("the policy keeps its other rules", async () => {
  const policy = policyOf(await builtPage());

  expect(directive(policy, "script-src")).not.toContain("'unsafe-inline'");
  expect(directive(policy, "connect-src")).toBe("connect-src 'self' http://127.0.0.1:*");
  expect(directive(policy, "object-src")).toBe("object-src 'none'");
});
