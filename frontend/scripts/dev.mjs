import { spawn } from "node:child_process";
import { existsSync, unwatchFile, watchFile } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import electronPath from "electron";
import { createServer } from "vite";

const frontendDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const bundles = ["main.cjs", "preload.cjs"].map((name) => join(frontendDir, "dist-electron", name));
const electronArgs = process.argv.slice(2).filter((arg) => arg !== "--");
const RESTART_DEBOUNCE_MS = 300;

const renderer = await createServer({ root: frontendDir, configFile: join(frontendDir, "vite.renderer.config.mts") });
await renderer.listen();
const devServerUrl = renderer.resolvedUrls?.local[0];
if (!devServerUrl) throw new Error("the Vite dev server of the renderer has no local address");
renderer.printUrls();

const pack = spawn("vp", ["pack", "--watch"], {
  cwd: frontendDir,
  stdio: "inherit",
  shell: process.platform === "win32",
});

let app = null;
let restartTimer = null;
let stopping = false;
const replaced = new WeakSet();

function bundlesReady() {
  return bundles.every((bundle) => existsSync(bundle));
}

function startApp() {
  if (stopping) return;
  const env = { ...process.env, VITE_DEV_SERVER_URL: devServerUrl };
  delete env.ELECTRON_RUN_AS_NODE;
  const child = spawn(electronPath, [".", ...electronArgs], { cwd: frontendDir, stdio: "inherit", env });
  child.once("exit", (code) => {
    if (app === child) app = null;
    if (!replaced.has(child)) void stop(code ?? 0);
  });
  app = child;
}

function replaceApp() {
  restartTimer = null;
  if (stopping || !bundlesReady()) return;
  if (!app) return startApp();
  if (replaced.has(app)) return;
  const old = app;
  replaced.add(old);
  old.once("exit", startApp);
  old.kill();
}

function scheduleRestart() {
  clearTimeout(restartTimer);
  restartTimer = setTimeout(replaceApp, RESTART_DEBOUNCE_MS);
}

async function stop(code) {
  if (stopping) return;
  stopping = true;
  clearTimeout(restartTimer);
  for (const bundle of bundles) unwatchFile(bundle);
  if (app) {
    replaced.add(app);
    app.kill();
  }
  pack.kill();
  await renderer.close();
  process.exit(code);
}

for (const bundle of bundles) watchFile(bundle, { interval: 200 }, scheduleRestart);
pack.once("exit", (code) => void stop(code ?? 1));
process.once("SIGINT", () => void stop(130));
process.once("SIGTERM", () => void stop(143));
scheduleRestart();
