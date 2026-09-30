import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { TerminalPanelResizeHandle } from "./terminal-panel-resize-handle";

test("resizes the terminal with keyboard controls", async () => {
  const onResize = vi.fn();
  const user = userEvent.setup();
  render(<TerminalPanelResizeHandle height={420} onResize={onResize} onReset={vi.fn()} />);

  const handle = screen.getByRole("separator", { name: "Resize terminal" });
  await user.tab();
  expect(handle).toHaveFocus();
  await user.keyboard("{ArrowUp}");

  expect(onResize).toHaveBeenCalledWith(436);
});

test("resets the terminal height when the resize handle is double clicked", async () => {
  const onReset = vi.fn();
  const user = userEvent.setup();
  Object.defineProperty(HTMLElement.prototype, "setPointerCapture", { configurable: true, value: vi.fn() });
  Object.defineProperty(HTMLElement.prototype, "releasePointerCapture", { configurable: true, value: vi.fn() });
  render(<TerminalPanelResizeHandle height={300} onResize={vi.fn()} onReset={onReset} />);

  await user.dblClick(screen.getByRole("separator", { name: "Resize terminal" }));

  expect(onReset).toHaveBeenCalledOnce();
});
