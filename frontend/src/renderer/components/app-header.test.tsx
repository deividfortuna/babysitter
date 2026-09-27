import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { renderWithProviders } from "@test/test-utils";
import { AppHeader, TitlebarSidebarTrigger } from "./app-header";

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

test("keeps the sidebar toggle in the same header when the sidebar collapses", async () => {
  const user = userEvent.setup();
  renderWithProviders(<AppHeader />, { withSidebar: true });

  const header = screen.getByRole("banner");
  const toggle = screen.getByRole("button", { name: "Toggle Sidebar" });
  expect(header).toContainElement(toggle);

  await user.click(toggle);

  expect(screen.getByRole("banner")).toBe(header);
  expect(header).toContainElement(screen.getByRole("button", { name: "Toggle Sidebar" }));
});

test("on macOS keeps the sidebar toggle beside the window buttons when the sidebar is open or collapsed", async () => {
  platform.isMac = true;
  const user = userEvent.setup();
  renderWithProviders(
    <>
      <AppHeader />
      <TitlebarSidebarTrigger />
    </>,
    { withSidebar: true },
  );

  expect(screen.queryByRole("banner")).not.toBeInTheDocument();

  const toggle = screen.getByRole("button", { name: "Toggle Sidebar" });
  expect(toggle).toHaveClass("fixed", "left-20");

  await user.click(toggle);

  expect(screen.getByRole("button", { name: "Toggle Sidebar" })).toHaveClass("fixed", "left-20");
});
