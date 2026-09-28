import { expect, test } from "vite-plus/test";
import { isGenerated, isTest, readingOrder } from "./reading-order";

function paths(...list: string[]) {
  return list.map((path) => ({ path }));
}

test("source comes first, then tests, then generated files", () => {
  const order = readingOrder(
    paths(
      "backend/go.sum",
      "backend/internal/prwatch/proposal_test.go",
      "frontend/src/renderer/components/proposal-code.tsx",
      "backend/internal/prwatch/proposal.go",
      "frontend/package-lock.json",
      "frontend/src/renderer/components/proposal-panel.test.tsx",
    ),
  );

  expect(order.map((f) => f.path)).toEqual([
    "backend/internal/prwatch/proposal.go",
    "frontend/src/renderer/components/proposal-code.tsx",
    "backend/internal/prwatch/proposal_test.go",
    "frontend/src/renderer/components/proposal-panel.test.tsx",
    "backend/go.sum",
    "frontend/package-lock.json",
  ]);
});

test("a test follows the place of the file it covers", () => {
  const order = readingOrder(
    paths("src/b.ts", "src/a.ts", "src/__tests__/b.test.ts", "tests/test_a.py", "src/a.spec.ts", "a.py"),
  );

  expect(order.map((f) => f.path)).toEqual([
    "a.py",
    "src/a.ts",
    "src/b.ts",
    "tests/test_a.py",
    "src/a.spec.ts",
    "src/__tests__/b.test.ts",
  ]);
});

test("tests and generated files are told apart by their path", () => {
  expect(isTest("internal/x/x_test.go")).toBe(true);
  expect(isTest("src/lib/time.test.ts")).toBe(true);
  expect(isTest("spec/models/user_spec.rb")).toBe(true);
  expect(isTest("src/lib/latest.ts")).toBe(false);
  expect(isGenerated("api/service.pb.go")).toBe(true);
  expect(isGenerated("src/lib/generator.ts")).toBe(false);
});
