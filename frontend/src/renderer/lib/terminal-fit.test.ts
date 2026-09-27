import { afterEach, expect, test } from "vite-plus/test";
import { scrollingAncestor, terminalBox } from "./terminal-fit";

function element(size: Partial<Record<"clientWidth" | "clientHeight" | "offsetHeight", number>>) {
  const el = document.createElement("div");
  for (const [key, value] of Object.entries(size)) Object.defineProperty(el, key, { value });
  return el;
}

afterEach(() => {
  document.body.replaceChildren();
});

test("finds the nearest ancestor that scrolls", () => {
  const outer = document.createElement("div");
  outer.style.overflowY = "auto";
  const middle = document.createElement("div");
  const inner = document.createElement("div");
  outer.append(middle);
  middle.append(inner);
  document.body.append(outer);

  expect(scrollingAncestor(inner)).toBe(outer);
});

test("falls back to the document when nothing scrolls", () => {
  const inner = document.createElement("div");
  document.body.append(inner);

  expect(scrollingAncestor(inner)).toBe(document.documentElement);
});

test("gives the terminal the width of its pane without the padding", () => {
  const scroller = element({ clientWidth: 900 });
  scroller.style.padding = "12px";

  const box = terminalBox({
    scroller,
    panel: element({ offsetHeight: 500 }),
    viewport: element({ clientHeight: 800 }),
    terminalHeight: 390,
  });

  expect(box.width).toBe(876);
});

test("gives the terminal the height of the window left by the panel around it", () => {
  const box = terminalBox({
    scroller: element({ clientWidth: 900 }),
    panel: element({ offsetHeight: 500 }),
    viewport: element({ clientHeight: 800 }),
    terminalHeight: 390,
  });

  expect(box.height).toBe(690);
});
