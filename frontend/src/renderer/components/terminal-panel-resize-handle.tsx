import { useState, type KeyboardEvent, type PointerEvent } from "react";
import { cn } from "@/lib/utils";
import { TERMINAL_PANEL_MIN_HEIGHT } from "@/hooks/use-terminal-panel-height";

const KEYBOARD_STEP = 16;

type Props = {
  height: number;
  maxHeight: number;
  onResize: (height: number) => void;
  onReset: () => void;
};

function panelBottom(handle: HTMLElement): number {
  return (handle.parentElement ?? handle).getBoundingClientRect().bottom;
}

export function TerminalPanelResizeHandle({ height, maxHeight, onResize, onReset }: Props) {
  const [resizing, setResizing] = useState(false);

  function start(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    setResizing(true);
  }

  function move(event: PointerEvent<HTMLDivElement>) {
    if (!resizing) return;
    onResize(panelBottom(event.currentTarget) - event.clientY);
  }

  function stop(event: PointerEvent<HTMLDivElement>) {
    if (!resizing) return;
    event.currentTarget.releasePointerCapture(event.pointerId);
    setResizing(false);
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowUp") onResize(height + KEYBOARD_STEP);
    else if (event.key === "ArrowDown") onResize(height - KEYBOARD_STEP);
    else if (event.key === "Home") onResize(TERMINAL_PANEL_MIN_HEIGHT);
    else if (event.key === "End") onResize(maxHeight);
    else return;
    event.preventDefault();
  }

  return (
    <div
      role="separator"
      aria-orientation="horizontal"
      aria-label="Resize terminal"
      aria-valuemin={TERMINAL_PANEL_MIN_HEIGHT}
      aria-valuemax={maxHeight}
      aria-valuenow={height}
      tabIndex={0}
      title="Drag to resize. Double click to reset."
      data-resizing={resizing || undefined}
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={stop}
      onPointerCancel={stop}
      onDoubleClick={onReset}
      onKeyDown={onKeyDown}
      className={cn(
        "absolute inset-x-0 -top-1 z-20 h-2 cursor-row-resize touch-none outline-hidden",
        "after:absolute after:inset-x-0 after:top-1/2 after:h-0.5 after:-translate-y-1/2 after:transition-colors",
        "hover:after:bg-border focus-visible:after:bg-ring data-resizing:after:bg-ring",
      )}
    />
  );
}
