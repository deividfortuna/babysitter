import { spawnSync } from "node:child_process";
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { fileURLToPath } from "node:url";
import { Arch, build, Platform } from "electron-builder";
import { builderConfig } from "./desktop-builder.ts";

const frontendDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outputDir = join(frontendDir, "out", "release");
const stageDir = join(frontendDir, "out", "stage");

const PLATFORMS = { darwin: Platform.MAC, win32: Platform.WINDOWS, linux: Platform.LINUX };

const { values } = parseArgs({
  options: {
    platform: { type: "string", default: process.platform },
    arch: { type: "string", default: process.arch },
    dir: { type: "boolean", default: false },
  },
});

const platform = PLATFORMS[values.platform];
const arch = Arch[values.arch];
if (!platform || arch === undefined) {
  console.error(`usage: build-desktop.mjs [--platform=darwin|win32|linux] [--arch=arm64|x64] [--dir]`);
  process.exit(2);
}

function run(command, args, env = {}) {
  const result = spawnSync(command, args, {
    cwd: frontendDir,
    stdio: "inherit",
    shell: process.platform === "win32",
    env: { ...process.env, ...env },
  });
  if (result.status !== 0) process.exit(result.status ?? 1);
}

function readJson(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

run(process.execPath, [join(frontendDir, "scripts", "build-daemon.mjs")], {
  BABYSITTER_DAEMON_PLATFORM: values.platform,
  BABYSITTER_DAEMON_ARCH: values.arch,
});
run("vp", ["pack"]);
run("vp", ["build", "-c", "vite.renderer.config.mts"]);

const app = readJson(join(frontendDir, "package.json"));
const electron = readJson(join(frontendDir, "node_modules", "electron", "package.json"));

rmSync(stageDir, { recursive: true, force: true });
mkdirSync(stageDir, { recursive: true });
cpSync(join(frontendDir, "dist-electron"), join(stageDir, "dist-electron"), { recursive: true });
cpSync(join(frontendDir, "dist", "renderer"), join(stageDir, "dist", "renderer"), { recursive: true });
writeFileSync(
  join(stageDir, "package.json"),
  `${JSON.stringify(
    {
      name: app.name,
      productName: app.productName,
      version: app.version,
      description: app.description,
      author: app.author,
      license: app.license,
      homepage: "https://github.com/deividfortuna/babysitter",
      private: true,
      main: app.main,
      devDependencies: { electron: electron.version },
    },
    null,
    2,
  )}\n`,
);

await build({
  projectDir: stageDir,
  targets: platform.createTarget(values.dir ? ["dir"] : null, arch),
  publish: "never",
  config: {
    ...builderConfig({ platform: values.platform, version: app.version, frontendDir, outputDir, env: process.env }),
    electronVersion: electron.version,
  },
});
