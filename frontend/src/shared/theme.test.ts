import { expect, test } from "vite-plus/test";
import { canvasColor, CANVAS, INK, windowControlsColors } from "./theme";

test("a window paints in the theme the user chose, not the one of the system", () => {
  expect(canvasColor("dark", false)).toBe(CANVAS.dark);
  expect(canvasColor("light", true)).toBe(CANVAS.light);
});

test("only a preference of system asks the system scheme", () => {
  expect(canvasColor("system", true)).toBe(CANVAS.dark);
  expect(canvasColor("system", false)).toBe(CANVAS.light);
});

test("the window buttons paint on the canvas with the ink of the same theme", () => {
  expect(windowControlsColors("dark", false)).toEqual({ color: CANVAS.dark, symbolColor: INK.dark });
  expect(windowControlsColors("system", false)).toEqual({ color: CANVAS.light, symbolColor: INK.light });
});
