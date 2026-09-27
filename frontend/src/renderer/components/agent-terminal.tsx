import { useEffect, useMemo, useRef, useState } from "react";
import { useTheme } from "@/hooks/use-theme";
import { GhosttySurface } from "@/lib/ghostty/surface";
import { terminalTheme } from "@/lib/terminal-palette";
import { plainOutput } from "@/lib/watch-status";

const COLS = 180;
const ROWS = 30;

type Props = { output: string };

export function AgentTerminal({ output }: Props) {
  const frame = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const written = useRef("");
  const [surface, setSurface] = useState<GhosttySurface | null>(null);
  const { theme } = useTheme();

  useEffect(() => {
    if (!frame.current || !scroller.current) return;
    let created: GhosttySurface | null = null;
    let unmounted = false;
    const elements = { frame: frame.current, scroller: scroller.current };
    const options = { cols: COLS, rows: ROWS, theme: terminalTheme(getComputedStyle(frame.current)) };
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
    <div ref={frame} className="relative rounded-md border bg-background">
      <div ref={scroller} aria-hidden className={emulated ? "overflow-x-auto p-3" : "hidden"} />
      <pre
        aria-label="Agent output"
        className={emulated ? "sr-only" : "p-3 font-mono text-2xs/relaxed whitespace-pre-wrap"}
      >
        {text}
      </pre>
    </div>
  );
}
