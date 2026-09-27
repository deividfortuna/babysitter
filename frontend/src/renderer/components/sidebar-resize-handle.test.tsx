import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { SidebarResizeHandle } from "./sidebar-resize-handle";

test("resizes the sidebar with keyboard controls", async () => {
  const onResize = vi.fn();
  const user = userEvent.setup();
  render(<SidebarResizeHandle width={256} onResize={onResize} onReset={vi.fn()} />);

  const handle = screen.getByRole("separator", { name: "Resize sidebar" });
  await user.tab();
  expect(handle).toHaveFocus();
  await user.keyboard("{ArrowRight}");

  expect(onResize).toHaveBeenCalledWith(272);
});

test("resets the sidebar width when the resize handle is double clicked", async () => {
  const onReset = vi.fn();
  const user = userEvent.setup();
  Object.defineProperty(HTMLElement.prototype, "setPointerCapture", { configurable: true, value: vi.fn() });
  Object.defineProperty(HTMLElement.prototype, "releasePointerCapture", { configurable: true, value: vi.fn() });
  render(<SidebarResizeHandle width={320} onResize={vi.fn()} onReset={onReset} />);

  await user.dblClick(screen.getByRole("separator", { name: "Resize sidebar" }));

  expect(onReset).toHaveBeenCalledOnce();
});
