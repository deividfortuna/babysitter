import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test } from "vite-plus/test";

const LIGHT_SURFACES = ["--background", "--popover", "--muted", "--sidebar", "--accent"];

function lightTheme(): string {
  const css = readFileSync(resolve(process.cwd(), "src/renderer/styles.css"), "utf8");
  const start = css.indexOf(":root {");
  return css.slice(start, css.indexOf("}", start));
}

function variable(theme: string, name: string): string {
  const value = theme.match(new RegExp(`${name}:\\s*([^;]+);`))?.[1];
  if (value === undefined) throw new Error(`the light theme carries no ${name}`);
  return value.trim();
}

function channels(color: string): number[] {
  const hex = color.match(/^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i);
  if (hex) return hex.slice(1).map((channel) => parseInt(channel, 16));
  const rgb = color.match(/^rgb\((\d+) (\d+) (\d+)\)$/);
  if (rgb) return rgb.slice(1).map(Number);
  throw new Error(`cannot read the colour ${color}`);
}

function luminance(color: string): number {
  const [r, g, b] = channels(color).map((channel) => {
    const c = channel / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (light + 0.05) / (dark + 0.05);
}

test.each(["--scrollbar-thumb", "--scrollbar-thumb-hover"])(
  "the light %s has a contrast of 3.1:1 or more on every light surface",
  (thumb) => {
    const theme = lightTheme();
    const color = variable(theme, thumb);

    for (const surface of LIGHT_SURFACES) {
      expect(contrast(color, variable(theme, surface)), `${thumb} on ${surface}`).toBeGreaterThanOrEqual(3.1);
    }
  },
);
