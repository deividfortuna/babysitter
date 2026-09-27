import { type GhosttyRuntime } from "./runtime";

const GHOSTTY_SUCCESS = 0;
const GHOSTTY_OUT_OF_SPACE = -3;
const MAX_SCROLLBACK_ROWS = 5_000;
const SELECTION_FORMAT_OPTIONS_SIZE = 16;
const PALETTE_SIZE = 256;
const RGB_SIZE = 3;

const TERMINAL_OPTION = {
  foreground: 11,
  background: 12,
  cursor: 13,
  palette: 14,
  selection: 21,
} as const;

const TERMINAL_DATA = {
  scrollbar: 9,
  defaultPalette: 25,
} as const;

const RENDER_DATA = {
  cols: 1,
  rows: 2,
  dirty: 3,
  rowIterator: 4,
  background: 5,
  foreground: 6,
  cursor: 7,
  cursorHasValue: 8,
  cursorStyle: 10,
  cursorVisible: 11,
  cursorInViewport: 14,
  cursorX: 15,
  cursorY: 16,
} as const;

const ROW_DATA = {
  dirty: 1,
  cells: 3,
} as const;

const CELL_DATA = {
  raw: 1,
  style: 2,
  graphemesLength: 3,
  graphemes: 4,
  background: 5,
  foreground: 6,
  selected: 7,
} as const;

const RAW_CELL_WIDE = 3;

const POINT_TAG = {
  viewport: 1,
  screen: 2,
} as const;

const SCROLL_TAG = {
  bottom: 1,
  delta: 2,
} as const;

export const GHOSTTY_CELL_WIDE = {
  narrow: 0,
  wide: 1,
  spacerTail: 2,
  spacerHead: 3,
} as const;

export interface GhosttyColor {
  readonly r: number;
  readonly g: number;
  readonly b: number;
}

export interface GhosttyTheme {
  readonly foreground: GhosttyColor;
  readonly background: GhosttyColor;
  readonly cursor: GhosttyColor;
  readonly ansi: readonly GhosttyColor[];
  readonly selectionBackground: string;
}

export interface GhosttyCell {
  readonly text: string;
  readonly wide: number;
  readonly foreground: GhosttyColor;
  readonly background: GhosttyColor;
  readonly bold: boolean;
  readonly italic: boolean;
  readonly invisible: boolean;
  readonly strikethrough: boolean;
  readonly overline: boolean;
  readonly underline: boolean;
  readonly selected: boolean;
}

export interface GhosttyRow {
  readonly cells: readonly GhosttyCell[];
}

export interface GhosttySnapshot {
  readonly cols: number;
  readonly rows: number;
  readonly foreground: GhosttyColor;
  readonly background: GhosttyColor;
  readonly cursor: GhosttyColor;
  readonly cursorX: number;
  readonly cursorY: number;
  readonly cursorVisible: boolean;
  readonly cursorStyle: number;
  readonly rowData: readonly GhosttyRow[];
}

export interface GhosttyPoint {
  readonly x: number;
  readonly y: number;
}

export interface GhosttyRange {
  readonly start: GhosttyPoint;
  readonly end: GhosttyPoint;
}

export interface GhosttyScrollbar {
  readonly total: number;
  readonly offset: number;
  readonly len: number;
}

type PointTag = (typeof POINT_TAG)[keyof typeof POINT_TAG];

const decoder = new TextDecoder();
const encoder = new TextEncoder();

function faint(foreground: GhosttyColor, background: GhosttyColor): GhosttyColor {
  const channel = (front: number, back: number) => Math.floor((front * 155 + back * 100) / 255);
  return {
    r: channel(foreground.r, background.r),
    g: channel(foreground.g, background.g),
    b: channel(foreground.b, background.b),
  };
}

export function ghosttyColorsEqual(left: GhosttyColor, right: GhosttyColor): boolean {
  return left.r === right.r && left.g === right.g && left.b === right.b;
}

export function ghosttyCellText(codepointView: DataView, graphemeLength: number): string {
  if (graphemeLength === 1) return String.fromCodePoint(codepointView.getUint32(0, true));
  const chunkSize = 4_096;
  let text = "";
  for (let start = 0; start < graphemeLength; start += chunkSize) {
    const count = Math.min(chunkSize, graphemeLength - start);
    const codes = Array.from({ length: count }, (_, index) => codepointView.getUint32((start + index) * 4, true));
    text += String.fromCodePoint(...codes);
  }
  return text;
}

export class GhosttyTerminalCore {
  private readonly runtime: GhosttyRuntime;
  private terminalSlot = 0;
  private terminal = 0;
  private renderStateSlot = 0;
  private renderState = 0;
  private rowIteratorSlot = 0;
  private rowCellsSlot = 0;
  private scratch = 0;
  private graphemes = 0;
  private graphemeCapacity = 0;
  private style = 0;
  private scrollbar = 0;
  private rows: GhosttyRow[] = [];
  private disposed = false;

  constructor(runtime: GhosttyRuntime, cols: number, rows: number, theme: GhosttyTheme) {
    this.runtime = runtime;
    try {
      this.initialize(cols, rows, theme);
    } catch (error) {
      this.dispose();
      throw error;
    }
  }

  private initialize(cols: number, rows: number, theme: GhosttyTheme): void {
    const optionsSize = this.runtime.layout("GhosttyTerminalOptions").size;
    const options = this.runtime.alloc(optionsSize);
    this.runtime.setField(options, "GhosttyTerminalOptions", "cols", cols);
    this.runtime.setField(options, "GhosttyTerminalOptions", "rows", rows);
    this.runtime.setField(options, "GhosttyTerminalOptions", "max_scrollback", MAX_SCROLLBACK_ROWS);
    this.terminalSlot = this.runtime.allocOpaque();
    const terminalResult = this.runtime.call("ghostty_terminal_new", 0, this.terminalSlot, options);
    this.runtime.free(options, optionsSize);
    this.assertSuccess("ghostty_terminal_new", terminalResult);
    this.terminal = this.runtime.readPointer(this.terminalSlot);

    this.renderStateSlot = this.runtime.allocOpaque();
    this.assertSuccess(
      "ghostty_render_state_new",
      this.runtime.call("ghostty_render_state_new", 0, this.renderStateSlot),
    );
    this.renderState = this.runtime.readPointer(this.renderStateSlot);

    this.rowIteratorSlot = this.runtime.allocOpaque();
    this.assertSuccess(
      "ghostty_render_state_row_iterator_new",
      this.runtime.call("ghostty_render_state_row_iterator_new", 0, this.rowIteratorSlot),
    );
    this.rowCellsSlot = this.runtime.allocOpaque();
    this.assertSuccess(
      "ghostty_render_state_row_cells_new",
      this.runtime.call("ghostty_render_state_row_cells_new", 0, this.rowCellsSlot),
    );

    this.scratch = this.runtime.alloc(16);
    const styleSize = this.runtime.layout("GhosttyStyle").size;
    this.style = this.runtime.alloc(styleSize);
    this.scrollbar = this.runtime.alloc(this.runtime.layout("GhosttyTerminalScrollbar").size);
    this.setTheme(theme);
  }

  write(data: string): void {
    this.ensureActive();
    const bytes = encoder.encode(data);
    if (bytes.length === 0) return;
    const pointer = this.runtime.alloc(bytes.length);
    this.runtime.bytes(pointer, bytes.length).set(bytes);
    this.runtime.call("ghostty_terminal_vt_write", this.terminal, pointer, bytes.length);
    this.runtime.free(pointer, bytes.length);
  }

  resetAndWrite(data: string): void {
    this.ensureActive();
    this.runtime.call("ghostty_terminal_reset", this.terminal);
    this.rows = [];
    this.write(data);
  }

  resize(cols: number, rows: number, cellWidth: number, cellHeight: number): void {
    this.ensureActive();
    this.assertSuccess(
      "ghostty_terminal_resize",
      this.runtime.call(
        "ghostty_terminal_resize",
        this.terminal,
        cols,
        rows,
        Math.max(1, Math.round(cellWidth)),
        Math.max(1, Math.round(cellHeight)),
      ),
    );
  }

  setTheme(theme: GhosttyTheme): void {
    this.ensureActive();
    const color = this.runtime.alloc(RGB_SIZE);
    for (const [option, value] of [
      [TERMINAL_OPTION.foreground, theme.foreground],
      [TERMINAL_OPTION.background, theme.background],
      [TERMINAL_OPTION.cursor, theme.cursor],
    ] as const) {
      this.runtime.bytes(color, RGB_SIZE).set([value.r, value.g, value.b]);
      this.runtime.call("ghostty_terminal_set", this.terminal, option, color);
    }
    this.runtime.free(color, RGB_SIZE);
    this.setAnsiPalette(theme.ansi);
  }

  private setAnsiPalette(ansi: readonly GhosttyColor[]): void {
    const size = PALETTE_SIZE * RGB_SIZE;
    const palette = this.runtime.alloc(size);
    this.runtime.call("ghostty_terminal_set", this.terminal, TERMINAL_OPTION.palette, 0);
    this.runtime.call("ghostty_terminal_get", this.terminal, TERMINAL_DATA.defaultPalette, palette);
    const bytes = this.runtime.bytes(palette, size);
    ansi.slice(0, 16).forEach((value, index) => bytes.set([value.r, value.g, value.b], index * RGB_SIZE));
    this.runtime.call("ghostty_terminal_set", this.terminal, TERMINAL_OPTION.palette, palette);
    this.runtime.free(palette, size);
  }

  scroll(deltaRows: number): void {
    this.scrollViewport(SCROLL_TAG.delta, deltaRows);
  }

  scrollToBottom(): void {
    this.scrollViewport(SCROLL_TAG.bottom, 0);
  }

  private scrollViewport(tag: (typeof SCROLL_TAG)[keyof typeof SCROLL_TAG], deltaRows: number): void {
    this.ensureActive();
    const layout = this.runtime.layout("GhosttyTerminalScrollViewport");
    const scroll = this.runtime.alloc(layout.size);
    this.runtime.setField(scroll, "GhosttyTerminalScrollViewport", "tag", tag);
    const value = layout.fields.value;
    if (value) this.runtime.view(scroll + value.offset, value.size).setInt32(0, deltaRows, true);
    this.runtime.call("ghostty_terminal_scroll_viewport", this.terminal, scroll);
    this.runtime.free(scroll, layout.size);
  }

  scrollbarState(): GhosttyScrollbar | null {
    this.ensureActive();
    const layout = this.runtime.layout("GhosttyTerminalScrollbar");
    this.runtime.bytes(this.scrollbar, layout.size).fill(0);
    const result = this.runtime.call("ghostty_terminal_get", this.terminal, TERMINAL_DATA.scrollbar, this.scrollbar);
    if (result !== GHOSTTY_SUCCESS) return null;
    return {
      total: this.runtime.readField(this.scrollbar, "GhosttyTerminalScrollbar", "total"),
      offset: this.runtime.readField(this.scrollbar, "GhosttyTerminalScrollbar", "offset"),
      len: this.runtime.readField(this.scrollbar, "GhosttyTerminalScrollbar", "len"),
    };
  }

  setSelection(anchor: GhosttyPoint, end: GhosttyPoint): void {
    this.ensureActive();
    const selectionLayout = this.runtime.layout("GhosttySelection");
    const gridRefSize = this.runtime.layout("GhosttyGridRef").size;
    const selection = this.runtime.alloc(selectionLayout.size);
    let startRef = 0;
    let endRef = 0;
    try {
      this.runtime.setField(selection, "GhosttySelection", "size", selectionLayout.size);
      startRef = this.gridRef(anchor, POINT_TAG.screen);
      endRef = this.gridRef(end, POINT_TAG.screen);
      this.copyField(selection, selectionLayout.fields.start, startRef);
      this.copyField(selection, selectionLayout.fields.end, endRef);
      this.runtime.call("ghostty_terminal_set", this.terminal, TERMINAL_OPTION.selection, selection);
    } finally {
      this.runtime.free(startRef, gridRefSize);
      this.runtime.free(endRef, gridRefSize);
      this.runtime.free(selection, selectionLayout.size);
    }
  }

  selectWord(viewport: GhosttyPoint): GhosttyRange | null {
    return this.selectAt("GhosttyTerminalSelectWordOptions", "ghostty_terminal_select_word", viewport);
  }

  selectLine(viewport: GhosttyPoint): GhosttyRange | null {
    return this.selectAt("GhosttyTerminalSelectLineOptions", "ghostty_terminal_select_line", viewport);
  }

  clearSelection(): void {
    this.ensureActive();
    this.runtime.call("ghostty_terminal_set", this.terminal, TERMINAL_OPTION.selection, 0);
  }

  selectionText(): string {
    this.ensureActive();
    const options = this.runtime.alloc(SELECTION_FORMAT_OPTIONS_SIZE);
    const optionsView = this.runtime.view(options, SELECTION_FORMAT_OPTIONS_SIZE);
    optionsView.setUint32(0, SELECTION_FORMAT_OPTIONS_SIZE, true);
    optionsView.setUint32(4, 0, true);
    optionsView.setUint8(8, 1);
    optionsView.setUint8(9, 1);
    optionsView.setUint32(12, 0, true);
    const text = this.readString((output, outputSize, written) =>
      this.runtime.call("ghostty_terminal_selection_format_buf", this.terminal, options, output, outputSize, written),
    );
    this.runtime.free(options, SELECTION_FORMAT_OPTIONS_SIZE);
    return text;
  }

  viewportToScreen(viewport: GhosttyPoint): GhosttyPoint | null {
    this.ensureActive();
    const ref = this.gridRef(viewport, POINT_TAG.viewport);
    const point = this.pointFromGridRef(ref, POINT_TAG.screen);
    this.runtime.free(ref, this.runtime.layout("GhosttyGridRef").size);
    return point;
  }

  snapshot(): GhosttySnapshot {
    this.ensureActive();
    this.assertSuccess(
      "ghostty_render_state_update",
      this.runtime.call("ghostty_render_state_update", this.renderState, this.terminal),
    );
    const cols = this.getU16(RENDER_DATA.cols);
    const rowCount = this.getU16(RENDER_DATA.rows);
    const dirty = this.getU32(RENDER_DATA.dirty);
    const foreground = this.getColor(RENDER_DATA.foreground, { r: 229, g: 231, b: 235 });
    const background = this.getColor(RENDER_DATA.background, { r: 0, g: 0, b: 0 });
    const cursor = this.getBool(RENDER_DATA.cursorHasValue)
      ? this.getColor(RENDER_DATA.cursor, foreground)
      : foreground;
    const cursorInViewport = this.getBool(RENDER_DATA.cursorInViewport);

    const shapeChanged = this.rows.length !== rowCount || this.rows.some((row) => row.cells.length !== cols);
    if (shapeChanged) {
      this.rows = Array.from({ length: rowCount }, () => ({
        cells: Array.from({ length: cols }, () => this.emptyCell(foreground, background)),
      }));
    }

    if (dirty !== 0) {
      this.assertSuccess(
        "ghostty_render_state_get(row iterator)",
        this.runtime.call("ghostty_render_state_get", this.renderState, RENDER_DATA.rowIterator, this.rowIteratorSlot),
      );
      const iterator = this.runtime.readPointer(this.rowIteratorSlot);
      for (
        let rowIndex = 0;
        rowIndex < rowCount && this.runtime.call("ghostty_render_state_row_iterator_next", iterator) !== 0;
        rowIndex += 1
      ) {
        const fullRedraw = dirty === 2;
        if (!fullRedraw && !this.getRowBool(iterator, ROW_DATA.dirty)) continue;
        this.rows[rowIndex] = this.readRow(iterator, cols, foreground, background);
        this.runtime.bytes(this.scratch, 1)[0] = 0;
        this.runtime.call("ghostty_render_state_row_set", iterator, 0, this.scratch);
      }
      this.runtime.view(this.scratch, 4).setUint32(0, 0, true);
      this.runtime.call("ghostty_render_state_set", this.renderState, 0, this.scratch);
    }

    return {
      cols,
      rows: rowCount,
      foreground,
      background,
      cursor,
      cursorX: cursorInViewport ? this.getU16(RENDER_DATA.cursorX) : -1,
      cursorY: cursorInViewport ? this.getU16(RENDER_DATA.cursorY) : -1,
      cursorVisible: cursorInViewport && this.getBool(RENDER_DATA.cursorVisible),
      cursorStyle: this.getU32(RENDER_DATA.cursorStyle),
      rowData: this.rows,
    };
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    if (this.rowCellsSlot) {
      const cells = this.runtime.readPointer(this.rowCellsSlot);
      if (cells) this.runtime.call("ghostty_render_state_row_cells_free", cells);
    }
    if (this.rowIteratorSlot) {
      const iterator = this.runtime.readPointer(this.rowIteratorSlot);
      if (iterator) this.runtime.call("ghostty_render_state_row_iterator_free", iterator);
    }
    if (this.renderState) this.runtime.call("ghostty_render_state_free", this.renderState);
    if (this.terminal) this.runtime.call("ghostty_terminal_free", this.terminal);
    if (this.style) this.runtime.free(this.style, this.runtime.layout("GhosttyStyle").size);
    if (this.scrollbar) this.runtime.free(this.scrollbar, this.runtime.layout("GhosttyTerminalScrollbar").size);
    this.runtime.free(this.scratch, 16);
    this.runtime.free(this.graphemes, this.graphemeCapacity);
    for (const slot of [this.rowCellsSlot, this.rowIteratorSlot, this.renderStateSlot, this.terminalSlot]) {
      this.runtime.freeOpaque(slot);
    }
  }

  private readString(format: (output: number, outputSize: number, written: number) => number): string {
    const written = this.runtime.call("ghostty_wasm_alloc_usize");
    const sizeResult = format(0, 0, written);
    const outputSize = this.runtime.view(written, 4).getUint32(0, true);
    let text = "";
    if (sizeResult === GHOSTTY_OUT_OF_SPACE && outputSize > 0) {
      const output = this.runtime.alloc(outputSize);
      const result = format(output, outputSize, written);
      const outputLength = this.runtime.view(written, 4).getUint32(0, true);
      if (result === GHOSTTY_SUCCESS) text = decoder.decode(this.runtime.bytes(output, outputLength));
      this.runtime.free(output, outputSize);
    }
    this.runtime.call("ghostty_wasm_free_usize", written);
    return text;
  }

  private readRow(
    iterator: number,
    cols: number,
    defaultForeground: GhosttyColor,
    defaultBackground: GhosttyColor,
  ): GhosttyRow {
    this.assertSuccess(
      "ghostty_render_state_row_get(cells)",
      this.runtime.call("ghostty_render_state_row_get", iterator, ROW_DATA.cells, this.rowCellsSlot),
    );
    const cellsIterator = this.runtime.readPointer(this.rowCellsSlot);
    const { size: styleSize, fields: styleFields } = this.runtime.layout("GhosttyStyle");
    const cells: GhosttyCell[] = [];
    while (cells.length < cols && this.runtime.call("ghostty_render_state_row_cells_next", cellsIterator) !== 0) {
      let foreground = this.getCellColor(cellsIterator, CELL_DATA.foreground, defaultForeground);
      let background = this.getCellColor(cellsIterator, CELL_DATA.background, defaultBackground);
      this.runtime.bytes(this.style, styleSize).fill(0);
      this.runtime.setField(this.style, "GhosttyStyle", "size", styleSize);
      this.runtime.call("ghostty_render_state_row_cells_get", cellsIterator, CELL_DATA.style, this.style);
      const text = this.readCellText(cellsIterator);
      const followsText = text.length === 0 && Boolean(cells.at(-1)?.text.length);
      const wide = followsText ? this.readCellWide(cellsIterator) : GHOSTTY_CELL_WIDE.narrow;
      const selected = this.getCellBool(cellsIterator, CELL_DATA.selected);
      const style = this.runtime.view(this.style, styleSize);
      const flag = (name: string) => {
        const field = styleFields[name];
        return field !== undefined && style.getUint8(field.offset) !== 0;
      };
      if (flag("inverse")) [foreground, background] = [background, foreground];
      if (flag("faint")) foreground = faint(foreground, background);
      const underline = styleFields.underline;
      cells.push({
        text,
        wide,
        foreground,
        background,
        bold: flag("bold"),
        italic: flag("italic"),
        invisible: flag("invisible"),
        strikethrough: flag("strikethrough"),
        overline: flag("overline"),
        underline: underline !== undefined && style.getInt32(underline.offset, true) !== 0,
        selected,
      });
    }
    while (cells.length < cols) cells.push(this.emptyCell(defaultForeground, defaultBackground));
    return { cells };
  }

  private readCellText(cellsIterator: number): string {
    const graphemeLength = this.getCellU32(cellsIterator, CELL_DATA.graphemesLength);
    if (graphemeLength === 0) return "";
    const bufferSize = graphemeLength * 4;
    if (bufferSize > this.graphemeCapacity) {
      const capacity = Math.max(bufferSize, this.graphemeCapacity * 2);
      const buffer = this.runtime.alloc(capacity);
      this.runtime.free(this.graphemes, this.graphemeCapacity);
      this.graphemes = buffer;
      this.graphemeCapacity = capacity;
    }
    const result = this.runtime.call(
      "ghostty_render_state_row_cells_get",
      cellsIterator,
      CELL_DATA.graphemes,
      this.graphemes,
    );
    if (result !== GHOSTTY_SUCCESS) return "";
    return ghosttyCellText(this.runtime.view(this.graphemes, bufferSize), graphemeLength);
  }

  private readCellWide(cellsIterator: number): number {
    this.assertSuccess(
      "ghostty_render_state_row_cells_get(raw)",
      this.runtime.call("ghostty_render_state_row_cells_get", cellsIterator, CELL_DATA.raw, this.scratch),
    );
    const rawCell = this.runtime.view(this.scratch, 8).getBigUint64(0, true);
    this.runtime.view(this.scratch + 8, 4).setUint32(0, 0, true);
    this.assertSuccess(
      "ghostty_cell_get(wide)",
      this.runtime.call("ghostty_cell_get", rawCell, RAW_CELL_WIDE, this.scratch + 8),
    );
    return this.runtime.view(this.scratch + 8, 4).getUint32(0, true);
  }

  private copyField(target: number, field: { offset: number; size: number } | undefined, source: number): void {
    if (!field) return;
    this.runtime.bytes(target + field.offset, field.size).set(this.runtime.bytes(source, field.size));
  }

  private gridRef(point: GhosttyPoint, tag: PointTag): number {
    const pointLayout = this.runtime.layout("GhosttyPoint");
    const pointer = this.runtime.alloc(pointLayout.size);
    this.runtime.setField(pointer, "GhosttyPoint", "tag", tag);
    const value = pointLayout.fields.value;
    if (value) {
      const view = this.runtime.view(pointer + value.offset, value.size);
      view.setUint16(0, Math.max(0, point.x), true);
      view.setUint32(4, Math.max(0, point.y), true);
    }
    const gridRefSize = this.runtime.layout("GhosttyGridRef").size;
    const gridRef = this.runtime.alloc(gridRefSize);
    this.runtime.setField(gridRef, "GhosttyGridRef", "size", gridRefSize);
    const result = this.runtime.call("ghostty_terminal_grid_ref", this.terminal, pointer, gridRef);
    this.runtime.free(pointer, pointLayout.size);
    if (result !== GHOSTTY_SUCCESS) {
      this.runtime.free(gridRef, gridRefSize);
      this.assertSuccess("ghostty_terminal_grid_ref", result);
    }
    return gridRef;
  }

  private selectAt(
    optionsName: "GhosttyTerminalSelectWordOptions" | "GhosttyTerminalSelectLineOptions",
    operation: "ghostty_terminal_select_word" | "ghostty_terminal_select_line",
    viewport: GhosttyPoint,
  ): GhosttyRange | null {
    this.ensureActive();
    const optionsLayout = this.runtime.layout(optionsName);
    const selectionLayout = this.runtime.layout("GhosttySelection");
    const options = this.runtime.alloc(optionsLayout.size);
    let ref = 0;
    let selection = 0;
    let range: GhosttyRange | null = null;
    try {
      this.runtime.setField(options, optionsName, "size", optionsLayout.size);
      ref = this.gridRef(viewport, POINT_TAG.viewport);
      this.copyField(options, optionsLayout.fields.ref, ref);
      selection = this.runtime.alloc(selectionLayout.size);
      this.runtime.setField(selection, "GhosttySelection", "size", selectionLayout.size);
      if (this.runtime.call(operation, this.terminal, options, selection) === GHOSTTY_SUCCESS) {
        const start = this.pointFromGridRef(selection + (selectionLayout.fields.start?.offset ?? 0), POINT_TAG.screen);
        const end = this.pointFromGridRef(selection + (selectionLayout.fields.end?.offset ?? 0), POINT_TAG.screen);
        if (start && end) range = { start, end };
        this.runtime.call("ghostty_terminal_set", this.terminal, TERMINAL_OPTION.selection, selection);
      }
    } finally {
      this.runtime.free(selection, selectionLayout.size);
      this.runtime.free(ref, this.runtime.layout("GhosttyGridRef").size);
      this.runtime.free(options, optionsLayout.size);
    }
    return range;
  }

  private pointFromGridRef(ref: number, tag: PointTag): GhosttyPoint | null {
    const coordinateLayout = this.runtime.layout("GhosttyPointCoordinate");
    const coordinate = this.runtime.alloc(coordinateLayout.size);
    const result = this.runtime.call("ghostty_terminal_point_from_grid_ref", this.terminal, ref, tag, coordinate);
    const point =
      result === GHOSTTY_SUCCESS
        ? {
            x: this.runtime.readField(coordinate, "GhosttyPointCoordinate", "x"),
            y: this.runtime.readField(coordinate, "GhosttyPointCoordinate", "y"),
          }
        : null;
    this.runtime.free(coordinate, coordinateLayout.size);
    return point;
  }

  private getU16(data: number): number {
    this.readRenderState(data, 2);
    return this.runtime.view(this.scratch, 2).getUint16(0, true);
  }

  private getU32(data: number): number {
    this.readRenderState(data, 4);
    return this.runtime.view(this.scratch, 4).getUint32(0, true);
  }

  private getBool(data: number): boolean {
    this.readRenderState(data, 1);
    return this.runtime.bytes(this.scratch, 1)[0] !== 0;
  }

  private readRenderState(data: number, size: number): void {
    this.runtime.bytes(this.scratch, size).fill(0);
    this.assertSuccess(
      "ghostty_render_state_get",
      this.runtime.call("ghostty_render_state_get", this.renderState, data, this.scratch),
    );
  }

  private getColor(data: number, fallback: GhosttyColor): GhosttyColor {
    this.runtime.bytes(this.scratch, RGB_SIZE).fill(0);
    const result = this.runtime.call("ghostty_render_state_get", this.renderState, data, this.scratch);
    return result === GHOSTTY_SUCCESS ? this.readColor(this.scratch) : fallback;
  }

  private getRowBool(iterator: number, data: number): boolean {
    this.runtime.bytes(this.scratch, 1)[0] = 0;
    const result = this.runtime.call("ghostty_render_state_row_get", iterator, data, this.scratch);
    return result === GHOSTTY_SUCCESS && this.runtime.bytes(this.scratch, 1)[0] !== 0;
  }

  private getCellU32(iterator: number, data: number): number {
    this.runtime.bytes(this.scratch, 4).fill(0);
    const result = this.runtime.call("ghostty_render_state_row_cells_get", iterator, data, this.scratch);
    return result === GHOSTTY_SUCCESS ? this.runtime.view(this.scratch, 4).getUint32(0, true) : 0;
  }

  private getCellBool(iterator: number, data: number): boolean {
    this.runtime.bytes(this.scratch, 1)[0] = 0;
    const result = this.runtime.call("ghostty_render_state_row_cells_get", iterator, data, this.scratch);
    return result === GHOSTTY_SUCCESS && this.runtime.bytes(this.scratch, 1)[0] !== 0;
  }

  private getCellColor(iterator: number, data: number, fallback: GhosttyColor): GhosttyColor {
    this.runtime.bytes(this.scratch, RGB_SIZE).fill(0);
    const result = this.runtime.call("ghostty_render_state_row_cells_get", iterator, data, this.scratch);
    return result === GHOSTTY_SUCCESS ? this.readColor(this.scratch) : fallback;
  }

  private readColor(pointer: number): GhosttyColor {
    const bytes = this.runtime.bytes(pointer, RGB_SIZE);
    return { r: bytes[0] ?? 0, g: bytes[1] ?? 0, b: bytes[2] ?? 0 };
  }

  private emptyCell(foreground: GhosttyColor, background: GhosttyColor): GhosttyCell {
    return {
      text: "",
      wide: GHOSTTY_CELL_WIDE.narrow,
      foreground,
      background,
      bold: false,
      italic: false,
      invisible: false,
      strikethrough: false,
      overline: false,
      underline: false,
      selected: false,
    };
  }

  private assertSuccess(operation: string, result: number): void {
    if (result !== GHOSTTY_SUCCESS) throw new Error(`${operation} failed with result ${result}`);
  }

  private ensureActive(): void {
    if (this.disposed) throw new Error("libghostty-vt terminal has been disposed");
  }
}
