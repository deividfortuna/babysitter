import { afterEach, expect, test } from "vite-plus/test";
import { GhosttyTerminalCore, type GhosttySnapshot, type GhosttyTheme } from "./core";
import { loadGhosttyRuntime } from "./runtime";

const ansi = Array.from({ length: 16 }, (_, index) => ({ r: index, g: index * 2, b: index * 3 }));

const theme: GhosttyTheme = {
  foreground: { r: 240, g: 246, b: 252 },
  background: { r: 13, g: 17, b: 23 },
  cursor: { r: 240, g: 246, b: 252 },
  ansi,
  selectionBackground: "#2f5f9950",
};

let core: GhosttyTerminalCore | null = null;

async function terminal(cols = 20, rows = 4) {
  core = new GhosttyTerminalCore(await loadGhosttyRuntime(), cols, rows, theme);
  return core;
}

function rowText(snapshot: GhosttySnapshot, row: number) {
  return (snapshot.rowData[row]?.cells ?? [])
    .map((cell) => cell.text || " ")
    .join("")
    .trimEnd();
}

afterEach(() => {
  core?.dispose();
  core = null;
});

test("draws the text the agent wrote on its grid", async () => {
  const term = await terminal();

  term.write("first line\r\nsecond line");
  const snapshot = term.snapshot();

  expect(snapshot.cols).toBe(20);
  expect(snapshot.rows).toBe(4);
  expect(rowText(snapshot, 0)).toBe("first line");
  expect(rowText(snapshot, 1)).toBe("second line");
});

test("paints the default colors of the theme", async () => {
  const term = await terminal();

  term.write("x");
  const cell = term.snapshot().rowData[0]?.cells[0];

  expect(cell?.foreground).toEqual(theme.foreground);
  expect(cell?.background).toEqual(theme.background);
});

test("takes the ansi colors from the theme palette", async () => {
  const term = await terminal();

  term.write("\u001b[32mgreen\u001b[0m \u001b[1;91mred\u001b[0m");
  const cells = term.snapshot().rowData[0]?.cells ?? [];

  expect(cells[0]?.foreground).toEqual(ansi[2]);
  expect(cells[6]?.foreground).toEqual(ansi[9]);
  expect(cells[6]?.bold).toBe(true);
});

test("follows the palette when the theme changes", async () => {
  const term = await terminal();
  term.write("\u001b[34mblue");

  const lighter = ansi.map((color) => ({ r: color.r + 100, g: color.g + 100, b: color.b + 100 }));
  term.setTheme({ ...theme, ansi: lighter });

  expect(term.snapshot().rowData[0]?.cells[0]?.foreground).toEqual(lighter[4]);
});

test("redraws the screen where a program moves the cursor", async () => {
  const term = await terminal();

  term.write("working...\r\u001b[2Kdone");

  expect(rowText(term.snapshot(), 0)).toBe("done");
});

test("starts over from a clean screen on reset", async () => {
  const term = await terminal();
  term.write("old output");

  term.resetAndWrite("new output");

  expect(rowText(term.snapshot(), 0)).toBe("new output");
});

test("keeps the lines that leave the screen in the scrollback", async () => {
  const term = await terminal(20, 3);

  term.write(Array.from({ length: 10 }, (_, index) => `line ${index}`).join("\r\n"));

  expect(term.scrollbarState()).toEqual({ total: 10, offset: 7, len: 3 });
  term.scroll(-7);
  expect(rowText(term.snapshot(), 0)).toBe("line 0");
  term.scrollToBottom();
  expect(rowText(term.snapshot(), 2)).toBe("line 9");
});

test("gives the text of a selection", async () => {
  const term = await terminal();
  term.write("hello world\r\nsecond");

  term.setSelection({ x: 6, y: 0 }, { x: 10, y: 0 });

  expect(term.selectionText()).toBe("world");
  expect(term.snapshot().rowData[0]?.cells[6]?.selected).toBe(true);
});

test("selects a whole word and a whole line", async () => {
  const term = await terminal();
  term.write("hello world");

  expect(term.selectWord({ x: 7, y: 0 })).toEqual({ start: { x: 6, y: 0 }, end: { x: 10, y: 0 } });
  expect(term.selectionText()).toBe("world");
  term.selectLine({ x: 0, y: 0 });
  expect(term.selectionText()).toBe("hello world");
});

test("has no selection text after the selection is cleared", async () => {
  const term = await terminal();
  term.write("hello");
  term.setSelection({ x: 0, y: 0 }, { x: 4, y: 0 });

  term.clearSelection();

  expect(term.selectionText()).toBe("");
});

test("refuses to work once disposed", async () => {
  const term = await terminal();

  term.dispose();

  expect(() => term.write("late")).toThrow("disposed");
});
