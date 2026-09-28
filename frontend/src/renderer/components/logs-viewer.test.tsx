import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { FakeEventSource } from "@test/fake-event-source";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { bridge } from "@/lib/bridge";
import type { LogRecord } from "../../shared/logs";
import { LogsViewer } from "./logs-viewer";

afterEach(() => {
  vi.restoreAllMocks();
});

function record(seq: number, overrides: Partial<LogRecord> = {}): LogRecord {
  return { seq, time: "2026-09-28T10:00:00.000Z", level: "info", msg: `record ${seq}`, attrs: [], ...overrides };
}

async function openDaemonStream() {
  const stream = await waitFor(() => {
    const es = FakeEventSource.instances.at(-1);
    if (!es) throw new Error("no stream yet");
    return es;
  });
  act(() => stream.dispatch("ready"));
  return stream;
}

function logLines() {
  return within(screen.getByRole("log", { name: "Log records" }));
}

test("shows what the daemon streams, from the start of the records it keeps", async () => {
  serveApi();
  renderWithProviders(<LogsViewer />);

  const stream = await openDaemonStream();
  act(() => {
    stream.dispatch("log", record(1, { msg: "daemon listening", attrs: [{ key: "addr", value: "127.0.0.1:4000" }] }));
    stream.dispatch("log", record(2, { level: "error", msg: "poll failed" }));
  });

  expect(stream.url).toBe("http://127.0.0.1:8080/logs/stream?after=0");
  expect(await logLines().findByText("daemon listening")).toBeVisible();
  expect(logLines().getByText("addr=127.0.0.1:4000")).toBeVisible();
  expect(logLines().getByText("poll failed")).toBeVisible();
});

test("says it waits while the daemon does not answer", () => {
  renderWithProviders(<LogsViewer />);

  expect(logLines().getByText("Waiting for the daemon.")).toBeVisible();
});

test("starts over when the daemon restarts, since its records count from 1 again", async () => {
  serveApi();
  renderWithProviders(<LogsViewer />);
  const stream = await openDaemonStream();
  act(() => stream.dispatch("log", record(1, { msg: "before the restart" })));
  await logLines().findByText("before the restart");

  act(() => {
    stream.dispatch("ready");
    stream.dispatch("log", record(1, { msg: "after the restart" }));
  });

  expect(await logLines().findByText("after the restart")).toBeVisible();
  expect(logLines().queryByText("before the restart")).not.toBeInTheDocument();
});

test("keeps the records at the level chosen that carry the text typed", async () => {
  serveApi();
  const user = userEvent.setup();
  renderWithProviders(<LogsViewer />);
  const stream = await openDaemonStream();
  act(() => {
    stream.dispatch(
      "log",
      record(1, { level: "debug", msg: "http", attrs: [{ key: "path", value: "/api/v1/watches" }] }),
    );
    stream.dispatch("log", record(2, { level: "warn", msg: "poll failed", attrs: [{ key: "watch", value: "7" }] }));
    stream.dispatch("log", record(3, { level: "warn", msg: "poll failed", attrs: [{ key: "watch", value: "9" }] }));
  });
  await logLines().findByText("http");

  await user.click(screen.getByLabelText("Lowest level shown"));
  await user.click(await screen.findByRole("option", { name: "Warnings and errors" }));
  expect(logLines().queryByText("http")).not.toBeInTheDocument();
  expect(logLines().getAllByText("poll failed")).toHaveLength(2);

  await user.type(screen.getByRole("searchbox", { name: "Filter the log" }), "watch=9");
  expect(logLines().getAllByText("poll failed")).toHaveLength(1);
  expect(logLines().getByText("watch=9")).toBeVisible();
});

test("copies the records it shows", async () => {
  serveApi();
  const user = userEvent.setup();
  renderWithProviders(<LogsViewer />);
  const stream = await openDaemonStream();
  act(() => stream.dispatch("log", record(1, { msg: "daemon listening", attrs: [{ key: "pid", value: "42" }] })));
  await logLines().findByText("daemon listening");

  await user.click(screen.getByRole("button", { name: "Copy" }));

  expect(await navigator.clipboard.readText()).toBe("2026-09-28T10:00:00.000Z INFO daemon listening pid=42");
});

test("shows the log of the app with the records it keeps and each new one", async () => {
  let push: (record: LogRecord) => void = () => undefined;
  vi.spyOn(bridge.logs, "appRecords").mockResolvedValue([record(1, { msg: "daemon: spawning babysitter" })]);
  vi.spyOn(bridge.logs, "onAppRecord").mockImplementation((listener) => {
    push = listener;
    return () => undefined;
  });
  const user = userEvent.setup();
  renderWithProviders(<LogsViewer />);

  await user.click(screen.getByRole("radio", { name: "App" }));

  expect(await logLines().findByText("daemon: spawning babysitter")).toBeVisible();
  act(() => push(record(2, { msg: "daemon: ready on port 4000" })));
  expect(await logLines().findByText("daemon: ready on port 4000")).toBeVisible();
});

test("offers the log folder only inside the desktop app", () => {
  renderWithProviders(<LogsViewer />);

  expect(screen.queryByRole("button", { name: "Open folder" })).not.toBeInTheDocument();
});
