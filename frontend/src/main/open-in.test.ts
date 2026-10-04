import { EventEmitter } from "node:events";
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, onTestFinished, test } from "vite-plus/test";
import { findEditor } from "../shared/open-in";
import { EditorLauncher, resolveEditorCommand, spawnCommand, type FileProbe } from "./open-in";

const isWindows = process.platform === "win32";

function tempDir(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "babysitter-open-in-"));
  onTestFinished(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}

function executable(file: string) {
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, "#!/bin/sh\n");
  chmodSync(file, 0o755);
}

function editor(id: string) {
  const found = findEditor(id);
  if (!found) throw new Error(`no editor ${id}`);
  return found;
}

function parentOf(file: string): string {
  return file.slice(0, Math.max(file.lastIndexOf("/"), file.lastIndexOf("\\")));
}

function entriesOf(dir: string, files: string[]): string[] {
  return files.filter((file) => parentOf(file) === dir).map((file) => file.slice(dir.length + 1));
}

function fakeFiles(runnable: string[], listings: Record<string, string[]> = {}): FileProbe {
  return {
    isRunnable: async (file) => runnable.includes(file),
    isDirectory: async () => true,
    list: async (dir) => listings[dir] ?? entriesOf(dir, runnable),
  };
}

type Spawned = { command: string; args: string[]; options: Record<string, unknown>; unrefs: number };

function fakeSpawn(outcome: "spawn" | Error = "spawn") {
  const calls: Spawned[] = [];
  const spawn = (command: string, args: string[], options: object) => {
    const child = Object.assign(new EventEmitter(), {
      unref: () => {
        call.unrefs++;
      },
    });
    const call: Spawned = { command, args, options: options as Record<string, unknown>, unrefs: 0 };
    calls.push(call);
    queueMicrotask(() => (outcome === "spawn" ? child.emit("spawn") : child.emit("error", outcome)));
    return child as never;
  };
  return { calls, spawn };
}

test.skipIf(isWindows)("finds an editor on the PATH", async () => {
  const bin = tempDir();
  executable(path.join(bin, "code"));

  const command = await resolveEditorCommand(editor("vscode"), "darwin", { PATH: `/nowhere:${bin}` });

  expect(command).toEqual({ command: path.join(bin, "code"), args: [] });
});

test.skipIf(isWindows)("skips a file on the PATH that does not run", async () => {
  const bin = tempDir();
  writeFileSync(path.join(bin, "code"), "");

  expect(await resolveEditorCommand(editor("vscode"), "linux", { PATH: bin, HOME: "/nowhere" })).toBeNull();
});

const vscodeBundle = "/Users/me/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code";
const zedCli = "/Applications/Zed.app/Contents/MacOS/cli";
const golandScript = "/Users/me/Library/Application Support/JetBrains/Toolbox/scripts/goland";
const kiroBundle = "/Applications/Kiro.app/Contents/Resources/app/bin/kiro";
const pycharmScript = "/home/me/.local/share/JetBrains/Toolbox/scripts/pycharm";
const vscodeShim = "C:\\Tools\\code.cmd";
const vscodeProgramFiles = "C:\\Program Files\\Microsoft VS Code\\bin\\code.cmd";
const ideaExe = "C:\\Program Files\\JetBrains\\IntelliJ IDEA 2026.2\\bin\\idea64.exe";

test.each<{
  name: string;
  id: string;
  platform: NodeJS.Platform;
  env: Record<string, string>;
  files: string[];
  listings?: Record<string, string[]>;
  expected: { command: string; args: string[] };
}>([
  {
    name: "finds VS Code inside its app bundle on macOS when the PATH does not have it",
    id: "vscode",
    platform: "darwin",
    env: { HOME: "/Users/me" },
    files: [vscodeBundle],
    expected: { command: vscodeBundle, args: [] },
  },
  {
    name: "finds the Zed cli inside its app bundle on macOS",
    id: "zed",
    platform: "darwin",
    env: {},
    files: [zedCli],
    expected: { command: zedCli, args: [] },
  },
  {
    name: "finds a JetBrains IDE in the Toolbox scripts on macOS",
    id: "goland",
    platform: "darwin",
    env: { HOME: "/Users/me" },
    files: [golandScript],
    expected: { command: golandScript, args: [] },
  },
  {
    name: "opens Cursor in the editor window and not in the agents window",
    id: "cursor",
    platform: "linux",
    env: { PATH: "/bin" },
    files: ["/bin/cursor"],
    expected: { command: "/bin/cursor", args: ["--classic"] },
  },
  {
    name: "starts Kiro from its app bundle without the ide argument of its cli",
    id: "kiro",
    platform: "darwin",
    env: {},
    files: [kiroBundle],
    expected: { command: kiroBundle, args: [] },
  },
  {
    name: "finds a JetBrains IDE in a Toolbox folder on Linux",
    id: "pycharm",
    platform: "linux",
    env: { HOME: "/home/me" },
    files: [pycharmScript],
    expected: { command: pycharmScript, args: [] },
  },
  {
    name: "finds the command shim of VS Code on Windows with the PATHEXT of the env",
    id: "vscode",
    platform: "win32",
    env: { Path: "C:\\Tools", PATHEXT: ".EXE;.CMD" },
    files: [vscodeShim],
    expected: { command: vscodeShim, args: [] },
  },
  {
    name: "finds VS Code in Program Files on Windows when the PATH does not have it",
    id: "vscode",
    platform: "win32",
    env: { ProgramFiles: "C:\\Program Files" },
    files: [vscodeProgramFiles],
    expected: { command: vscodeProgramFiles, args: [] },
  },
  {
    name: "finds a JetBrains IDE in a versioned folder on Windows",
    id: "idea",
    platform: "win32",
    env: { ProgramFiles: "C:\\Program Files" },
    files: [ideaExe],
    listings: { "C:\\Program Files\\JetBrains": ["IntelliJ IDEA 2026.2", "GoLand 2026.2"] },
    expected: { command: ideaExe, args: [] },
  },
])("$name", async ({ id, platform, env, files, listings, expected }) => {
  expect(await resolveEditorCommand(editor(id), platform, env, fakeFiles(files, listings))).toEqual(expected);
});

test("runs a command shim on Windows through cmd with every argument quoted", () => {
  expect(spawnCommand({ command: "C:\\Program Files\\code.cmd", args: [] }, "C:\\work\\a&b (1)", "win32")).toEqual({
    command: '^"C:\\Program^ Files\\code.cmd^"',
    args: ['^"C:\\work\\a^&b^ ^(1^)^"'],
    shell: true,
  });
});

test("runs an executable without a shell", () => {
  expect(spawnCommand({ command: "/bin/cursor", args: ["--classic"] }, "/work/a b", "darwin")).toEqual({
    command: "/bin/cursor",
    args: ["--classic", "/work/a b"],
    shell: false,
  });
});

const WATCH_FOLDERS: Record<number, string> = { 12: "/work/pr-12", 13: "/work/gone" };

function launcher(options: Partial<ConstructorParameters<typeof EditorLauncher>[0]> = {}) {
  const opened: string[] = [];
  const spawned = fakeSpawn();
  const editors = new EditorLauncher({
    platform: "darwin",
    env: async () => ({ PATH: "/bin" }),
    openPath: async (dir) => {
      opened.push(dir);
      return "";
    },
    watchFolder: async (watchId) => WATCH_FOLDERS[watchId] ?? null,
    files: fakeFiles(["/bin/code", "/bin/zed"]),
    spawn: spawned.spawn,
    ...options,
  });
  return { editors, opened, spawned: spawned.calls };
}

test("lists the installed editors in the order of the table, with the file manager last", async () => {
  const { editors } = launcher();

  expect(await editors.targets()).toEqual(["vscode", "zed", "file-manager"]);
});

test("reads each folder of the PATH one time per scan", async () => {
  const listed: string[] = [];
  const runnable = ["/bin/code", "/usr/bin/zed"];
  const { editors } = launcher({
    env: async () => ({ PATH: "/bin:/usr/bin:/bin" }),
    files: {
      ...fakeFiles(runnable),
      list: async (dir) => {
        listed.push(dir);
        return entriesOf(dir, runnable);
      },
    },
  });

  expect(await editors.targets()).toEqual(["vscode", "zed", "file-manager"]);
  expect(listed.filter((dir) => dir === "/bin")).toEqual(["/bin"]);
  expect(listed.filter((dir) => dir === "/usr/bin")).toEqual(["/usr/bin"]);
});

test("opens the folder of the watch in the file manager of the system", async () => {
  const { editors, opened } = launcher();

  expect(await editors.launch(12, "file-manager")).toEqual({ ok: true });
  expect(opened).toEqual(["/work/pr-12"]);
});

test("says why the file manager did not open the folder", async () => {
  const { editors } = launcher({ openPath: async () => "no app for the folder" });

  expect(await editors.launch(12, "file-manager")).toEqual({ ok: false, error: "no app for the folder" });
});

test("starts the editor on the folder of the watch in the background with the env of the login shell", async () => {
  const { editors, spawned } = launcher();

  expect(await editors.launch(12, "vscode")).toEqual({ ok: true });
  expect(spawned).toEqual([
    {
      command: "/bin/code",
      args: ["/work/pr-12"],
      options: { detached: true, stdio: "ignore", shell: false, windowsHide: true, env: { PATH: "/bin" } },
      unrefs: 1,
    },
  ]);
});

test("says why the editor did not start", async () => {
  const { editors } = launcher({ spawn: fakeSpawn(new Error("spawn EACCES")).spawn });

  expect(await editors.launch(12, "vscode")).toEqual({
    ok: false,
    error: "Could not start VS Code: spawn EACCES",
  });
});

test("says when the editor is not installed", async () => {
  const { editors } = launcher();

  expect(await editors.launch(12, "webstorm")).toEqual({ ok: false, error: "WebStorm is not installed." });
});

test("refuses an editor it does not know, a watch it does not know and a watch with no folder", async () => {
  const { editors, spawned, opened } = launcher({
    files: { ...fakeFiles(["/bin/code"]), isDirectory: async (dir) => dir === "/work/pr-12" },
  });

  expect(await editors.launch(12, "notepad")).toEqual({ ok: false, error: "Unknown editor: notepad." });
  expect(await editors.launch("/work/pr-12", "vscode")).toEqual({ ok: false, error: "Unknown watch: /work/pr-12." });
  expect(await editors.launch(1.5, "vscode")).toEqual({ ok: false, error: "Unknown watch: 1.5." });
  expect(await editors.launch(99, "vscode")).toEqual({
    ok: false,
    error: "This watch has no folder on this computer.",
  });
  expect(await editors.launch(13, "file-manager")).toEqual({
    ok: false,
    error: "The folder /work/gone does not exist.",
  });
  expect({ spawned, opened }).toEqual({ spawned: [], opened: [] });
});

test("reads the env of the login shell one time, even when it fell back", async () => {
  let reads = 0;
  const { editors } = launcher({
    env: async () => {
      reads++;
      return { PATH: "/bin" };
    },
  });

  await editors.targets();
  await editors.launch(12, "vscode");
  await editors.launch(12, "zed");

  expect(reads).toBe(1);
});

test("reads each install folder on Windows one time per scan", async () => {
  const listed: string[] = [];
  const { editors } = launcher({
    platform: "win32",
    env: async () => ({ ProgramFiles: "C:\\Program Files", ProgramW6432: "C:\\Program Files" }),
    files: {
      ...fakeFiles([]),
      list: async (dir) => {
        listed.push(dir);
        return [];
      },
    },
  });

  await editors.targets();

  expect([...listed].sort()).toEqual(["C:\\Program Files", "C:\\Program Files\\JetBrains"]);
});

test.skipIf(isWindows)("checks the real folder and the real file on the disk", async () => {
  const dir = tempDir();
  const bin = path.join(dir, "bin");
  executable(path.join(bin, "zed"));
  const spawned = fakeSpawn();
  const folders: Record<number, string> = { 1: dir, 2: path.join(dir, "missing") };
  const editors = new EditorLauncher({
    platform: process.platform,
    env: async () => ({ PATH: bin }),
    openPath: async () => "",
    watchFolder: async (watchId) => folders[watchId] ?? null,
    spawn: spawned.spawn,
  });

  expect(await editors.launch(1, "zed")).toEqual({ ok: true });
  expect(await editors.launch(2, "zed")).toMatchObject({ ok: false });
  expect(spawned.calls.map((call) => call.command)).toEqual([path.join(bin, "zed")]);
});
