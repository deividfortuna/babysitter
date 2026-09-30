import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { TerminalPanelResizeHandle } from "./terminal-panel-resize-handle";

test("resizes the terminal with keyboard controls", async () => {
  const onResize = vi.fn();
  const user = userEvent.setup();
  render(<TerminalPanelResizeHandle height={420} maxHeight={1200} onResize={onResize} onReset={vi.fn()} />);

  const handle = screen.getByRole("separator", { name: "Resize terminal" });
  await user.tab();
  expect(handle).toHaveFocus();
  await user.keyboard("{ArrowUp}");

  expect(onResize).toHaveBeenCalledWith(436);
});

test("grows the terminal to the maximum height of the window with End", async () => {
  const onResize = vi.fn();
  const user = userEvent.setup();
  render(<TerminalPanelResizeHandle height={420} maxHeight={720} onResize={onResize} onReset={vi.fn()} />);

  const handle = screen.getByRole("separator", { name: "Resize terminal" });
  expect(handle).toHaveAttribute("aria-valuemax", "720");
  await user.tab();
  await user.keyboard("{End}");

  expect(onResize).toHaveBeenCalledWith(720);
});

test("resets the terminal height when the resize handle is double clicked", async () => {
  const onReset = vi.fn();
  const user = userEvent.setup();
  Object.defineProperty(HTMLElement.prototype, "setPointerCapture", { configurable: true, value: vi.fn() });
  Object.defineProperty(HTMLElement.prototype, "releasePointerCapture", { configurable: true, value: vi.fn() });
  render(<TerminalPanelResizeHandle height={300} maxHeight={1200} onResize={vi.fn()} onReset={onReset} />);

  await user.dblClick(screen.getByRole("separator", { name: "Resize terminal" }));

  expect(onReset).toHaveBeenCalledOnce();
});
