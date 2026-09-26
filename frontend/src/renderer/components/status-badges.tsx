import type { ComponentProps } from "react";
import type { Watch } from "@/hooks/useWatches";
import { Badge } from "@/components/ui/badge";
import {
  checksWord,
  failedCheckNames,
  isAgentLive,
  mergeWord,
  mergeableWord,
  sessionWord,
  stopWord,
  type SessionState,
  type Tone,
} from "@/lib/watch-status";
import { cn } from "@/lib/utils";

export function Meta({ className, ...props }: ComponentProps<"span">) {
  return <span className={cn("font-mono text-2xs text-muted-foreground", className)} {...props} />;
}

export function AttentionBadge({ className, ...props }: ComponentProps<typeof Badge>) {
  return (
    <Badge
      className={cn("border-transparent bg-attention font-mono text-attention-foreground", className)}
      {...props}
    />
  );
}

export function ToneBadge({ tone, className, ...props }: ComponentProps<typeof Badge> & { tone: Tone }) {
  return (
    <Badge
      variant="outline"
      className={cn(
        "font-mono",
        tone === "done" && "border-success/60 text-success",
        tone === "bad" && "border-destructive/60 text-destructive",
        className,
      )}
      {...props}
    />
  );
}

export function LiveDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      title="The agent works right now"
      className={cn("relative inline-flex size-1.5 shrink-0", className)}
    >
      <span className="absolute inset-0 animate-ping rounded-full bg-success opacity-75 motion-reduce:animate-none" />
      <span className="relative size-full rounded-full bg-success" />
    </span>
  );
}

export function SessionBadge({ state, className }: { state: SessionState; className?: string }) {
  const session = sessionWord(state);
  return (
    <ToneBadge tone={session.tone} className={className}>
      {isAgentLive(state) ? <LiveDot /> : null}
      {session.label}
    </ToneBadge>
  );
}

export function StopBadge({ watch, className }: { watch: Pick<Watch, "stopReason">; className?: string }) {
  const stop = stopWord(watch);
  return (
    <ToneBadge tone={stop.tone} className={className}>
      {stop.label}
    </ToneBadge>
  );
}

export function ChecksBadge({ watch }: { watch: Pick<Watch, "checkStates" | "greenSha" | "headSha"> }) {
  const checks = checksWord(watch);
  const failed = failedCheckNames(watch);
  return (
    <ToneBadge tone={checks.tone} title={failed.length > 0 ? failed.join(", ") : undefined}>
      {checks.label}
    </ToneBadge>
  );
}

export function MergeableBadge({ state }: { state: string }) {
  const word = mergeableWord(state);
  if (!word) return null;
  return <ToneBadge tone={word.tone}>{word.label}</ToneBadge>;
}

export function MergeBadge({ watch }: { watch: Pick<Watch, "status" | "readySince" | "readyBlockers"> }) {
  const word = mergeWord(watch);
  if (!word) return null;
  return <ToneBadge tone={word.tone}>{word.label}</ToneBadge>;
}
