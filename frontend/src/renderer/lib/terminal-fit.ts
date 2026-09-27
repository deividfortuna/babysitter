import type { TerminalGrid } from "./ghostty/surface";

function scrolls(element: Element): boolean {
  const overflow = getComputedStyle(element).overflowY;
  return overflow === "auto" || overflow === "scroll";
}

export function scrollingAncestor(element: HTMLElement): HTMLElement {
  for (let parent = element.parentElement; parent; parent = parent.parentElement) {
    if (scrolls(parent)) return parent;
  }
  return document.documentElement;
}

function horizontalPadding(element: HTMLElement): number {
  const style = getComputedStyle(element);
  return (Number.parseFloat(style.paddingLeft) || 0) + (Number.parseFloat(style.paddingRight) || 0);
}

export function terminalBox(options: {
  readonly scroller: HTMLElement;
  readonly panel: HTMLElement;
  readonly viewport: HTMLElement;
  readonly terminalHeight: number;
}): { width: number; height: number } {
  const { scroller, panel, viewport, terminalHeight } = options;
  const aroundTerminal = panel.offsetHeight - terminalHeight;
  return {
    width: scroller.clientWidth - horizontalPadding(scroller),
    height: viewport.clientHeight - aroundTerminal,
  };
}

function sameGrid(left: TerminalGrid | null, right: TerminalGrid): boolean {
  return left?.cols === right.cols && left.rows === right.rows;
}

export function settledReporter(report: (grid: TerminalGrid) => void, settleMs: number) {
  let reported: TerminalGrid | null = null;
  let settle = 0;
  return {
    offer(grid: TerminalGrid) {
      window.clearTimeout(settle);
      if (sameGrid(reported, grid)) return;
      settle = window.setTimeout(() => {
        reported = grid;
        report(grid);
      }, settleMs);
    },
    cancel() {
      window.clearTimeout(settle);
    },
  };
}
