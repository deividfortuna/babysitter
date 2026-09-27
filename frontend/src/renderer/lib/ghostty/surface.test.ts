import { expect, test } from "vite-plus/test";
import { GhosttySurface, scrollbarThumb, terminalGrid, wheelRows } from "./surface";

const theme = {
  foreground: { r: 0, g: 0, b: 0 },
  background: { r: 255, g: 255, b: 255 },
  cursor: { r: 0, g: 0, b: 0 },
  ansi: [],
  selectionBackground: "#54aeff66",
};

test("has no thumb while everything fits on the screen", () => {
  expect(scrollbarThumb({ total: 30, offset: 0, len: 30 }, 300)).toBeNull();
});

test("sizes the thumb to the share of the scrollback on screen", () => {
  expect(scrollbarThumb({ total: 120, offset: 90, len: 30 }, 300)).toEqual({ height: 75, top: 225 });
  expect(scrollbarThumb({ total: 120, offset: 0, len: 30 }, 300)).toEqual({ height: 75, top: 0 });
});

test("keeps a thumb large enough to see on a long scrollback", () => {
  expect(scrollbarThumb({ total: 5000, offset: 0, len: 30 }, 300)?.height).toBe(18);
});

test("turns wheel pixels, lines and pages into rows", () => {
  expect(wheelRows({ deltaY: 40, deltaMode: 0 }, 20, 30)).toBe(2);
  expect(wheelRows({ deltaY: 3, deltaMode: 1 }, 20, 30)).toBe(3);
  expect(wheelRows({ deltaY: -1, deltaMode: 2 }, 20, 30)).toBe(-30);
});

const cell = { width: 6.6, height: 13 };
const limits = { minCols: 20, minRows: 30, maxRows: 60 };

test("fits as many whole cells as the box holds", () => {
  expect(terminalGrid({ width: 1000, height: 600 }, cell, limits)).toEqual({ cols: 151, rows: 46 });
});

test("never goes under the minimum grid", () => {
  expect(terminalGrid({ width: 60, height: 200 }, cell, limits)).toEqual({ cols: 20, rows: 30 });
});

test("never goes over the most rows", () => {
  expect(terminalGrid({ width: 1000, height: 1400 }, cell, limits)).toEqual({ cols: 151, rows: 60 });
});

test("does not start without a canvas", async () => {
  const frame = document.createElement("div");
  const scroller = document.createElement("div");

  const surface = await GhosttySurface.create(
    { frame, scroller },
    { limits: { minCols: 10, minRows: 2, maxRows: 4 }, theme },
  );

  expect(surface).toBeNull();
  expect(scroller.childElementCount).toBe(0);
});
