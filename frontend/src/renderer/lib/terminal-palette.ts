const ansiSlots = {
  black: "--ansi-black",
  red: "--ansi-red",
  green: "--ansi-green",
  yellow: "--ansi-yellow",
  blue: "--ansi-blue",
  magenta: "--ansi-magenta",
  cyan: "--ansi-cyan",
  white: "--ansi-white",
  brightBlack: "--ansi-bright-black",
  brightRed: "--ansi-bright-red",
  brightGreen: "--ansi-bright-green",
  brightYellow: "--ansi-bright-yellow",
  brightBlue: "--ansi-bright-blue",
  brightMagenta: "--ansi-bright-magenta",
  brightCyan: "--ansi-bright-cyan",
  brightWhite: "--ansi-bright-white",
  selectionBackground: "--terminal-selection",
};

export function terminalTheme(style: CSSStyleDeclaration) {
  const theme: Record<string, string> = {
    background: style.backgroundColor,
    foreground: style.color,
    cursor: style.color,
  };
  for (const [slot, token] of Object.entries(ansiSlots)) {
    theme[slot] = style.getPropertyValue(token).trim();
  }
  return theme;
}
