import { expect, test } from "vite-plus/test";
import { fileTree, type TreeNode } from "./file-tree";

function outline(nodes: TreeNode<string>[], depth = 0): string[] {
  return nodes.flatMap((n) =>
    n.type === "dir"
      ? [`${"  ".repeat(depth)}${n.name}/`, ...outline(n.children, depth + 1)]
      : [`${"  ".repeat(depth)}${n.name}`],
  );
}

test("folders come first, then files, each in name order", () => {
  const tree = fileTree(["b.go", "lib/z.js", "a.go", "lib/a.js", "cmd/main.go"], (p) => p);

  expect(outline(tree)).toEqual(["cmd/", "  main.go", "lib/", "  a.js", "  z.js", "a.go", "b.go"]);
});

test("a chain of folders with one folder each shows as one row", () => {
  const tree = fileTree(
    ["internal/webhook/deliver.go", "internal/webhook/deliver_test.go", "internal/store/sql/db.go"],
    (p) => p,
  );

  expect(outline(tree)).toEqual([
    "internal/",
    "  store/sql/",
    "    db.go",
    "  webhook/",
    "    deliver.go",
    "    deliver_test.go",
  ]);
  const [internal] = tree;
  expect(internal.type === "dir" && internal.children.map((c) => c.path)).toEqual([
    "internal/store/sql",
    "internal/webhook",
  ]);
});

test("a file keeps its item and its full path", () => {
  const [dir] = fileTree([{ path: "src/app.ts", added: 3 }], (f) => f.path);

  expect(dir).toMatchObject({ type: "dir", name: "src" });
  expect(dir.type === "dir" && dir.children[0]).toEqual({
    type: "file",
    name: "app.ts",
    path: "src/app.ts",
    item: { path: "src/app.ts", added: 3 },
  });
});
