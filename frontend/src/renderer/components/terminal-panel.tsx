import type { CSSProperties } from "react";
import { PanelBottomDashedIcon, TerminalIcon } from "lucide-react";
import { useResizeTerminal, useWatchOutput } from "@/hooks/useSession";
import { useTerminalPanelHeight } from "@/hooks/use-terminal-panel-height";
import type { Watch } from "@/hooks/useWatches";
import { AgentTerminal } from "@/components/agent-terminal";
import { Meta } from "@/components/status-badges";
import { TerminalPanelResizeHandle } from "@/components/terminal-panel-resize-handle";
import { Tip } from "@/components/tip";
import { Button } from "@/components/ui/button";
import { isTakenOver } from "@/lib/watch-status";

type Props = { watch: Watch; enabled: boolean; onClose: () => void };

export function TerminalPanel({ watch, enabled, onClose }: Props) {
  const output = useWatchOutput(enabled ? watch.id : null);
  const resize = useResizeTerminal();
  const panel = useTerminalPanelHeight();
  const active = watch.status === "active";
  const title = isTakenOver(watch) ? "Last session" : "Terminal";

  return (
    <section
      aria-labelledby="terminal-panel-title"
      style={{ "--terminal-panel-height": `${panel.height}px` } as CSSProperties}
      className="relative col-start-1 row-start-2 flex h-(--terminal-panel-height) max-h-[80vh] min-w-0 flex-col border-t bg-background"
    >
      <TerminalPanelResizeHandle height={panel.height} onResize={panel.setHeight} onReset={panel.resetHeight} />
      <div className="flex h-10 shrink-0 items-center gap-2 border-b px-5">
        <TerminalIcon className="size-4 shrink-0 text-muted-foreground" />
        <h2 id="terminal-panel-title" className="text-sm font-medium">
          {title}
        </h2>
        <Meta>{watch.provider}</Meta>
        <Tip label="Close the terminal" side="left">
          <Button
            variant="ghost"
            size="icon"
            className="ml-auto size-7"
            aria-label="Close the terminal"
            onClick={onClose}
          >
            <PanelBottomDashedIcon />
          </Button>
        </Tip>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="px-5 py-3">
          {output.error || !output.data ? (
            <pre
              aria-label="Agent output"
              className="overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-2xs/relaxed whitespace-pre-wrap"
            >
              {output.error ? output.error.message : "Nothing printed yet."}
            </pre>
          ) : (
            <AgentTerminal
              output={output.data}
              onResize={active ? (grid) => resize.mutate({ id: watch.id, ...grid }) : undefined}
            />
          )}
        </div>
      </div>
    </section>
  );
}
