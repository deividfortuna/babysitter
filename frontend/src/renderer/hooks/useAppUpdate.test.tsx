import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { bridge } from "@/lib/bridge";
import type { UpdateSettings, UpdateStatus } from "../../shared/updates";
import { useAppUpdate, useUpdateSettings } from "./useAppUpdate";

afterEach(() => {
  vi.restoreAllMocks();
});

function pushedStatus(initial: UpdateStatus) {
  let push: (status: UpdateStatus) => void = () => undefined;
  const off = vi.fn();
  vi.spyOn(bridge.updates, "getStatus").mockResolvedValue(initial);
  vi.spyOn(bridge.updates, "onStatus").mockImplementation((listener) => {
    push = listener;
    return off;
  });
  return { push: (status: UpdateStatus) => push(status), off };
}

test("the status starts unknown, then shows what the main process says", async () => {
  pushedStatus({ state: "idle", currentVersion: "0.1.0" });

  const { result } = renderHook(() => useAppUpdate());

  expect(result.current.status).toBeNull();
  await waitFor(() => expect(result.current.status).toEqual({ state: "idle", currentVersion: "0.1.0" }));
});

test("each status the main process pushes replaces the last one", async () => {
  const main = pushedStatus({ state: "idle", currentVersion: "0.1.0" });
  const { result } = renderHook(() => useAppUpdate());
  await waitFor(() => expect(result.current.status?.state).toBe("idle"));

  act(() => main.push({ state: "downloading", currentVersion: "0.1.0", version: "0.2.0", percent: 30 }));

  expect(result.current.status).toMatchObject({ state: "downloading", percent: 30 });
});

test("the subscription ends with the component", async () => {
  const main = pushedStatus({ state: "idle", currentVersion: "0.1.0" });
  const { unmount } = renderHook(() => useAppUpdate());

  unmount();

  expect(main.off).toHaveBeenCalledOnce();
});

test("a check shows the status the main process pushes", async () => {
  const main = pushedStatus({ state: "idle", currentVersion: "0.1.0" });
  const check = vi.spyOn(bridge.updates, "check").mockImplementation(async () => {
    main.push({ state: "not-available", currentVersion: "0.1.0", checkedAt: "2026-09-26T10:00:00Z" });
  });
  const { result } = renderHook(() => useAppUpdate());
  await waitFor(() => expect(result.current.status?.state).toBe("idle"));

  await act(() => result.current.check());

  expect(check).toHaveBeenCalledOnce();
  expect(result.current.status).toMatchObject({ state: "not-available" });
});

test("download and install go to the main process", async () => {
  pushedStatus({ state: "available", currentVersion: "0.1.0", version: "0.2.0" });
  const download = vi.spyOn(bridge.updates, "download").mockResolvedValue(undefined);
  const install = vi.spyOn(bridge.updates, "install").mockResolvedValue(undefined);
  const { result } = renderHook(() => useAppUpdate());

  await act(() => result.current.download());
  await act(() => result.current.install());

  expect(download).toHaveBeenCalledOnce();
  expect(install).toHaveBeenCalledOnce();
});

test("the update choices load from the main process", async () => {
  vi.spyOn(bridge.updates, "getSettings").mockResolvedValue({ autoDownload: false, channel: "nightly" });

  const { result } = renderHook(() => useUpdateSettings());

  await waitFor(() => expect(result.current.settings).toEqual({ autoDownload: false, channel: "nightly" }));
});

test("a change shows at once and keeps what the main process saved", async () => {
  vi.spyOn(bridge.updates, "getSettings").mockResolvedValue({ autoDownload: true, channel: "stable" });
  let answer: (settings: UpdateSettings) => void = () => undefined;
  vi.spyOn(bridge.updates, "setSettings").mockImplementation(
    () =>
      new Promise((resolve) => {
        answer = resolve;
      }),
  );
  const { result } = renderHook(() => useUpdateSettings());
  await waitFor(() => expect(result.current.settings).not.toBeNull());

  let saving: Promise<void> = Promise.resolve();
  act(() => {
    saving = result.current.save({ channel: "nightly" });
  });
  expect(result.current.settings).toEqual({ autoDownload: true, channel: "nightly" });

  await act(async () => {
    answer({ autoDownload: true, channel: "nightly" });
    await saving;
  });
  expect(result.current.settings).toEqual({ autoDownload: true, channel: "nightly" });
});

test("a change the main process refuses goes back", async () => {
  vi.spyOn(bridge.updates, "getSettings").mockResolvedValue({ autoDownload: true, channel: "stable" });
  vi.spyOn(bridge.updates, "setSettings").mockRejectedValue(new Error("no handler"));
  const { result } = renderHook(() => useUpdateSettings());
  await waitFor(() => expect(result.current.settings).not.toBeNull());

  await act(() => result.current.save({ autoDownload: false }));

  expect(result.current.settings).toEqual({ autoDownload: true, channel: "stable" });
});
