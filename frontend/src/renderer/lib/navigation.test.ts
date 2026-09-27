import { expect, test } from "vite-plus/test";
import { sameView } from "./navigation";

test("distinguishes navigation destinations by their visible resource", () => {
  expect(sameView({ kind: "watch", id: 42 }, { kind: "watch", id: 42 })).toBe(true);
  expect(sameView({ kind: "watch", id: 42 }, { kind: "watch", id: 43 })).toBe(false);
  expect(sameView({ kind: "repo", name: "octo/babysitter" }, { kind: "repo", name: "octo/other" })).toBe(false);
  expect(sameView({ kind: "watching" }, { kind: "watching", repo: "octo/babysitter" })).toBe(false);
});
