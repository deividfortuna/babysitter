import { GhosttyTerminalCore, type GhosttyPoint, type GhosttyRange, type GhosttyTheme } from "./core";
import { measureGhosttyCell, renderGhosttySnapshot, type GhosttyCellMetrics } from "./renderer";

const FONT_SIZE = 11;
const LINE_HEIGHT = 1.2;
const FONT_FAMILY = '"SF Mono", SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace';
const MIN_THUMB_HEIGHT = 18;
const THUMB_INSET = 4;

type SelectionMode = "cell" | "word" | "line";

export interface GhosttySurfaceElements {
  readonly scroller: HTMLElement;
  readonly frame: HTMLElement;
}

export interface GhosttySurfaceOptions {
  readonly cols: number;
  readonly rows: number;
  readonly theme: GhosttyTheme;
}

export function scrollbarThumb(
  state: { total: number; offset: number; len: number },
  trackHeight: number,
): { height: number; top: number } | null {
  const maxOffset = state.total - state.len;
  if (trackHeight <= 0 || state.len <= 0 || maxOffset <= 0) return null;
  const height = Math.min(trackHeight, Math.max(MIN_THUMB_HEIGHT, (trackHeight * state.len) / state.total));
  const offset = Math.max(0, Math.min(state.offset, maxOffset));
  return { height, top: (trackHeight - height) * (offset / maxOffset) };
}

export function wheelRows(event: Pick<WheelEvent, "deltaY" | "deltaMode">, cellHeight: number, pageRows: number) {
  const linesPerUnit = [1 / cellHeight, 1, pageRows];
  return event.deltaY * (linesPerUnit[event.deltaMode] ?? 1 / cellHeight);
}

function selectionMode(clickCount: number): SelectionMode {
  if (clickCount >= 3) return "line";
  if (clickCount === 2) return "word";
  return "cell";
}

function isBefore(point: GhosttyPoint, other: GhosttyPoint): boolean {
  return point.y < other.y || (point.y === other.y && point.x < other.x);
}

async function createCore(options: GhosttySurfaceOptions): Promise<GhosttyTerminalCore> {
  const { loadGhosttyRuntime } = await import("./runtime");
  return new GhosttyTerminalCore(await loadGhosttyRuntime(), options.cols, options.rows, options.theme);
}

export class GhosttySurface {
  private readonly scroller: HTMLElement;
  private readonly frame: HTMLElement;
  private readonly canvas: HTMLCanvasElement;
  private readonly thumb: HTMLDivElement;
  private readonly context: CanvasRenderingContext2D;
  private readonly core: GhosttyTerminalCore;
  private readonly cols: number;
  private readonly rows: number;
  private theme: GhosttyTheme;
  private metrics: GhosttyCellMetrics;
  private renderRequest = 0;
  private wheelRemainder = 0;
  private disposed = false;
  private dprMedia: MediaQueryList | null = null;
  private selecting = false;
  private selectionMoved = false;
  private selectionMode: SelectionMode = "cell";
  private selectionAnchor: GhosttyPoint | null = null;
  private selectionBase: GhosttyRange | null = null;
  private hasSelection = false;

  private constructor(
    elements: GhosttySurfaceElements,
    canvas: HTMLCanvasElement,
    thumb: HTMLDivElement,
    context: CanvasRenderingContext2D,
    core: GhosttyTerminalCore,
    options: GhosttySurfaceOptions,
  ) {
    this.scroller = elements.scroller;
    this.frame = elements.frame;
    this.canvas = canvas;
    this.thumb = thumb;
    this.context = context;
    this.core = core;
    this.cols = options.cols;
    this.rows = options.rows;
    this.theme = options.theme;
    this.metrics = measureGhosttyCell(context, FONT_SIZE, FONT_FAMILY, LINE_HEIGHT);
    this.installEvents();
    this.watchDevicePixelRatio();
    this.layout();
  }

  static async create(
    elements: GhosttySurfaceElements,
    options: GhosttySurfaceOptions,
  ): Promise<GhosttySurface | null> {
    const canvas = document.createElement("canvas");
    const context = canvas.getContext("2d", { alpha: false });
    if (!context) return null;
    canvas.setAttribute("aria-hidden", "true");
    canvas.style.display = "block";
    canvas.style.cursor = "text";
    const thumb = document.createElement("div");
    thumb.setAttribute("aria-hidden", "true");
    thumb.style.cssText = `position:absolute;right:${THUMB_INSET}px;top:${THUMB_INSET}px;width:4px;border-radius:2px;background:currentColor;opacity:0.3;pointer-events:none;display:none;`;
    await document.fonts.load(`${FONT_SIZE}px ${FONT_FAMILY}`).catch(() => []);
    const core = await createCore(options);
    elements.scroller.replaceChildren(canvas);
    elements.frame.append(thumb);
    return new GhosttySurface(elements, canvas, thumb, context, core, options);
  }

  write(data: string): void {
    if (this.disposed) return;
    this.core.write(data);
    this.requestRender();
  }

  resetAndWrite(data: string): void {
    if (this.disposed) return;
    this.core.resetAndWrite(data);
    this.core.scrollToBottom();
    this.forgetSelection();
    this.requestRender();
  }

  setTheme(theme: GhosttyTheme): void {
    if (this.disposed) return;
    this.theme = theme;
    this.core.setTheme(theme);
    this.requestRender();
  }

  selectionText(): string {
    return this.hasSelection ? this.core.selectionText() : "";
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    if (this.renderRequest !== 0) window.cancelAnimationFrame(this.renderRequest);
    this.removeEvents();
    this.dprMedia?.removeEventListener("change", this.onDevicePixelRatioChange);
    this.core.dispose();
    this.canvas.remove();
    this.thumb.remove();
  }

  private layout(): void {
    const ratio = window.devicePixelRatio || 1;
    const width = Math.ceil(this.cols * this.metrics.width);
    const height = this.rows * this.metrics.height;
    this.canvas.style.width = `${width}px`;
    this.canvas.style.height = `${height}px`;
    this.canvas.width = Math.round(width * ratio);
    this.canvas.height = Math.round(height * ratio);
    this.context.setTransform(ratio, 0, 0, ratio, 0, 0);
    this.core.resize(this.cols, this.rows, this.metrics.width, this.metrics.height);
    this.render();
  }

  private requestRender(): void {
    if (this.disposed || this.renderRequest !== 0) return;
    this.renderRequest = window.requestAnimationFrame(() => {
      this.renderRequest = 0;
      this.render();
    });
  }

  private render(): void {
    if (this.disposed) return;
    renderGhosttySnapshot({
      context: this.context,
      snapshot: this.core.snapshot(),
      metrics: this.metrics,
      fontSize: FONT_SIZE,
      fontFamily: FONT_FAMILY,
      selectionBackground: this.theme.selectionBackground,
    });
    this.updateScrollbar();
  }

  private updateScrollbar(): void {
    const state = this.core.scrollbarState();
    const thumb = state && scrollbarThumb(state, this.frame.clientHeight - THUMB_INSET * 2);
    this.thumb.style.display = thumb ? "block" : "none";
    if (!thumb) return;
    this.thumb.style.height = `${thumb.height}px`;
    this.thumb.style.transform = `translateY(${thumb.top}px)`;
  }

  private cellAt(clientX: number, clientY: number): GhosttyPoint {
    const bounds = this.canvas.getBoundingClientRect();
    const column = Math.floor((clientX - bounds.left) / this.metrics.width);
    const row = Math.floor((clientY - bounds.top) / this.metrics.height);
    return {
      x: Math.max(0, Math.min(this.cols - 1, column)),
      y: Math.max(0, Math.min(this.rows - 1, row)),
    };
  }

  private selectRange(mode: SelectionMode, cell: GhosttyPoint): GhosttyRange | null {
    if (mode === "word") return this.core.selectWord(cell);
    if (mode === "line") return this.core.selectLine(cell);
    return null;
  }

  private beginSelection(event: MouseEvent): void {
    const cell = this.cellAt(event.clientX, event.clientY);
    this.selecting = true;
    this.selectionMoved = false;
    this.selectionMode = selectionMode(event.detail);
    this.selectionBase = this.selectRange(this.selectionMode, cell);
    this.selectionAnchor = this.selectionBase?.start ?? this.core.viewportToScreen(cell);
    this.hasSelection = this.selectionBase !== null;
    if (this.selectionBase === null && this.selectionAnchor) {
      this.selectionMode = "cell";
      this.core.setSelection(this.selectionAnchor, this.selectionAnchor);
    }
    this.requestRender();
  }

  private extendSelection(event: MouseEvent): void {
    const anchor = this.selectionAnchor;
    if (!anchor) return;
    const cell = this.cellAt(event.clientX, event.clientY);
    const screen = this.core.viewportToScreen(cell);
    if (!screen) return;
    const base = this.selectionBase;
    const range = this.selectRange(this.selectionMode, cell);
    const backwards = base !== null && isBefore(screen, base.start);
    const start = base === null ? anchor : backwards ? base.end : base.start;
    const end = range === null ? screen : backwards ? range.start : range.end;
    this.core.setSelection(start, end);
    this.selectionMoved = true;
    this.hasSelection = true;
    this.requestRender();
  }

  private forgetSelection(): void {
    this.selecting = false;
    this.selectionAnchor = null;
    this.selectionBase = null;
    if (!this.hasSelection) return;
    this.hasSelection = false;
    this.core.clearSelection();
    this.requestRender();
  }

  private readonly onMouseDown = (event: MouseEvent) => {
    if (event.button !== 0) return;
    event.preventDefault();
    window.getSelection()?.removeAllRanges();
    this.beginSelection(event);
    window.addEventListener("mousemove", this.onMouseMove);
    window.addEventListener("mouseup", this.onMouseUp);
  };

  private readonly onMouseMove = (event: MouseEvent) => {
    if (this.selecting) this.extendSelection(event);
  };

  private readonly onMouseUp = () => {
    window.removeEventListener("mousemove", this.onMouseMove);
    window.removeEventListener("mouseup", this.onMouseUp);
    this.selecting = false;
    const clickedWithoutDrag = this.selectionMode === "cell" && !this.selectionMoved;
    if (clickedWithoutDrag) this.forgetSelection();
  };

  private readonly onDocumentMouseDown = (event: MouseEvent) => {
    if (event.target !== this.canvas) this.forgetSelection();
  };

  private readonly onCopy = (event: ClipboardEvent) => {
    const domSelection = window.getSelection()?.toString() ?? "";
    const text = this.selectionText();
    if (domSelection.length > 0 || text.length === 0 || !event.clipboardData) return;
    event.clipboardData.setData("text/plain", text);
    event.preventDefault();
  };

  private readonly onWheel = (event: WheelEvent) => {
    const state = this.core.scrollbarState();
    if (!state) return;
    const total = this.wheelRemainder + wheelRows(event, this.metrics.height, this.rows);
    const rows = Math.trunc(total);
    const maxOffset = Math.max(0, state.total - state.len);
    const target = Math.max(0, Math.min(maxOffset, state.offset + rows));
    if (target === state.offset) {
      this.wheelRemainder = 0;
      return;
    }
    event.preventDefault();
    this.wheelRemainder = total - rows;
    this.core.scroll(target - state.offset);
    if (this.selecting) this.extendSelection(event);
    this.requestRender();
  };

  private readonly onDevicePixelRatioChange = () => {
    this.watchDevicePixelRatio();
    this.layout();
  };

  private watchDevicePixelRatio(): void {
    this.dprMedia?.removeEventListener("change", this.onDevicePixelRatioChange);
    this.dprMedia = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`);
    this.dprMedia.addEventListener("change", this.onDevicePixelRatioChange);
  }

  private installEvents(): void {
    this.canvas.addEventListener("mousedown", this.onMouseDown);
    this.scroller.addEventListener("wheel", this.onWheel, { passive: false });
    document.addEventListener("mousedown", this.onDocumentMouseDown);
    document.addEventListener("copy", this.onCopy);
  }

  private removeEvents(): void {
    this.canvas.removeEventListener("mousedown", this.onMouseDown);
    this.scroller.removeEventListener("wheel", this.onWheel);
    document.removeEventListener("mousedown", this.onDocumentMouseDown);
    document.removeEventListener("copy", this.onCopy);
    window.removeEventListener("mousemove", this.onMouseMove);
    window.removeEventListener("mouseup", this.onMouseUp);
  }
}
