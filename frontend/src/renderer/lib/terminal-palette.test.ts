import { afterEach, expect, test } from "vite-plus/test";
import { parseColor, terminalTheme } from "./terminal-palette";

function hostWithTokens(tokens: Record<string, string>) {
  const host = document.createElement("div");
  host.style.backgroundColor = "rgb(255, 255, 255)";
  host.style.color = "rgb(31, 35, 40)";
  for (const [token, value] of Object.entries(tokens)) host.style.setProperty(token, value);
  document.body.append(host);
  return host;
}

afterEach(() => {
  document.body.replaceChildren();
});

test("takes the canvas from the element the terminal sits in", () => {
  const host = hostWithTokens({});

  const theme = terminalTheme(getComputedStyle(host));

  expect(theme.background).toEqual({ r: 255, g: 255, b: 255 });
  expect(theme.foreground).toEqual({ r: 31, g: 35, b: 40 });
  expect(theme.cursor).toEqual({ r: 31, g: 35, b: 40 });
});

test("takes every ansi slot from the tokens in force", () => {
  const host = hostWithTokens({
    "--ansi-green": "#116329",
    "--ansi-bright-green": "#1a7f37",
    "--terminal-selection": "#54aeff66",
  });

  const theme = terminalTheme(getComputedStyle(host));

  expect(theme.ansi[2]).toEqual({ r: 17, g: 99, b: 41 });
  expect(theme.ansi[10]).toEqual({ r: 26, g: 127, b: 55 });
  expect(theme.selectionBackground).toBe("#54aeff66");
});

test("carries all sixteen ansi slots", () => {
  const host = hostWithTokens({});

  const theme = terminalTheme(getComputedStyle(host));

  expect(theme.ansi).toHaveLength(16);
});

test.each([
  ["#fff", { r: 255, g: 255, b: 255 }],
  ["#0d1117", { r: 13, g: 17, b: 23 }],
  ["#54aeff66", { r: 84, g: 174, b: 255 }],
  ["rgb(13, 17, 23)", { r: 13, g: 17, b: 23 }],
  ["rgba(13, 17, 23, 0.5)", { r: 13, g: 17, b: 23 }],
  ["", { r: 0, g: 0, b: 0 }],
  ["#zzzzzz", { r: 0, g: 0, b: 0 }],
])("reads %s as a color", (value, color) => {
  expect(parseColor(value)).toEqual(color);
});
