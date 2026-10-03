import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { renderWithProviders } from "@test/test-utils";
import { useSidebar } from "@/components/ui/sidebar";
import { PanelToggle, PanelToggleSpace } from "./panel-toggle";

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

function Toggle() {
  const [open, setOpen] = useState(false);
  return <PanelToggle label="Watch settings" open={open} onOpenChange={setOpen} />;
}

test("the same button opens and closes the panel", async () => {
  const user = userEvent.setup();
  renderWithProviders(<Toggle />);

  const toggle = screen.getByRole("button", { name: "Watch settings" });
  expect(toggle).toHaveAttribute("aria-expanded", "false");

  await user.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "true");

  await user.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "false");
});

test.each([
  { mac: true, keys: { metaKey: true, altKey: true, key: "∫", code: "KeyB" }, toggles: true },
  { mac: true, keys: { metaKey: true, key: "b", code: "KeyB" }, toggles: false },
  { mac: true, keys: { metaKey: true, shiftKey: true, key: "b", code: "KeyB" }, toggles: false },
  { mac: true, keys: { ctrlKey: true, altKey: true, key: "∫", code: "KeyB" }, toggles: false },
  { mac: true, keys: { metaKey: true, altKey: true, shiftKey: true, key: "ı", code: "KeyB" }, toggles: false },
  { mac: true, keys: { metaKey: true, altKey: true, key: "≈", code: "KeyX" }, toggles: false },
  { mac: false, keys: { ctrlKey: true, altKey: true, key: "b", code: "KeyB" }, toggles: true },
  { mac: false, keys: { ctrlKey: true, altKey: true, key: "{", code: "KeyB", modifierAltGraph: true }, toggles: false },
  { mac: false, keys: { ctrlKey: true, key: "b", code: "KeyB" }, toggles: false },
  { mac: false, keys: { metaKey: true, altKey: true, key: "b", code: "KeyB" }, toggles: false },
])("on macOS $mac, $keys toggles the panel: $toggles", ({ mac, keys, toggles }) => {
  platform.isMac = mac;
  renderWithProviders(<Toggle />);

  fireEvent.keyDown(window, keys);

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveAttribute("aria-expanded", String(toggles));
});

test("the shortcut opens and closes the panel", () => {
  renderWithProviders(<Toggle />);
  const toggle = screen.getByRole("button", { name: "Watch settings" });

  fireEvent.keyDown(window, { ctrlKey: true, altKey: true, key: "b", code: "KeyB" });
  expect(toggle).toHaveAttribute("aria-expanded", "true");

  fireEvent.keyDown(window, { ctrlKey: true, altKey: true, key: "b", code: "KeyB" });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
});

function SidebarState() {
  const { state } = useSidebar();
  return <output aria-label="Sidebar">{state}</output>;
}

test("the shortcut leaves the left sidebar alone", () => {
  platform.isMac = true;
  renderWithProviders(
    <>
      <Toggle />
      <SidebarState />
    </>,
    { withSidebar: true },
  );

  fireEvent.keyDown(document.body, { metaKey: true, altKey: true, key: "∫", code: "KeyB" });

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("status", { name: "Sidebar" })).toHaveTextContent("expanded");
});

test("the left sidebar keeps its own shortcut", () => {
  platform.isMac = true;
  renderWithProviders(
    <>
      <Toggle />
      <SidebarState />
    </>,
    { withSidebar: true },
  );

  fireEvent.keyDown(document.body, { metaKey: true, key: "b" });

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByRole("status", { name: "Sidebar" })).toHaveTextContent("collapsed");
});

test("the shortcut does not toggle the panel over a dialog", () => {
  render(<div role="dialog" aria-label="Settings" />);
  renderWithProviders(<Toggle />);

  fireEvent.keyDown(window, { ctrlKey: true, altKey: true, key: "b", code: "KeyB" });

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveAttribute("aria-expanded", "false");
});

test("off macOS stays clear of the window buttons", () => {
  renderWithProviders(<Toggle />);

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveClass("fixed", "right-window-controls");
});

test("on macOS keeps the inset of the header", () => {
  platform.isMac = true;

  renderWithProviders(<Toggle />);

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveClass("fixed", "right-5");
});

test("the space under the toggle takes its place, lets clicks through and stops the window drag", () => {
  const { container } = renderWithProviders(
    <>
      <Toggle />
      <PanelToggleSpace />
    </>,
  );

  const toggle = screen.getByRole("button", { name: "Watch settings" });
  const space = container.querySelector('[data-slot="panel-toggle-space"]');
  expect(space).toHaveAttribute("aria-hidden", "true");
  expect(space).toHaveClass("fixed", "top-titlebar-button-top", "size-7", "right-window-controls");
  expect(space).toHaveClass("pointer-events-none", "app-no-drag");
  expect(toggle).toHaveClass("fixed", "top-titlebar-button-top", "size-7", "right-window-controls");
});
