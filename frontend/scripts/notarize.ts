import { spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { setTimeout as sleep } from "node:timers/promises";

export type NotaryKey = { key: string; keyId: string; issuer: string };

export type Command = { command: string; args: string[]; cwd?: string };
export type CommandResult = { status: number; output: string };
export type Runner = (command: Command) => Promise<CommandResult>;

export type NotaryOptions = {
  run?: Runner;
  pause?: (ms: number) => Promise<void>;
  log?: (line: string) => void;
  attempts?: number;
  pauseMs?: number;
};

type Notary = Required<NotaryOptions>;

export const runCommand: Runner = async ({ command, args, cwd }) => {
  const result = spawnSync(command, args, { cwd, encoding: "utf8" });
  return { status: result.status ?? 1, output: `${result.stdout}${result.stderr}` };
};

const DEFAULTS: Notary = {
  run: runCommand,
  pause: (ms) => sleep(ms),
  log: (line) => console.log(line),
  attempts: 8,
  pauseMs: 30_000,
};

function keyArgs(key: NotaryKey): string[] {
  return ["--key", key.key, "--key-id", key.keyId, "--issuer", key.issuer];
}

function notarytool(args: string[], key: NotaryKey): Command {
  return { command: "xcrun", args: ["notarytool", ...args, ...keyArgs(key), "--output-format", "json"] };
}

function zip(appPath: string, archive: string): Command {
  return {
    command: "ditto",
    args: ["-c", "-k", "--sequesterRsrc", "--keepParent", path.basename(appPath), archive],
    cwd: path.dirname(appPath),
  };
}

function staple(appPath: string): Command {
  return { command: "xcrun", args: ["stapler", "staple", "-v", path.basename(appPath)], cwd: path.dirname(appPath) };
}

async function retry(step: string, notary: Notary, command: Command): Promise<string> {
  for (let attempt = 1; attempt <= notary.attempts; attempt += 1) {
    const result = await notary.run(command);
    if (result.status === 0) return result.output;
    notary.log(`${step} failed on attempt ${attempt} of ${notary.attempts}\n${result.output}`);
    if (attempt < notary.attempts) await notary.pause(notary.pauseMs);
  }
  throw new Error(`${step} failed ${notary.attempts} times`);
}

function field(output: string, name: string): string {
  const value: unknown = JSON.parse(output)[name];
  if (typeof value !== "string") throw new Error(`notarytool gave no ${name}:\n${output}`);
  return value;
}

export async function notarizeApp(appPath: string, key: NotaryKey, options: NotaryOptions = {}): Promise<void> {
  const notary = { ...DEFAULTS, ...options };
  const dir = mkdtempSync(path.join(tmpdir(), "notarize-"));
  try {
    const archive = path.join(dir, `${path.basename(appPath, ".app")}.zip`);
    await retry("zip", notary, zip(appPath, archive));
    const id = field(await retry("submit", notary, notarytool(["submit", archive], key)), "id");
    notary.log(`submitted ${id}`);
    const status = field(await retry("wait", notary, notarytool(["wait", id], key)), "status");
    if (status !== "Accepted") {
      const log = await notary.run(notarytool(["log", id], key));
      throw new Error(`notarization of ${id} ended ${status}\n${log.output}`);
    }
    await retry("staple", notary, staple(appPath));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
