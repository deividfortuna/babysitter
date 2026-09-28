import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { renderWithProviders } from "@test/test-utils";
import type { HistoryControls } from "@/hooks/use-view-history";
import { AppHeader, TitlebarNav } from "./app-header";

const platform = vi.hoisted(() => ({ isMac: false }));

vi.mock("@/lib/platform", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/platform")>()),
  get isMac() {
    return platform.isMac;
  },
}));

afterEach(() => {
  platform.isMac = false;
});

function history(overrides: Partial<HistoryControls> = {}): HistoryControls {
  return { canGoBack: false, canGoForward: false, onBack: vi.fn(), onForward: vi.fn(), ...overrides };
}

function titlebarNav() {
  return screen.getByRole("button", { name: "Toggle Sidebar" }).closest("[data-slot=titlebar-nav]");
}

test("keeps the sidebar toggle in the same header when the sidebar collapses", async () => {
  const user = userEvent.setup();
  renderWithProviders(<AppHeader {...history()} />, { withSidebar: true });

  const header = screen.getByRole("banner");
  const toggle = screen.getByRole("button", { name: "Toggle Sidebar" });
  expect(header).toContainElement(toggle);

  await user.click(toggle);

  expect(screen.getByRole("banner")).toBe(header);
  expect(header).toContainElement(screen.getByRole("button", { name: "Toggle Sidebar" }));
});

test("puts back and forward after the sidebar toggle in the header", () => {
  renderWithProviders(<AppHeader {...history()} />, { withSidebar: true });

  const buttons = screen
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? button.textContent);
  expect(buttons).toEqual(["Toggle Sidebar", "Go back", "Go forward"]);
});

test("disables back and forward when the history cannot move", () => {
  renderWithProviders(<AppHeader {...history()} />, { withSidebar: true });

  expect(screen.getByRole("button", { name: "Go back" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Go forward" })).toBeDisabled();
});

test("goes back and forward when the history can move", async () => {
  const user = userEvent.setup();
  const controls = history({ canGoBack: true, canGoForward: true });
  renderWithProviders(<AppHeader {...controls} />, { withSidebar: true });

  await user.click(screen.getByRole("button", { name: "Go back" }));
  await user.click(screen.getByRole("button", { name: "Go forward" }));

  expect(controls.onBack).toHaveBeenCalledOnce();
  expect(controls.onForward).toHaveBeenCalledOnce();
});

test("shows the shortcut in the tooltip of back", async () => {
  const user = userEvent.setup();
  renderWithProviders(<AppHeader {...history({ canGoBack: true })} />, { withSidebar: true });

  await user.hover(screen.getByRole("button", { name: "Go back" }));

  expect(await screen.findByRole("tooltip")).toHaveTextContent("Go back Alt+←");
});

test("on macOS keeps the sidebar toggle, back and forward beside the window buttons when the sidebar is open or collapsed", async () => {
  platform.isMac = true;
  const user = userEvent.setup();
  renderWithProviders(
    <>
      <AppHeader {...history()} />
      <TitlebarNav {...history()} />
    </>,
    { withSidebar: true },
  );

  expect(screen.queryByRole("banner")).not.toBeInTheDocument();

  const cluster = titlebarNav();
  expect(cluster).toHaveClass("fixed", "left-titlebar-nav-left");
  expect(cluster).toContainElement(screen.getByRole("button", { name: "Go back" }));
  expect(cluster).toContainElement(screen.getByRole("button", { name: "Go forward" }));

  await user.click(screen.getByRole("button", { name: "Toggle Sidebar" }));

  expect(titlebarNav()).toHaveClass("fixed", "left-titlebar-nav-left");
});
