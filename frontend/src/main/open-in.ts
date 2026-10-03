import { spawn as nodeSpawn, type ChildProcess, type SpawnOptions } from "node:child_process";
import { constants } from "node:fs";
import { access, readdir, stat } from "node:fs/promises";
import path from "node:path";
import {
  EDITORS,
  FILE_MANAGER,
  findEditor,
  isOpenTarget,
  openPathResult,
  type EditorDefinition,
  type OpenFolderResult,
  type OpenTarget,
} from "../shared/open-in";
import type { Env } from "../shared/shell-env";
import { errorMessage } from "../shared/values";

const DEFAULT_PATHEXT = [".COM", ".EXE", ".BAT", ".CMD"];
const WINDOWS_SHELL_META_CHARS = /([()\][%!^"`<>&|;, *?])/g;

export type FileProbe = {
  isRunnable(file: string): Promise<boolean>;
  isDirectory(dir: string): Promise<boolean>;
  list(dir: string): Promise<string[]>;
};

type Spawn = (command: string, args: string[], options: SpawnOptions) => ChildProcess;

type EditorLauncherOptions = {
  platform: NodeJS.Platform;
  env: () => Promise<Env>;
  openPath: (dir: string) => Promise<string>;
  watchFolder: (watchId: number) => Promise<string | null>;
  files?: FileProbe;
  spawn?: Spawn;
};

type EditorCommand = { command: string; args: readonly string[] };

type SpawnCommand = { command: string; args: string[]; shell: boolean };

const realFiles: FileProbe = {
  isRunnable: async (file) => {
    try {
      const info = await stat(file);
      if (!info.isFile()) return false;
      await access(file, constants.X_OK);
      return true;
    } catch {
      return false;
    }
  },
  isDirectory: async (dir) => {
    try {
      return (await stat(dir)).isDirectory();
    } catch {
      return false;
    }
  },
  list: (dir) => readdir(dir).catch(() => []),
};

export class EditorLauncher {
  private loadedEnv: Promise<Env> | null = null;

  constructor(private readonly opts: EditorLauncherOptions) {}

  async targets(): Promise<OpenTarget[]> {
    const env = await this.env();
    const files = withListingsReadOnce(this.files);
    const commands = await Promise.all(
      EDITORS.map((editor) => resolveEditorCommand(editor, this.opts.platform, env, files)),
    );
    const installed = EDITORS.filter((_, index) => commands[index] !== null).map((editor) => editor.id);
    return [...installed, FILE_MANAGER];
  }

  async launch(watchId: unknown, target: unknown): Promise<OpenFolderResult> {
    if (!isOpenTarget(target)) return failure(`Unknown editor: ${String(target)}.`);
    if (!isWatchId(watchId)) return failure(`Unknown watch: ${String(watchId)}.`);
    const dir = await this.opts.watchFolder(watchId);
    if (!dir) return failure("This watch has no folder on this computer.");
    if (!(await this.files.isDirectory(dir))) return failure(`The folder ${dir} does not exist.`);
    const editor = findEditor(target);
    if (!editor) return openPathResult(await this.opts.openPath(dir));
    const env = await this.env();
    const command = await resolveEditorCommand(editor, this.opts.platform, env, this.files);
    if (!command) return failure(`${editor.label} is not installed.`);
    return this.start(editor, spawnCommand(command, dir, this.opts.platform), env);
  }

  private start(editor: EditorDefinition, command: SpawnCommand, env: Env): Promise<OpenFolderResult> {
    const spawn = this.opts.spawn ?? nodeSpawn;
    return new Promise((resolve) => {
      const fail = (error: unknown) => resolve(failure(`Could not start ${editor.label}: ${errorMessage(error)}`));
      let child: ChildProcess;
      try {
        child = spawn(command.command, command.args, {
          detached: true,
          stdio: "ignore",
          shell: command.shell,
          windowsHide: true,
          env,
        });
      } catch (error) {
        fail(error);
        return;
      }
      child.once("error", fail);
      child.once("spawn", () => {
        child.unref();
        resolve({ ok: true });
      });
    });
  }

  private env(): Promise<Env> {
    this.loadedEnv ??= this.opts.env();
    return this.loadedEnv;
  }

  private get files(): FileProbe {
    return this.opts.files ?? realFiles;
  }
}

export async function resolveEditorCommand(
  editor: EditorDefinition,
  platform: NodeJS.Platform,
  env: Env,
  files: FileProbe = realFiles,
): Promise<EditorCommand | null> {
  const baseArgs = editor.baseArgs ?? [];
  for (const name of editor.commands) {
    const command = await findOnPath(name, platform, env, files);
    if (command) return { command, args: baseArgs };
  }
  const candidates = await installCandidates(editor, platform, env, files);
  const command = await firstRunnable(candidates, platform, env, files);
  if (!command) return null;
  const fromAppBundle = platform === "darwin" || platform === "win32";
  return { command, args: (fromAppBundle && editor.installArgs) || baseArgs };
}

async function findOnPath(name: string, platform: NodeJS.Platform, env: Env, files: FileProbe) {
  const paths = pathFor(platform);
  const dirs = pathDirs(env, paths.delimiter);
  const listings = await Promise.all(dirs.map((dir) => files.list(dir)));
  const wanted = withExtensions(name, platform, env);
  const candidates = dirs.flatMap((dir, index) =>
    wanted.filter((file) => isListed(listings[index], file, platform)).map((file) => paths.join(dir, file)),
  );
  return firstRunnable(candidates, platform, env, files);
}

function pathDirs(env: Env, delimiter: string): string[] {
  const dirs = (envValue(env, "PATH") ?? "")
    .split(delimiter)
    .map((dir) => dir.trim().replace(/^"(.*)"$/, "$1"))
    .filter(Boolean);
  return [...new Set(dirs)];
}

function isListed(entries: string[], file: string, platform: NodeJS.Platform): boolean {
  if (platform !== "win32") return entries.includes(file);
  const lower = file.toLowerCase();
  return entries.some((entry) => entry.toLowerCase() === lower);
}

async function firstRunnable(
  candidates: string[],
  platform: NodeJS.Platform,
  env: Env,
  files: FileProbe,
): Promise<string | null> {
  for (const candidate of candidates.flatMap((file) => withExtensions(file, platform, env))) {
    if (await files.isRunnable(candidate)) return candidate;
  }
  return null;
}

function withExtensions(file: string, platform: NodeJS.Platform, env: Env): string[] {
  if (platform !== "win32") return [file];
  const extensions = windowsPathExtensions(env);
  const extension = path.win32.extname(file).toUpperCase();
  if (extensions.includes(extension)) return [file];
  return extensions.map((ext) => `${file}${ext.toLowerCase()}`);
}

function windowsPathExtensions(env: Env): string[] {
  const parsed = (envValue(env, "PATHEXT") ?? "")
    .split(";")
    .map((entry) => entry.trim().toUpperCase())
    .filter(Boolean)
    .map((entry) => (entry.startsWith(".") ? entry : `.${entry}`));
  return parsed.length > 0 ? [...new Set(parsed)] : DEFAULT_PATHEXT;
}

async function installCandidates(
  editor: EditorDefinition,
  platform: NodeJS.Platform,
  env: Env,
  files: FileProbe,
): Promise<string[]> {
  const home = envValue(env, "HOME");
  if (platform === "darwin") return macCandidates(editor, home);
  if (platform === "win32") return windowsCandidates(editor, env, files);
  if (platform === "linux") return linuxCandidates(editor, home, env);
  return [];
}

function installNames(editor: EditorDefinition): readonly string[] {
  return editor.installNames ?? [editor.label];
}

function macCandidates(editor: EditorDefinition, home: string | undefined): string[] {
  const paths = path.posix;
  const command = editor.commands[0];
  const roots = [home && paths.join(home, "Applications"), "/Applications"].filter(isPresent);
  const bundles = roots.flatMap((root) =>
    installNames(editor).map((name) => paths.join(root, `${name}.app`, "Contents")),
  );
  const inBundles = bundles.flatMap((contents) => macBundleCommands(editor).map((file) => paths.join(contents, file)));
  const toolbox =
    editor.jetbrains && home
      ? [paths.join(home, "Library/Application Support/JetBrains/Toolbox/scripts", command)]
      : [];
  return [...inBundles, ...toolbox];
}

function macBundleCommands(editor: EditorDefinition): string[] {
  const command = editor.commands[0];
  if (editor.macBundleCommand) return [editor.macBundleCommand];
  if (editor.jetbrains) return [`MacOS/${command}`];
  return [`Resources/app/bin/${command}`, "Resources/app/bin/code"];
}

async function windowsCandidates(editor: EditorDefinition, env: Env, files: FileProbe): Promise<string[]> {
  const paths = path.win32;
  const command = editor.commands[0];
  const localAppData = envValue(env, "LOCALAPPDATA");
  const roots = uniqueIgnoringCase(
    [
      localAppData && paths.join(localAppData, "Programs"),
      envValue(env, "ProgramFiles"),
      envValue(env, "ProgramFiles(x86)"),
      envValue(env, "ProgramW6432"),
    ].filter(isPresent),
  );
  if (editor.jetbrains) {
    const toolbox = localAppData ? [paths.join(localAppData, "JetBrains/Toolbox/scripts", `${command}.cmd`)] : [];
    const dirs = roots.flatMap((root) => [root, paths.join(root, "JetBrains")]);
    const installs = await Promise.all(dirs.map((dir) => jetbrainsInstalls(dir, installNames(editor), files)));
    const binaries = installs
      .flat()
      .flatMap((install) => [
        paths.join(install, "bin", `${command}64.exe`),
        paths.join(install, "bin", `${command}.exe`),
      ]);
    return [...toolbox, ...binaries];
  }
  const folder = editor.windowsFolder ?? editor.label;
  const executable = editor.windowsExecutable;
  return roots.flatMap((root) => {
    const base = paths.join(root, folder);
    const shims = [
      paths.join(base, "resources/app/bin", `${command}.cmd`),
      paths.join(base, "resources/app/bin/code.cmd"),
      paths.join(base, "bin", `${command}.cmd`),
      paths.join(base, "bin/code.cmd"),
    ];
    const executables = executable ? [paths.join(base, "bin", executable), paths.join(base, executable)] : [];
    return [...shims, ...executables];
  });
}

async function jetbrainsInstalls(dir: string, names: readonly string[], files: FileProbe): Promise<string[]> {
  const entries = await files.list(dir);
  const isInstall = (entry: string) => names.some((name) => entry === name || entry.startsWith(`${name} `));
  return entries.filter(isInstall).map((entry) => path.win32.join(dir, entry));
}

function linuxCandidates(editor: EditorDefinition, home: string | undefined, env: Env): string[] {
  const paths = path.posix;
  const dataHome = envValue(env, "XDG_DATA_HOME") || (home && paths.join(home, ".local/share"));
  const toolbox = editor.jetbrains && dataHome ? paths.join(dataHome, "JetBrains/Toolbox/scripts") : undefined;
  const dirs = [home && paths.join(home, ".local/bin"), "/usr/local/bin", "/usr/bin", "/snap/bin", toolbox].filter(
    isPresent,
  );
  return dirs.flatMap((dir) => editor.commands.map((command) => paths.join(dir, command)));
}

export function spawnCommand(command: EditorCommand, dir: string, platform: NodeJS.Platform): SpawnCommand {
  const args = [...command.args, dir];
  const extension = path.win32.extname(command.command).toLowerCase();
  const isBatchShim = platform === "win32" && (extension === ".cmd" || extension === ".bat");
  if (!isBatchShim) return { command: command.command, args, shell: false };
  return { command: escapeWindowsShellArg(command.command), args: args.map(escapeWindowsShellArg), shell: true };
}

function escapeWindowsShellArg(arg: string): string {
  const quoted = arg.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\*)$/, "$1$1");
  return `"${quoted}"`.replace(WINDOWS_SHELL_META_CHARS, "^$1");
}

function envValue(env: Env, key: string): string | undefined {
  if (env[key] !== undefined) return env[key];
  const upper = key.toUpperCase();
  return Object.entries(env).find(([name]) => name.toUpperCase() === upper)?.[1];
}

function pathFor(platform: NodeJS.Platform): path.PlatformPath {
  return platform === "win32" ? path.win32 : path.posix;
}

function isWatchId(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function uniqueIgnoringCase(dirs: string[]): string[] {
  const seen = new Map<string, string>();
  for (const dir of dirs) {
    const key = dir.toLowerCase();
    if (!seen.has(key)) seen.set(key, dir);
  }
  return [...seen.values()];
}

function withListingsReadOnce(files: FileProbe): FileProbe {
  const listings = new Map<string, Promise<string[]>>();
  return {
    ...files,
    list: (dir) => {
      const listing = listings.get(dir) ?? files.list(dir);
      listings.set(dir, listing);
      return listing;
    },
  };
}

function isPresent(value: string | undefined | false): value is string {
  return Boolean(value);
}

function failure(error: string): OpenFolderResult {
  return { ok: false, error };
}
