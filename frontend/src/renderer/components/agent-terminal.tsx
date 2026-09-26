import { useEffect, useMemo, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { useTheme } from "@/hooks/use-theme";
import { terminalTheme } from "@/lib/terminal-palette";
import { plainOutput } from "@/lib/watch-status";
import "@xterm/xterm/css/xterm.css";

const COLS = 180;
const ROWS = 30;

function newTerminal(host: HTMLDivElement, box: HTMLDivElement): Terminal | null {
  try {
    const term = new Terminal({
      cols: COLS,
      rows: ROWS,
      fontSize: 11,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
      lineHeight: 1.2,
      scrollback: 5000,
      disableStdin: true,
      cursorBlink: false,
      theme: terminalTheme(getComputedStyle(box)),
    });
    term.open(host);
    return term;
  } catch {
    return null;
  }
}

type Props = { output: string };

export function AgentTerminal({ output }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const host = useRef<HTMLDivElement>(null);
  const term = useRef<Terminal | null>(null);
  const written = useRef("");
  const [emulated, setEmulated] = useState(false);
  const { theme } = useTheme();

  useEffect(() => {
    if (!host.current || !box.current) return;
    const created = newTerminal(host.current, box.current);
    term.current = created;
    written.current = "";
    setEmulated(created !== null);
    return () => {
      created?.dispose();
      term.current = null;
    };
  }, []);

  useEffect(() => {
    if (!term.current || !box.current) return;
    term.current.options.theme = terminalTheme(getComputedStyle(box.current));
  }, [theme]);

  useEffect(() => {
    const current = term.current;
    if (!current) return;
    if (output.startsWith(written.current)) {
      current.write(output.slice(written.current.length));
    } else {
      current.reset();
      current.write(output);
    }
    written.current = output;
  }, [output]);

  const text = useMemo(() => plainOutput(output).trimEnd(), [output]);

  return (
    <div ref={box} className="overflow-x-auto rounded-md border bg-background p-3 [&_.xterm-viewport]:bg-transparent!">
      <div ref={host} aria-hidden className={emulated ? undefined : "hidden"} />
      <pre
        aria-label="Agent output"
        className={emulated ? "sr-only" : "font-mono text-2xs/relaxed whitespace-pre-wrap"}
      >
        {text}
      </pre>
    </div>
  );
}
