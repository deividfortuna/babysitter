import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
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

function activeView() {
  return screen.getAllByRole("button").find((button) => button.dataset.active === "true")?.textContent;
}

test("goes back to the previous view and forward again", async () => {
  const user = userEvent.setup();
  renderWithProviders(<App />);
  await screen.findByRole("button", { name: "Start the daemon" });
  expect(activeView()).toBe("Watching");
  expect(screen.getByRole("button", { name: "Go back" })).toBeDisabled();

  await user.click(screen.getByRole("button", { name: "Stopped" }));
  expect(activeView()).toBe("Stopped");

  await user.click(screen.getByRole("button", { name: "Go back" }));
  expect(activeView()).toBe("Watching");
  expect(screen.getByRole("button", { name: "Go back" })).toBeDisabled();

  await user.click(screen.getByRole("button", { name: "Go forward" }));
  expect(activeView()).toBe("Stopped");
  expect(screen.getByRole("button", { name: "Go forward" })).toBeDisabled();
});

test("on macOS goes back with Command and the left bracket", async () => {
  platform.isMac = true;
  const user = userEvent.setup();
  renderWithProviders(<App />);
  await screen.findByRole("button", { name: "Start the daemon" });

  await user.click(screen.getByRole("button", { name: "Stopped" }));
  await user.keyboard("{Meta>}[[{/Meta}");

  expect(activeView()).toBe("Watching");
});

test("on macOS Command and P opens the palette, which goes to a view", async () => {
  platform.isMac = true;
  const user = userEvent.setup();
  renderWithProviders(<App />);
  await screen.findByRole("button", { name: "Start the daemon" });

  await user.keyboard("{Meta>}p{/Meta}");
  await user.click(await screen.findByRole("option", { name: "Stopped" }));

  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(activeView()).toBe("Stopped");
});

test("the palette opens with an empty query each time", async () => {
  const user = userEvent.setup();
  renderWithProviders(<App />);
  await screen.findByRole("button", { name: "Start the daemon" });

  await user.keyboard("{Control>}p{/Control}");
  await user.type(await screen.findByRole("combobox"), ">sync");
  await user.keyboard("{Escape}");
  await user.keyboard("{Control>}p{/Control}");

  expect(await screen.findByRole("combobox")).toHaveValue("");
});

test("Control and P on the loading screen does not open the palette once the daemon is up", async () => {
  let emit: (status: DaemonStatus) => void = () => undefined;
  vi.spyOn(bridge.daemon, "getStatus").mockResolvedValue({ state: "starting", step: "environment" });
  vi.spyOn(bridge.daemon, "onStatus").mockImplementation((listener) => {
    emit = listener;
    return () => undefined;
  });
  const user = userEvent.setup();
  renderWithProviders(<App />);
  await screen.findByRole("status");

  await user.keyboard("{Control>}p{/Control}");
  act(() => emit({ state: "error", message: "The daemon stopped (exit code 1)." }));

  expect(await screen.findByRole("button", { name: "Start the daemon" })).toBeInTheDocument();
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
});

test("Control and P does nothing while another dialog is open", async () => {
  const user = userEvent.setup();
  renderWithProviders(<App />);
  await screen.findByRole("button", { name: "Start the daemon" });

  await user.click(screen.getByRole("button", { name: "Settings" }));
  await screen.findByRole("dialog");
  await user.keyboard("{Control>}p{/Control}");

  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
});

test("a remote paired again at a new address starts over as a new daemon", async () => {
  const user = userEvent.setup();
  const atAddress = (url: string): DaemonStatus => ({
    state: "error",
    message: "No babysitter daemon answers.",
    connection: { id: "r1", kind: "remote", name: "studio", url },
  });
  let emit: (status: DaemonStatus) => void = () => undefined;
  vi.spyOn(bridge.daemon, "getStatus").mockResolvedValue(atAddress("http://192.168.1.20:7420"));
  vi.spyOn(bridge.daemon, "onStatus").mockImplementation((listener) => {
    emit = listener;
    return () => undefined;
  });
  renderWithProviders(<App />);
  await user.click(await screen.findByRole("button", { name: "Stopped" }));
  expect(activeView()).toBe("Stopped");

  act(() => emit(atAddress("http://192.168.1.30:7420")));

  expect(activeView()).toBe("Watching");
});
