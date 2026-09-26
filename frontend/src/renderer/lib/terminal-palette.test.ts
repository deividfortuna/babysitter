import { afterEach, expect, test } from "vitest";
import { terminalTheme } from "./terminal-palette";

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

  expect(theme.background).toBe("rgb(255, 255, 255)");
  expect(theme.foreground).toBe("rgb(31, 35, 40)");
  expect(theme.cursor).toBe("rgb(31, 35, 40)");
});

test("takes every ansi slot from the tokens in force", () => {
  const host = hostWithTokens({
    "--ansi-green": "#116329",
    "--ansi-bright-green": "#1a7f37",
    "--terminal-selection": "#54aeff66",
  });

  const theme = terminalTheme(getComputedStyle(host));

  expect(theme.green).toBe("#116329");
  expect(theme.brightGreen).toBe("#1a7f37");
  expect(theme.selectionBackground).toBe("#54aeff66");
});

test("carries all sixteen ansi slots", () => {
  const host = hostWithTokens({});

  const theme = terminalTheme(getComputedStyle(host));

  expect(Object.keys(theme)).toHaveLength(20);
  expect(theme).toHaveProperty("brightWhite");
});
