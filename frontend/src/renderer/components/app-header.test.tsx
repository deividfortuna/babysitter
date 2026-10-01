import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { renderWithProviders } from "@test/test-utils";
import { useSidebar } from "@/components/ui/sidebar";
import { bridge } from "@/lib/bridge";
import type { HistoryControls } from "@/hooks/use-view-history";
import { TitlebarNav } from "./app-header";

const platform = vi.hoisted(() => ({ isMac: false, isWindows: false }));

vi.mock("@/lib/platform", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/platform")>()),
  get isMac() {
    return platform.isMac;
  },
  get isWindows() {
    return platform.isWindows;
  },
}));

afterEach(() => {
  platform.isMac = false;
  platform.isWindows = false;
  vi.restoreAllMocks();
});

function history(overrides: Partial<HistoryControls> = {}): HistoryControls {
  return { canGoBack: false, canGoForward: false, onBack: vi.fn(), onForward: vi.fn(), ...overrides };
}

function titlebarNav() {
  return screen.getByRole("button", { name: "Toggle Sidebar" }).closest("[data-slot=titlebar-nav]");
}

test("off Windows puts back and forward after the sidebar toggle", () => {
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });

  const buttons = screen
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? button.textContent);
  expect(buttons).toEqual(["Toggle Sidebar", "Go back", "Go forward"]);
});

test("on Windows puts the menu of the app before the sidebar toggle", () => {
  platform.isWindows = true;
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });

  const buttons = screen
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? button.textContent);
  expect(buttons).toEqual(["Menu", "Toggle Sidebar", "Go back", "Go forward"]);
});

test("on Windows opens the menu of the app under its button", async () => {
  platform.isWindows = true;
  const popupMenu = vi.spyOn(bridge.app, "popupMenu");
  const user = userEvent.setup();
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });
  const button = screen.getByRole("button", { name: "Menu" });
  vi.spyOn(button, "getBoundingClientRect").mockReturnValue(new DOMRect(12, 10, 28, 28));

  await user.click(button);

  expect(popupMenu).toHaveBeenCalledWith({ x: 12, y: 38 });
});

test("disables back and forward when the history cannot move", () => {
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });

  expect(screen.getByRole("button", { name: "Go back" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Go forward" })).toBeDisabled();
});

test("goes back and forward when the history can move", async () => {
  const user = userEvent.setup();
  const controls = history({ canGoBack: true, canGoForward: true });
  renderWithProviders(<TitlebarNav {...controls} />, { withSidebar: true });

  await user.click(screen.getByRole("button", { name: "Go back" }));
  await user.click(screen.getByRole("button", { name: "Go forward" }));

  expect(controls.onBack).toHaveBeenCalledOnce();
  expect(controls.onForward).toHaveBeenCalledOnce();
});

test("shows the shortcut in the tooltip of back", async () => {
  const user = userEvent.setup();
  renderWithProviders(<TitlebarNav {...history({ canGoBack: true })} />, { withSidebar: true });

  await user.hover(screen.getByRole("button", { name: "Go back" }));

  expect(await screen.findByRole("tooltip")).toHaveTextContent("Go back Alt+←");
});

function SidebarState() {
  const { open } = useSidebar();
  return <output>{open ? "open" : "collapsed"}</output>;
}

test("shows the shortcut in the tooltip of the sidebar toggle", async () => {
  const user = userEvent.setup();
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });

  await user.hover(screen.getByRole("button", { name: "Toggle Sidebar" }));

  expect(await screen.findByRole("tooltip")).toHaveTextContent("Toggle sidebar Ctrl+B");
});

test("on macOS shows the command key in the tooltip of the sidebar toggle", async () => {
  platform.isMac = true;
  const user = userEvent.setup();
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });

  await user.hover(screen.getByRole("button", { name: "Toggle Sidebar" }));

  expect(await screen.findByRole("tooltip")).toHaveTextContent("Toggle sidebar ⌘B");
});

test.each([
  ["Ctrl", "{Control>}b{/Control}"],
  ["⌘", "{Meta>}b{/Meta}"],
])("the %s+B shortcut collapses and opens the sidebar", async (_, keys) => {
  const user = userEvent.setup();
  renderWithProviders(
    <>
      <TitlebarNav {...history()} />
      <SidebarState />
    </>,
    { withSidebar: true },
  );
  expect(screen.getByRole("status")).toHaveTextContent("open");

  await user.keyboard(keys);
  expect(screen.getByRole("status")).toHaveTextContent("collapsed");

  await user.keyboard(keys);
  expect(screen.getByRole("status")).toHaveTextContent("open");
});

test("keeps the sidebar toggle, back and forward in the title bar when the sidebar is open or collapsed", async () => {
  const user = userEvent.setup();
  renderWithProviders(<TitlebarNav {...history()} />, { withSidebar: true });

  const cluster = titlebarNav();
  expect(cluster).toHaveClass("fixed", "left-titlebar-nav-left");
  expect(cluster).toContainElement(screen.getByRole("button", { name: "Go back" }));
  expect(cluster).toContainElement(screen.getByRole("button", { name: "Go forward" }));

  await user.click(screen.getByRole("button", { name: "Toggle Sidebar" }));

  expect(titlebarNav()).toHaveClass("fixed", "left-titlebar-nav-left");
});
