import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const scriptsDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(scriptsDir, "..");
const backendRoot = resolve(frontendRoot, "..", "backend");
const outDir = join(frontendRoot, "daemon");
const binaryName = process.platform === "win32" ? "babysitter.exe" : "babysitter";
const outPath = join(outDir, binaryName);

const { version } = JSON.parse(readFileSync(join(frontendRoot, "package.json"), "utf8"));

rmSync(outDir, { recursive: true, force: true });
mkdirSync(outDir, { recursive: true });

const result = spawnSync(
  "go",
  ["build", "-trimpath", "-ldflags", `-s -w -X main.version=${version}`, "-o", outPath, "./cmd/babysitter"],
  { cwd: backendRoot, stdio: "inherit", windowsHide: true },
);

if (result.error) {
  console.error(`failed to start go build: ${result.error.message}`);
  process.exit(1);
}
if (result.status !== 0) {
  process.exit(result.status ?? 1);
}
console.log(`daemon built at ${outPath}`);
