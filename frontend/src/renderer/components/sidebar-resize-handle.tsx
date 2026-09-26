import { useState, type KeyboardEvent, type PointerEvent } from "react";
import { cn } from "@/lib/utils";
import { SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH } from "@/hooks/use-sidebar-width";

const KEYBOARD_STEP = 16;

type Props = {
  width: number;
  onResize: (width: number) => void;
  onReset: () => void;
  onResizingChange?: (resizing: boolean) => void;
};

export function SidebarResizeHandle({ width, onResize, onReset, onResizingChange }: Props) {
  const [resizing, setResizing] = useState(false);

  function start(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    setResizing(true);
    onResizingChange?.(true);
  }

  function move(event: PointerEvent<HTMLDivElement>) {
    if (!resizing) return;
    onResize(event.clientX);
  }

  function stop(event: PointerEvent<HTMLDivElement>) {
    if (!resizing) return;
    event.currentTarget.releasePointerCapture(event.pointerId);
    setResizing(false);
    onResizingChange?.(false);
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowLeft") onResize(width - KEYBOARD_STEP);
    else if (event.key === "ArrowRight") onResize(width + KEYBOARD_STEP);
    else if (event.key === "Home") onResize(SIDEBAR_MIN_WIDTH);
    else if (event.key === "End") onResize(SIDEBAR_MAX_WIDTH);
    else return;
    event.preventDefault();
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize sidebar"
      aria-valuemin={SIDEBAR_MIN_WIDTH}
      aria-valuemax={SIDEBAR_MAX_WIDTH}
      aria-valuenow={width}
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
        "absolute inset-y-0 -right-1 z-20 hidden w-2 cursor-col-resize touch-none outline-hidden sm:block",
        "after:absolute after:inset-y-0 after:left-1/2 after:w-0.5 after:-translate-x-1/2 after:transition-colors",
        "hover:after:bg-sidebar-border focus-visible:after:bg-sidebar-ring data-resizing:after:bg-sidebar-ring",
        "group-data-[collapsible=offcanvas]:hidden",
      )}
    />
  );
}
