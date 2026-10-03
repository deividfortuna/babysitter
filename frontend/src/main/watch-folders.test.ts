import { expect, test } from "vite-plus/test";
import type { DaemonStatus } from "../shared/daemon-status";
import { localWatchFolder } from "./watch-folders";

const LOCAL: DaemonStatus = {
  state: "ready",
  baseUrl: "http://127.0.0.1:4100/api/v1",
  connection: { id: "local", kind: "local", name: "This Mac" },
};

function daemonWith(body: unknown, status = 200) {
  const asked: string[] = [];
  const fetcher = (async (url: string) => {
    asked.push(url);
    return new Response(JSON.stringify(body), { status });
  }) as typeof fetch;
  return { asked, fetcher };
}

function watch(fields: Record<string, unknown>) {
  return { id: 42, worktreeDir: "", sourceDir: "", provider: "claude", ...fields };
}

test("gives the worktree of the watch the local daemon has", async () => {
  const { asked, fetcher } = daemonWith(watch({ worktreeDir: "/data/worktrees/pr-12", sourceDir: "/code/repo" }));

  expect(await localWatchFolder(LOCAL, 42, fetcher)).toBe("/data/worktrees/pr-12");
  expect(asked).toEqual(["http://127.0.0.1:4100/api/v1/watches/42"]);
});

test("gives the checkout of a self watch", async () => {
  const { fetcher } = daemonWith(watch({ provider: "self", sourceDir: "/code/babysitter" }));

  expect(await localWatchFolder(LOCAL, 42, fetcher)).toBe("/code/babysitter");
});

test("gives no folder for a worktree that a stop deleted", async () => {
  const { fetcher } = daemonWith(watch({ worktreeDir: "/data/worktrees/pr-12", summary: { worktreeRemoved: true } }));

  expect(await localWatchFolder(LOCAL, 42, fetcher)).toBeNull();
});

test("gives no folder for the watch of a remote daemon, whose folders are on another machine", async () => {
  const { asked, fetcher } = daemonWith(watch({ worktreeDir: "/data/worktrees/pr-12" }));
  const remote: DaemonStatus = {
    ...LOCAL,
    baseUrl: "https://build-box:4100/api/v1",
    connection: { id: "build-box", kind: "remote", name: "Build box" },
  };

  expect(await localWatchFolder(remote, 42, fetcher)).toBeNull();
  expect(asked).toEqual([]);
});

test("gives no folder while the local daemon is not ready", async () => {
  const { asked, fetcher } = daemonWith(watch({ worktreeDir: "/data/worktrees/pr-12" }));

  expect(await localWatchFolder({ state: "starting", connection: LOCAL.connection }, 42, fetcher)).toBeNull();
  expect(asked).toEqual([]);
});

test("gives no folder when the daemon refuses or answers with something else", async () => {
  expect(await localWatchFolder(LOCAL, 42, daemonWith({ error: "not found" }, 404).fetcher)).toBeNull();
  expect(await localWatchFolder(LOCAL, 42, daemonWith(["not", "a", "watch"]).fetcher)).toBeNull();
  const unreachable = (async () => {
    throw new Error("connect ECONNREFUSED");
  }) as typeof fetch;
  expect(await localWatchFolder(LOCAL, 42, unreachable)).toBeNull();
});
