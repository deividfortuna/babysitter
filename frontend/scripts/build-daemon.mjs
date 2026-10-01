import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const scriptsDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(scriptsDir, "..");
const backendRoot = resolve(frontendRoot, "..", "backend");
const outDir = join(frontendRoot, "daemon");

const GOOS = { darwin: "darwin", win32: "windows", linux: "linux" };
const GOARCH = { arm64: "arm64", x64: "amd64" };
const MINGW_CC = { amd64: "x86_64-w64-mingw32-gcc", arm64: "aarch64-w64-mingw32-gcc" };

const platform = process.env.BABYSITTER_DAEMON_PLATFORM || process.platform;
const arch = process.env.BABYSITTER_DAEMON_ARCH || process.arch;
const goos = GOOS[platform];
const goarch = GOARCH[arch];
if (!goos || !goarch) {
  console.error(`the daemon does not build for ${platform} ${arch}`);
  process.exit(2);
}

const binaryName = goos === "windows" ? "babysitter.exe" : "babysitter";
const outPath = join(outDir, binaryName);
const crossToWindows = goos === "windows" && process.platform !== "win32";
const cc = crossToWindows && !process.env.CC ? { CC: MINGW_CC[goarch] } : {};

const { version } = JSON.parse(readFileSync(join(frontendRoot, "package.json"), "utf8"));

rmSync(outDir, { recursive: true, force: true });
mkdirSync(outDir, { recursive: true });

const result = spawnSync(
  "go",
  ["build", "-trimpath", "-ldflags", `-s -w -X main.version=${version}`, "-o", outPath, "./cmd/babysitter"],
  {
    cwd: backendRoot,
    stdio: "inherit",
    windowsHide: true,
    env: { ...process.env, CGO_ENABLED: "1", GOOS: goos, GOARCH: goarch, ...cc },
  },
);

if (result.error) {
  console.error(`failed to start go build: ${result.error.message}`);
  process.exit(1);
}
if (result.status !== 0) {
  process.exit(result.status ?? 1);
}
console.log(`daemon built at ${outPath}`);
