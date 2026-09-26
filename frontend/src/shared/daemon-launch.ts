export type DaemonLaunchSpec = {
  command: string;
  source: "configured" | "bundled" | "dev";
};

function joinPath(...segments: string[]): string {
  return segments.map((s) => s.replace(/[/\\]+$/, "")).join("/");
}

export function daemonBinaryName(platform: NodeJS.Platform): string {
  return platform === "win32" ? "babysitter.exe" : "babysitter";
}

export function resolveDaemonLaunch(
  env: Record<string, string | undefined>,
  isPackaged: boolean,
  resourcesPath: string,
  appPath: string,
  platform: NodeJS.Platform,
): DaemonLaunchSpec {
  const configured = env.BABYSITTER_DAEMON_BINARY?.trim();
  if (configured) return { command: configured, source: "configured" };
  if (!isPackaged) return { command: joinPath(appPath, "daemon", daemonBinaryName(platform)), source: "dev" };
  return { command: joinPath(resourcesPath, "daemon", daemonBinaryName(platform)), source: "bundled" };
}
