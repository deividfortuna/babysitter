import {
  GHOSTTY_CELL_WIDE,
  ghosttyColorsEqual,
  type GhosttyCell,
  type GhosttyColor,
  type GhosttySnapshot,
} from "./core";

export interface GhosttyCellMetrics {
  readonly width: number;
  readonly height: number;
  readonly baseline: number;
}

const CURSOR_STYLE = {
  bar: 0,
  underline: 2,
  hollow: 3,
} as const;

function cssColor(color: GhosttyColor): string {
  return `rgb(${color.r}, ${color.g}, ${color.b})`;
}

function sameTextStyle(left: GhosttyCell, right: GhosttyCell): boolean {
  return (
    ghosttyColorsEqual(left.foreground, right.foreground) &&
    left.bold === right.bold &&
    left.italic === right.italic &&
    left.invisible === right.invisible
  );
}

function sameBackground(left: GhosttyCell, right: GhosttyCell): boolean {
  return left.selected === right.selected && ghosttyColorsEqual(left.background, right.background);
}

function hasDecoration(cell: GhosttyCell): boolean {
  return cell.underline || cell.strikethrough || cell.overline;
}

export function ghosttyTextRunEnd(
  cells: readonly GhosttyCell[],
  start: number,
  sameStyle: (cell: GhosttyCell) => boolean,
): number {
  let end = start + 1;
  while (end < cells.length) {
    const next = cells[end];
    if (!next) break;
    if (next.wide === GHOSTTY_CELL_WIDE.spacerTail) {
      end += 1;
      continue;
    }
    if (next.text.length === 0 || !sameStyle(next)) break;
    end += 1;
  }
  return end;
}

function fontForCell(cell: GhosttyCell, fontSize: number, fontFamily: string): string {
  const style = cell.italic ? "italic" : "normal";
  const weight = cell.bold ? "700" : "400";
  return `${style} ${weight} ${fontSize}px ${fontFamily}`;
}

export function measureGhosttyCell(
  context: CanvasRenderingContext2D,
  fontSize: number,
  fontFamily: string,
  lineHeight: number,
): GhosttyCellMetrics {
  context.font = `normal 400 ${fontSize}px ${fontFamily}`;
  const widthMeasurement = context.measureText("M");
  const verticalMeasurement = context.measureText("Mg");
  const ascent = verticalMeasurement.actualBoundingBoxAscent || fontSize;
  const descent = verticalMeasurement.actualBoundingBoxDescent;
  const glyphHeight = ascent + descent;
  const height = Math.max(1, Math.round(fontSize * lineHeight), Math.ceil(glyphHeight));
  return {
    width: Math.max(1, widthMeasurement.width),
    height,
    baseline: Math.round((height - glyphHeight) / 2 + ascent),
  };
}

export interface RenderOptions {
  readonly context: CanvasRenderingContext2D;
  readonly snapshot: GhosttySnapshot;
  readonly metrics: GhosttyCellMetrics;
  readonly fontSize: number;
  readonly fontFamily: string;
  readonly selectionBackground: string;
}

function drawBackgrounds(options: RenderOptions, cells: readonly GhosttyCell[], top: number): void {
  const { context, snapshot, metrics, selectionBackground } = options;
  let start = 0;
  while (start < cells.length) {
    const first = cells[start];
    if (!first) break;
    let end = start + 1;
    while (end < cells.length && cells[end] && sameBackground(cells[end] as GhosttyCell, first)) end += 1;
    const left = start * metrics.width;
    const width = (end - start) * metrics.width;
    if (!ghosttyColorsEqual(first.background, snapshot.background)) {
      context.fillStyle = cssColor(first.background);
      context.fillRect(left, top, width, metrics.height);
    }
    if (first.selected) {
      context.fillStyle = selectionBackground;
      context.fillRect(left, top, width, metrics.height);
    }
    start = end;
  }
}

function drawText(options: RenderOptions, cells: readonly GhosttyCell[], top: number): void {
  const { context, metrics, fontSize, fontFamily } = options;
  let start = 0;
  while (start < cells.length) {
    const first = cells[start];
    if (!first) break;
    if (first.text.length === 0) {
      start += 1;
      continue;
    }
    const end = ghosttyTextRunEnd(cells, start, (cell) => sameTextStyle(cell, first));
    const text = cells
      .slice(start, end)
      .map((cell) => cell.text)
      .join("");
    const visible = !first.invisible && text.trim().length > 0;
    if (visible) {
      const left = start * metrics.width;
      const width = (end - start) * metrics.width;
      context.save();
      context.beginPath();
      context.rect(left, top, width, metrics.height);
      context.clip();
      context.font = fontForCell(first, fontSize, fontFamily);
      context.fillStyle = cssColor(first.foreground);
      context.fillText(text, left, top + metrics.baseline, width);
      context.restore();
    }
    start = end;
  }
}

function drawDecorations(options: RenderOptions, cells: readonly GhosttyCell[], top: number): void {
  const { context, metrics } = options;
  cells.forEach((cell, column) => {
    if (!hasDecoration(cell)) return;
    const left = column * metrics.width;
    context.fillStyle = cssColor(cell.foreground);
    if (cell.underline) context.fillRect(left, top + metrics.height - 2, metrics.width, 1);
    if (cell.strikethrough) context.fillRect(left, top + Math.floor(metrics.height * 0.55), metrics.width, 1);
    if (cell.overline) context.fillRect(left, top + 1, metrics.width, 1);
  });
}

function drawCursor(options: RenderOptions): void {
  const { context, snapshot, metrics, fontSize, fontFamily } = options;
  const onScreen = snapshot.cursorVisible && snapshot.cursorX >= 0 && snapshot.cursorY >= 0;
  if (!onScreen) return;
  const left = snapshot.cursorX * metrics.width;
  const top = snapshot.cursorY * metrics.height;
  context.fillStyle = cssColor(snapshot.cursor);
  context.strokeStyle = cssColor(snapshot.cursor);
  switch (snapshot.cursorStyle) {
    case CURSOR_STYLE.bar:
      context.fillRect(left, top, 2, metrics.height);
      return;
    case CURSOR_STYLE.underline:
      context.fillRect(left, top + metrics.height - 2, metrics.width, 2);
      return;
    case CURSOR_STYLE.hollow:
      context.strokeRect(left + 0.5, top + 0.5, metrics.width - 1, metrics.height - 1);
      return;
    default: {
      context.fillRect(left, top, metrics.width, metrics.height);
      const cell = snapshot.rowData[snapshot.cursorY]?.cells[snapshot.cursorX];
      if (!cell?.text) return;
      context.font = fontForCell(cell, fontSize, fontFamily);
      context.fillStyle = cssColor(snapshot.background);
      context.fillText(cell.text, left, top + metrics.baseline, metrics.width);
    }
  }
}

export function renderGhosttySnapshot(options: RenderOptions): void {
  const { context, snapshot, metrics } = options;
  context.save();
  context.resetTransform();
  context.fillStyle = cssColor(snapshot.background);
  context.fillRect(0, 0, context.canvas.width, context.canvas.height);
  context.restore();
  context.textBaseline = "alphabetic";
  snapshot.rowData.forEach((row, rowIndex) => {
    const top = rowIndex * metrics.height;
    drawBackgrounds(options, row.cells, top);
    drawText(options, row.cells, top);
    drawDecorations(options, row.cells, top);
  });
  drawCursor(options);
}
