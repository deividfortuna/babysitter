import { act, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { bridge } from "@/lib/bridge";
import type { DaemonStatus } from "../shared/daemon-status";
import { renderWithProviders } from "@test/test-utils";
import { App } from "./App";

const platform = vi.hoisted(() => ({ isMac: false }));

vi.mock("@/lib/platform", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/platform")>()),
  get isMac() {
    return platform.isMac;
  },
}));

afterEach(() => {
  platform.isMac = false;
  vi.restoreAllMocks();
});

test("while the daemon starts, shows the loading screen in place of the sidebar and its account", async () => {
  vi.spyOn(bridge.daemon, "getStatus").mockResolvedValue({ state: "starting", step: "environment" });
  renderWithProviders(<App />);

  expect(await screen.findByRole("status")).toHaveTextContent("Reading your shell environment");
  expect(screen.queryByRole("button", { name: "Toggle Sidebar" })).not.toBeInTheDocument();
  expect(screen.queryByText("No account")).not.toBeInTheDocument();
});

test("a restart after the first start keeps the sidebar and shows the start in the pane", async () => {
  let emit: (status: DaemonStatus) => void = () => undefined;
  vi.spyOn(bridge.daemon, "getStatus").mockResolvedValue({
    state: "error",
    message: "The daemon stopped (exit code 1).",
  });
  vi.spyOn(bridge.daemon, "onStatus").mockImplementation((listener) => {
    emit = listener;
    return () => undefined;
  });
  renderWithProviders(<App />);
  await screen.findByRole("button", { name: "Start the daemon" });

  act(() => emit({ state: "starting", step: "daemon" }));

  expect(screen.getByRole("button", { name: "Toggle Sidebar" })).toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: "Babysitter" })).not.toBeInTheDocument();
  expect(screen.getByText("Starting the daemon…")).toBeInTheDocument();
});

test("on macOS renders the sidebar toggle after the view header, so the drag region of the header does not cover the toggle", async () => {
  platform.isMac = true;
  renderWithProviders(<App />);

  const viewHeader = await screen.findByRole("banner");
  const toggle = screen.getByRole("button", { name: "Toggle Sidebar" });

  expect(viewHeader).toHaveClass("app-drag");
  expect(viewHeader.compareDocumentPosition(toggle) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});
