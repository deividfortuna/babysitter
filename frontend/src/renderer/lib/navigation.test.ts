import { expect, test } from "vite-plus/test";
import { goBack, goForward, HISTORY_LIMIT, sameView, startHistory, visit, type View } from "./navigation";

const watching: View = { kind: "watching" };
const stopped: View = { kind: "stopped" };
const watch: View = { kind: "watch", id: 42 };

test("distinguishes navigation destinations by their visible resource", () => {
  expect(sameView({ kind: "watch", id: 42 }, { kind: "watch", id: 42 })).toBe(true);
  expect(sameView({ kind: "watch", id: 42 }, { kind: "watch", id: 43 })).toBe(false);
  expect(sameView({ kind: "repo", name: "octo/babysitter" }, { kind: "repo", name: "octo/other" })).toBe(false);
  expect(sameView({ kind: "watching" }, { kind: "watching", repo: "octo/babysitter" })).toBe(false);
});

test("a visit to the current view keeps the history", () => {
  const history = visit(startHistory(watching), stopped);

  expect(visit(history, { kind: "stopped" })).toBe(history);
});

test("back and forward move between the visited views", () => {
  const visited = visit(visit(startHistory(watching), stopped), watch);

  const once = goBack(visited);
  expect(once).toEqual({ back: [watching], current: stopped, forward: [watch] });

  const twice = goBack(once);
  expect(twice).toEqual({ back: [], current: watching, forward: [stopped, watch] });

  expect(goForward(twice)).toEqual({ back: [watching], current: stopped, forward: [watch] });
});

test("back and forward do nothing at the ends of the history", () => {
  const history = startHistory(watching);

  expect(goBack(history)).toBe(history);
  expect(goForward(history)).toBe(history);
});

test("a visit after going back drops the forward views", () => {
  const history = goBack(visit(startHistory(watching), stopped));

  expect(visit(history, watch)).toEqual({ back: [watching], current: watch, forward: [] });
});

test("keeps the last views up to the history limit", () => {
  let history = startHistory({ kind: "watch", id: 0 });
  for (let id = 1; id <= HISTORY_LIMIT + 5; id++) history = visit(history, { kind: "watch", id });

  expect(history.back).toHaveLength(HISTORY_LIMIT);
  expect(history.back[0]).toEqual({ kind: "watch", id: 5 });
});
