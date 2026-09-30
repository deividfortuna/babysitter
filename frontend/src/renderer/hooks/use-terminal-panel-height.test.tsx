import { act, renderHook } from "@testing-library/react";
import { expect, test, vi } from "vite-plus/test";
import { useTerminalPanelHeight } from "./use-terminal-panel-height";

test("keeps the panel within 80% of the window", () => {
  vi.stubGlobal("innerHeight", 900);
  const { result } = renderHook(() => useTerminalPanelHeight());

  act(() => result.current.setHeight(1200));

  expect(result.current.maxHeight).toBe(720);
  expect(result.current.height).toBe(720);
});

test("follows the window when it is resized", () => {
  window.localStorage.setItem("terminal_panel_height", "1000");
  vi.stubGlobal("innerHeight", 900);
  const { result } = renderHook(() => useTerminalPanelHeight());
  expect(result.current.height).toBe(720);

  vi.stubGlobal("innerHeight", 1500);
  act(() => {
    window.dispatchEvent(new Event("resize"));
  });

  expect(result.current.maxHeight).toBe(1200);
  expect(result.current.height).toBe(1000);
});
