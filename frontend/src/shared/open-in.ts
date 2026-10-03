export type EditorDefinition = {
  readonly id: string;
  readonly label: string;
  readonly commands: readonly [string, ...string[]];
  readonly baseArgs?: readonly string[];
  readonly installArgs?: readonly string[];
  readonly installNames?: readonly string[];
  readonly macBundleCommand?: string;
  readonly windowsFolder?: string;
  readonly windowsExecutable?: string;
  readonly jetbrains?: boolean;
};

export const EDITORS = [
  { id: "cursor", label: "Cursor", commands: ["cursor"], baseArgs: ["--classic"] },
  { id: "trae", label: "Trae", commands: ["trae"] },
  { id: "kiro", label: "Kiro", commands: ["kiro"], baseArgs: ["ide"], installArgs: [] },
  {
    id: "vscode",
    label: "VS Code",
    commands: ["code"],
    installNames: ["Visual Studio Code"],
    windowsFolder: "Microsoft VS Code",
  },
  {
    id: "vscode-insiders",
    label: "VS Code Insiders",
    commands: ["code-insiders"],
    installNames: ["Visual Studio Code - Insiders"],
    windowsFolder: "Microsoft VS Code Insiders",
  },
  { id: "vscodium", label: "VSCodium", commands: ["codium"] },
  {
    id: "zed",
    label: "Zed",
    commands: ["zed", "zeditor"],
    macBundleCommand: "MacOS/cli",
    windowsExecutable: "zed.exe",
  },
  {
    id: "antigravity",
    label: "Antigravity",
    commands: ["antigravity-ide", "agy-ide"],
    installNames: ["Antigravity IDE"],
  },
  {
    id: "idea",
    label: "IntelliJ IDEA",
    commands: ["idea"],
    installNames: ["IntelliJ IDEA", "IntelliJ IDEA CE", "IntelliJ IDEA Ultimate"],
    jetbrains: true,
  },
  { id: "aqua", label: "Aqua", commands: ["aqua"], jetbrains: true },
  { id: "clion", label: "CLion", commands: ["clion"], jetbrains: true },
  { id: "datagrip", label: "DataGrip", commands: ["datagrip"], jetbrains: true },
  { id: "dataspell", label: "DataSpell", commands: ["dataspell"], jetbrains: true },
  { id: "goland", label: "GoLand", commands: ["goland"], jetbrains: true },
  { id: "phpstorm", label: "PhpStorm", commands: ["phpstorm"], jetbrains: true },
  { id: "pycharm", label: "PyCharm", commands: ["pycharm"], installNames: ["PyCharm", "PyCharm CE"], jetbrains: true },
  { id: "rider", label: "Rider", commands: ["rider"], installNames: ["Rider", "JetBrains Rider"], jetbrains: true },
  { id: "rubymine", label: "RubyMine", commands: ["rubymine"], jetbrains: true },
  { id: "rustrover", label: "RustRover", commands: ["rustrover"], jetbrains: true },
  { id: "webstorm", label: "WebStorm", commands: ["webstorm"], jetbrains: true },
] as const satisfies readonly EditorDefinition[];

export type EditorId = (typeof EDITORS)[number]["id"];

export const FILE_MANAGER = "file-manager";

export type OpenTarget = EditorId | typeof FILE_MANAGER;

export type OpenFolderResult = { ok: true } | { ok: false; error: string };

const EDITOR_BY_ID = new Map<string, EditorDefinition>(EDITORS.map((editor) => [editor.id, editor]));

export function findEditor(id: unknown): EditorDefinition | undefined {
  return typeof id === "string" ? EDITOR_BY_ID.get(id) : undefined;
}

export function isOpenTarget(value: unknown): value is OpenTarget {
  return value === FILE_MANAGER || findEditor(value) !== undefined;
}

export function openPathResult(error: string): OpenFolderResult {
  return error ? { ok: false, error } : { ok: true };
}

export type WatchFolderSource = {
  worktreeDir: string;
  sourceDir: string;
  provider: string;
  summary?: { worktreeRemoved?: boolean } | null;
};

export function watchFolder(watch: WatchFolderSource): string | null {
  if (watch.summary?.worktreeRemoved) return null;
  if (watch.worktreeDir) return watch.worktreeDir;
  const agentWorkedInTheCheckout = watch.provider === "self";
  return agentWorkedInTheCheckout && watch.sourceDir ? watch.sourceDir : null;
}
