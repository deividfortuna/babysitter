import type { GhosttyColor, GhosttyTheme } from "@/lib/ghostty/core";

const ansiTokens = [
  "--ansi-black",
  "--ansi-red",
  "--ansi-green",
  "--ansi-yellow",
  "--ansi-blue",
  "--ansi-magenta",
  "--ansi-cyan",
  "--ansi-white",
  "--ansi-bright-black",
  "--ansi-bright-red",
  "--ansi-bright-green",
  "--ansi-bright-yellow",
  "--ansi-bright-blue",
  "--ansi-bright-magenta",
  "--ansi-bright-cyan",
  "--ansi-bright-white",
];

const black: GhosttyColor = { r: 0, g: 0, b: 0 };

function hexColor(hex: string): GhosttyColor | null {
  const digits = hex.length <= 4 ? hex.replace(/./g, "$&$&") : hex;
  const channels = digits
    .slice(0, 6)
    .match(/../g)
    ?.map((pair) => Number.parseInt(pair, 16));
  if (channels?.length !== 3 || channels.some(Number.isNaN)) return null;
  const [r = 0, g = 0, b = 0] = channels;
  return { r, g, b };
}

function functionalColor(value: string): GhosttyColor | null {
  const channels = value
    .match(/[\d.]+/g)
    ?.slice(0, 3)
    .map(Number);
  if (channels?.length !== 3) return null;
  const [r = 0, g = 0, b = 0] = channels.map(Math.round);
  return { r, g, b };
}

export function parseColor(value: string): GhosttyColor {
  const color = value.trim();
  if (color.startsWith("#")) return hexColor(color.slice(1)) ?? black;
  if (color.startsWith("rgb")) return functionalColor(color) ?? black;
  return black;
}

export function terminalTheme(style: CSSStyleDeclaration): GhosttyTheme {
  const foreground = parseColor(style.color);
  return {
    foreground,
    background: parseColor(style.backgroundColor),
    cursor: foreground,
    ansi: ansiTokens.map((token) => parseColor(style.getPropertyValue(token))),
    selectionBackground: style.getPropertyValue("--terminal-selection").trim(),
  };
}
