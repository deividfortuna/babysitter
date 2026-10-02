import { afterEach, expect, test, vi } from "vite-plus/test";
import { NO_CONNECTIONS, type Connections } from "../shared/connections";
import type { DaemonStatus } from "../shared/daemon-status";
import { ConnectionManager, type ConnectionManagerOptions } from "./connection-manager";
import type { RemoteCheck } from "./remote-check";

const TOKEN = "secret";

function fakeLocal() {
  let status: DaemonStatus = { state: "stopped" };
  const listeners = new Set<(s: DaemonStatus) => void>();
  const set = (next: DaemonStatus) => {
    status = next;
    for (const listener of listeners) listener(next);
  };
  return {
    getStatus: () => status,
    onStatus: (listener: (s: DaemonStatus) => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    start: vi.fn(async () => set({ state: "ready", baseUrl: "http://127.0.0.1:5000/api/v1", port: 5000 })),
    restart: vi.fn(async () => undefined),
    stop: vi.fn(async () => set({ state: "stopped" })),
    stopAnyOwner: vi.fn(async () => set({ state: "stopped" })),
  };
}

function setup(stored: Connections = NO_CONNECTIONS, check?: ConnectionManagerOptions["check"]) {
  const local = fakeLocal();
  const saved: Connections[] = [];
  const answers = check ?? (async (): Promise<RemoteCheck> => ({ ok: true, name: "studio", version: "1.0.0" }));
  const checkRemote = vi.fn(answers);
  const manager = new ConnectionManager({
    local,
    localName: "This Mac",
    read: () => stored,
    write: (next) => saved.push(next),
    check: checkRemote,
    discover: async () => [],
    probeMs: 60_000,
  });
  return { manager, local, saved, check: checkRemote };
}

const managers: ConnectionManager[] = [];

afterEach(() => {
  for (const manager of managers.splice(0)) manager.dispose();
});

test("the app starts the local daemon when the user shows this computer", async () => {
  const { manager, local } = setup();
  managers.push(manager);

  await manager.start();

  expect(local.start).toHaveBeenCalledOnce();
  expect(manager.getStatus()).toMatchObject({
    state: "ready",
    connection: { id: "local", kind: "local", name: "This Mac" },
  });
});

test("a pairing saves the remote, stops the local daemon and points the app at the remote with its token", async () => {
  const { manager, local, saved } = setup();
  managers.push(manager);
  await manager.start();

  const result = await manager.pair({ link: "http://studio.local:7420/#token=secret" });

  expect(result).toMatchObject({ ok: true, connection: { name: "studio", url: "http://studio.local:7420" } });
  expect(result.ok && "token" in result.connection).toBe(false);
  expect(local.stopAnyOwner).toHaveBeenCalledOnce();
  expect(saved.at(-1)?.remotes).toEqual([expect.objectContaining({ url: "http://studio.local:7420", token: TOKEN })]);
  expect(manager.getStatus()).toMatchObject({
    state: "ready",
    baseUrl: "http://studio.local:7420/api/v1",
    token: TOKEN,
    connection: { kind: "remote", name: "studio", version: "1.0.0" },
  });
});

test("a refused token saves nothing and leaves the local daemon running", async () => {
  const refused = async (): Promise<RemoteCheck> => ({ ok: false, error: "The daemon refused the token." });
  const { manager, local, saved } = setup(NO_CONNECTIONS, refused);
  managers.push(manager);
  await manager.start();

  const result = await manager.pair({ link: "http://studio.local:7420/#token=wrong" });

  expect(result).toEqual({ ok: false, error: "The daemon refused the token." });
  expect(saved).toEqual([]);
  expect(local.stopAnyOwner).not.toHaveBeenCalled();
});

test("a remote that does not answer shows as an error of that remote, not of the local daemon", async () => {
  const stored: Connections = {
    activeId: "r1",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const down = async (): Promise<RemoteCheck> => ({ ok: false, error: "No babysitter daemon answers." });
  const { manager, local } = setup(stored, down);
  managers.push(manager);

  await manager.start();

  expect(local.start).not.toHaveBeenCalled();
  expect(manager.getStatus()).toMatchObject({
    state: "error",
    message: "No babysitter daemon answers.",
    connection: { id: "r1", kind: "remote" },
  });
});

test("forgetting the remote that is shown goes back to the local daemon", async () => {
  const stored: Connections = {
    activeId: "r1",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const { manager, local, saved } = setup(stored);
  managers.push(manager);
  await manager.start();

  await manager.remove("r1");

  expect(local.start).toHaveBeenCalledOnce();
  expect(saved.at(-1)).toEqual({ activeId: "local", remotes: [] });
  expect(manager.list()).toEqual({ activeId: "local", localName: "This Mac", remotes: [] });
});

test("a save that fails leaves the list and the daemon that is shown as they were", async () => {
  const stored: Connections = {
    activeId: "local",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const local = fakeLocal();
  const manager = new ConnectionManager({
    local,
    localName: "This Mac",
    read: () => stored,
    write: () => {
      throw new Error("disk full");
    },
    check: async () => ({ ok: true, name: "studio", version: "1.0.0" }),
    discover: async () => [],
    probeMs: 60_000,
  });
  managers.push(manager);
  await manager.start();

  await expect(manager.use("r1")).rejects.toThrow("disk full");

  expect(manager.list().activeId).toBe("local");
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { kind: "local" } });
  expect(local.stopAnyOwner).not.toHaveBeenCalled();
});

test("a remote shown at launch stops the daemon that runs on this computer, also one started from a terminal", async () => {
  const stored: Connections = {
    activeId: "r1",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const { manager, local } = setup(stored);
  managers.push(manager);

  await manager.start();

  expect(local.stopAnyOwner).toHaveBeenCalledOnce();
  expect(local.start).not.toHaveBeenCalled();
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { id: "r1", kind: "remote" } });
});

test("showing this computer again after a remote starts its daemon again", async () => {
  const stored: Connections = {
    activeId: "r1",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const { manager, local } = setup(stored);
  managers.push(manager);
  await manager.start();

  await manager.use("local");

  expect(local.start).toHaveBeenCalledOnce();
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { kind: "local" } });
});

test("a remote shown at launch connects only after the daemon of this computer stopped", async () => {
  const stored: Connections = {
    activeId: "r1",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const { manager, local, check } = setup(stored);
  managers.push(manager);
  let stopped: () => void = () => undefined;
  local.stopAnyOwner.mockImplementationOnce(() => new Promise<void>((resolve) => (stopped = resolve)));

  const starting = manager.start();
  await vi.waitFor(() => expect(local.stopAnyOwner).toHaveBeenCalledOnce());

  expect(check).not.toHaveBeenCalled();
  expect(manager.getStatus().state).not.toBe("ready");
  stopped();
  await starting;
  expect(check).toHaveBeenCalledOnce();
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { id: "r1" } });
});

test("showing this computer while a switch to a remote still stops the local daemon waits for that stop", async () => {
  const stored: Connections = {
    activeId: "local",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const { manager, local } = setup(stored);
  managers.push(manager);
  await manager.start();
  let stopped: () => void = () => undefined;
  local.stopAnyOwner.mockImplementationOnce(() => new Promise<void>((resolve) => (stopped = resolve)));

  const toRemote = manager.use("r1");
  await vi.waitFor(() => expect(local.stopAnyOwner).toHaveBeenCalledOnce());
  const backToLocal = manager.use("local");
  await Promise.resolve();

  expect(local.start).toHaveBeenCalledOnce();
  stopped();
  await Promise.all([toRemote, backToLocal]);
  expect(local.start).toHaveBeenCalledTimes(2);
  expect(manager.list().activeId).toBe("local");
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { kind: "local" } });
});

function remoteShownAtLaunchWithASlowLocalStop() {
  const stored: Connections = {
    activeId: "r1",
    remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: TOKEN }],
  };
  const harness = setup(stored);
  managers.push(harness.manager);
  let stopped: () => void = () => undefined;
  harness.local.stopAnyOwner.mockImplementationOnce(() => new Promise<void>((resolve) => (stopped = resolve)));
  const starting = harness.manager.start();
  return { ...harness, starting, stopLocal: () => stopped() };
}

test("pairing again with the remote that is shown waits for the local daemon to stop", async () => {
  const { manager, local, starting, stopLocal } = remoteShownAtLaunchWithASlowLocalStop();
  await vi.waitFor(() => expect(local.stopAnyOwner).toHaveBeenCalledOnce());

  const pairing = manager.pair({ link: "http://studio.local:7420/#token=secret" });
  await new Promise((resolve) => setTimeout(resolve, 0));

  expect(manager.getStatus().state).not.toBe("ready");
  stopLocal();
  await Promise.all([starting, pairing]);
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { id: "r1" } });
});

test("a retry of the remote that is shown waits for the local daemon to stop", async () => {
  const { manager, local, starting, stopLocal } = remoteShownAtLaunchWithASlowLocalStop();
  await vi.waitFor(() => expect(local.stopAnyOwner).toHaveBeenCalledOnce());

  const retrying = manager.retry();
  await new Promise((resolve) => setTimeout(resolve, 0));

  expect(manager.getStatus().state).not.toBe("ready");
  stopLocal();
  await Promise.all([starting, retrying]);
  expect(manager.getStatus()).toMatchObject({ state: "ready", connection: { id: "r1" } });
});
