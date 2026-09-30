import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { useTheme } from "@/hooks/use-theme";
import { GhosttySurface, type GridLimits, type TerminalGrid } from "@/lib/ghostty/surface";
import { scrollingAncestor, settledReporter, terminalBox } from "@/lib/terminal-fit";
import { terminalTheme } from "@/lib/terminal-palette";
import { plainOutput } from "@/lib/watch-status";

const LIMITS: GridLimits = { minCols: 20, minRows: 10, maxRows: 60 };
const RESIZE_SETTLE_MS = 150;

type Props = {
  output: string;
  onResize?: (grid: TerminalGrid) => void;
};

export function AgentTerminal({ output, onResize }: Props) {
  const frame = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const written = useRef("");
  const [surface, setSurface] = useState<GhosttySurface | null>(null);
  const { theme } = useTheme();
  const reportResize = useEffectEvent((grid: TerminalGrid) => onResize?.(grid));

  useEffect(() => {
    if (!frame.current || !scroller.current) return;
    let created: GhosttySurface | null = null;
    let unmounted = false;
    const elements = { frame: frame.current, scroller: scroller.current };
    const options = { limits: LIMITS, theme: terminalTheme(getComputedStyle(frame.current)) };
    GhosttySurface.create(elements, options)
      .then((ready) => {
        if (unmounted) {
          ready?.dispose();
          return;
        }
        created = ready;
        written.current = "";
        setSurface(ready);
      })
      .catch((error: unknown) => console.error("The terminal could not start", error));
    return () => {
      unmounted = true;
      created?.dispose();
      setSurface(null);
    };
  }, []);

  useEffect(() => {
    if (!surface || !frame.current || !scroller.current) return;
    const pane = scroller.current;
    const panel = frame.current.parentElement ?? frame.current;
    const viewport = scrollingAncestor(panel);
    const reporter = settledReporter((grid) => reportResize(grid), RESIZE_SETTLE_MS);
    const fit = () => {
      const box = terminalBox({ scroller: pane, panel, viewport, terminalHeight: surface.height });
      surface.fit(box.width, box.height);
      reporter.offer(surface.grid);
    };
    const observer = new ResizeObserver(fit);
    for (const element of [pane, panel, viewport]) observer.observe(element);
    fit();
    return () => {
      observer.disconnect();
      reporter.cancel();
    };
  }, [surface]);

  useEffect(() => {
    if (!surface || !frame.current) return;
    surface.setTheme(terminalTheme(getComputedStyle(frame.current)));
  }, [surface, theme]);

  useEffect(() => {
    if (!surface) return;
    if (written.current.length > 0 && output.startsWith(written.current)) {
      surface.write(output.slice(written.current.length));
    } else {
      surface.resetAndWrite(output);
    }
    written.current = output;
  }, [surface, output]);

  const text = useMemo(() => plainOutput(output).trimEnd(), [output]);
  const emulated = surface !== null;

  return (
    <div ref={frame} className="relative bg-background">
      <div ref={scroller} aria-hidden className={emulated ? "overflow-x-auto px-5 py-2" : "hidden"} />
      <pre
        aria-label="Agent output"
        className={emulated ? "sr-only" : "px-5 py-2 font-mono text-2xs/relaxed whitespace-pre-wrap"}
      >
        {text}
      </pre>
    </div>
  );
}
