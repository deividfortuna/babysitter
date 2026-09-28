const STORAGE_KEY = "proposal_viewed_files";

const KEPT_PROPOSALS = 20;

export type ViewedFiles = Readonly<Record<string, string>>;

type Stored = Record<string, { at: number; files: Record<string, string> }>;

function readAll(): Stored {
  try {
    const stored: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "null");
    return typeof stored === "object" && stored !== null ? (stored as Stored) : {};
  } catch {
    return {};
  }
}

function isFileMap(value: unknown): value is Record<string, string> {
  return typeof value === "object" && value !== null && Object.values(value).every((v) => typeof v === "string");
}

export function readViewedFiles(key: string): ViewedFiles {
  const files: unknown = readAll()[key]?.files;
  return isFileMap(files) ? files : {};
}

function newest(stored: Stored): Stored {
  const kept = Object.entries(stored)
    .sort(([, a], [, b]) => b.at - a.at)
    .slice(0, KEPT_PROPOSALS);
  return Object.fromEntries(kept);
}

export function storeViewedFiles(key: string, files: ViewedFiles): void {
  try {
    const stored = { ...readAll(), [key]: { at: Date.now(), files: { ...files } } };
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(newest(stored)));
  } catch {}
}
