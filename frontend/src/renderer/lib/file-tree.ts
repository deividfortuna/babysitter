export type TreeFile<T> = { type: "file"; name: string; path: string; item: T };

export type TreeDir<T> = { type: "dir"; name: string; path: string; children: TreeNode<T>[] };

export type TreeNode<T> = TreeFile<T> | TreeDir<T>;

type Folder<T> = { dirs: Map<string, Folder<T>>; files: TreeFile<T>[] };

function emptyFolder<T>(): Folder<T> {
  return { dirs: new Map(), files: [] };
}

function insert<T>(root: Folder<T>, path: string, item: T): void {
  const parts = path.split("/");
  const name = parts.pop() ?? path;
  let folder = root;
  for (const part of parts) {
    const next = folder.dirs.get(part) ?? emptyFolder<T>();
    folder.dirs.set(part, next);
    folder = next;
  }
  folder.files.push({ type: "file", name, path, item });
}

function byName(a: { name: string }, b: { name: string }): number {
  if (a.name === b.name) return 0;
  return a.name < b.name ? -1 : 1;
}

function nodes<T>(folder: Folder<T>, prefix: string): TreeNode<T>[] {
  const dirs = [...folder.dirs].map(([name, child]) => dir(name, child, prefix));
  return [...dirs.sort(byName), ...folder.files.sort(byName)];
}

function dir<T>(name: string, folder: Folder<T>, prefix: string): TreeDir<T> {
  let label = name;
  let current = folder;
  const onlyOneFolder = (f: Folder<T>) => f.files.length === 0 && f.dirs.size === 1;
  while (onlyOneFolder(current)) {
    const [[childName, child]] = current.dirs;
    label = `${label}/${childName}`;
    current = child;
  }
  const path = prefix ? `${prefix}/${label}` : label;
  return { type: "dir", name: label, path, children: nodes(current, path) };
}

export function fileTree<T>(items: readonly T[], pathOf: (item: T) => string): TreeNode<T>[] {
  const root = emptyFolder<T>();
  for (const item of items) insert(root, pathOf(item), item);
  return nodes(root, "");
}
