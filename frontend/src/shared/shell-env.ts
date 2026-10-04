export type Env = Record<string, string | undefined>;

export type ShellRunner = (shell: string, args: string[]) => Promise<string | null>;

export const SHELL_ENV_MARKER = "__BABYSITTER_SHELL_ENV__";

export function loginShellProbe(env: Env, platform: NodeJS.Platform): { shell: string; args: string[] } {
  const shell = env.SHELL?.trim() || defaultShell(platform);
  const flags = SHELL_FLAGS.get(shell.slice(shell.lastIndexOf("/") + 1)) ?? ["-ilc"];
  return { shell, args: [...flags, `printf '%s' '${SHELL_ENV_MARKER}'; env -0`] };
}

const SHELL_FLAGS = new Map([
  ["tcsh", ["-ic"]],
  ["csh", ["-ic"]],
  ["pwsh", ["-Login", "-Command"]],
]);

function defaultShell(platform: NodeJS.Platform): string {
  return platform === "darwin" ? "/bin/zsh" : "/bin/sh";
}

export function parseShellEnv(stdout: string): Record<string, string> | null {
  const at = stdout.lastIndexOf(SHELL_ENV_MARKER);
  const block = at === -1 ? stdout : stdout.slice(at + SHELL_ENV_MARKER.length);
  const records = block.includes("\0") ? block.split("\0") : block.split(/\r?\n/);
  const env: Record<string, string> = {};
  for (const record of records) {
    const eq = record.indexOf("=");
    if (eq <= 0) continue;
    env[record.slice(0, eq)] = record.slice(eq + 1);
  }
  return env.PATH ? env : null;
}

function fallbackDirs(home: string): string[] {
  return [
    "/opt/homebrew/bin",
    "/opt/homebrew/sbin",
    "/usr/local/bin",
    `${home}/.local/bin`,
    "/usr/bin",
    "/bin",
    "/usr/sbin",
    "/sbin",
  ];
}

export function withFallbackPath(path: string | undefined, home: string): string {
  const dirs = (path ?? "").split(":").filter(Boolean);
  const missing = fallbackDirs(home).filter((dir) => !dirs.includes(dir));
  return [...dirs, ...missing].join(":");
}

async function readLoginShellEnv(
  env: Env,
  platform: NodeJS.Platform,
  run: ShellRunner,
): Promise<Record<string, string> | null> {
  const { shell, args } = loginShellProbe(env, platform);
  try {
    const stdout = await run(shell, args);
    return stdout === null ? null : parseShellEnv(stdout);
  } catch {
    return null;
  }
}

export type DaemonEnvOptions = {
  platform: NodeJS.Platform;
  env: Env;
  home: string;
  run: ShellRunner;
  log?: (msg: string) => void;
};

function ownedByApp(name: string): boolean {
  return name === "HOME" || name === "TMPDIR" || name.startsWith("BABYSITTER_");
}

function appOwned(env: Env): Env {
  return Object.fromEntries(Object.entries(env).filter(([name]) => ownedByApp(name)));
}

async function readDaemonEnv({
  platform,
  env,
  home,
  run,
  log,
}: DaemonEnvOptions): Promise<{ env: Env; fellBack: boolean }> {
  if (platform === "win32") return { env, fellBack: false };
  const shellEnv = await readLoginShellEnv(env, platform, run);
  if (!shellEnv)
    log?.("daemon: could not read the environment of the login shell, the PATH takes the usual directories");
  return {
    env: { ...env, ...shellEnv, ...appOwned(env), PATH: withFallbackPath(shellEnv?.PATH ?? env.PATH, home) },
    fellBack: shellEnv === null,
  };
}

export function daemonEnvOnce(options: DaemonEnvOptions): () => Promise<Env> {
  let resolved: ReturnType<typeof readDaemonEnv> | null = null;
  return async () => {
    const pending = (resolved ??= readDaemonEnv(options));
    const { env, fellBack } = await pending;
    const dropFallback = fellBack && resolved === pending;
    if (dropFallback) resolved = null;
    return env;
  };
}
