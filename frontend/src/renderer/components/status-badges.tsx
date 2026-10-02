import type { ComponentProps } from "react";
import { CircleCheckIcon, CircleDotIcon, CircleXIcon, type LucideIcon } from "lucide-react";
import type { Watch } from "@/hooks/useWatches";
import { Badge } from "@/components/ui/badge";
import {
  autoTags,
  checksWord,
  failedCheckNames,
  isAgentLive,
  mergeWord,
  mergeableWord,
  sessionWord,
  stopWord,
  type SessionState,
  type Tag,
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

const LIVE_TONES = { success: "bg-success", attention: "bg-attention" };

type LiveDotProps = { className?: string; tone?: keyof typeof LIVE_TONES; title?: string };

export function LiveDot({ className, tone = "success", title = "The agent works right now" }: LiveDotProps) {
  const color = LIVE_TONES[tone];
  return (
    <span aria-hidden="true" title={title} className={cn("relative inline-flex size-1.5 shrink-0", className)}>
      <span className={cn("absolute inset-0 animate-ping rounded-full opacity-75 motion-reduce:animate-none", color)} />
      <span className={cn("relative size-full rounded-full", color)} />
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

export function QueuedBadge() {
  return (
    <ToneBadge tone="neutral" className="border-attention/50 text-attention">
      queued
    </ToneBadge>
  );
}

export function TagBadges({ tags }: { tags: Tag[] }) {
  return tags.map((tag) => (
    <ToneBadge key={tag.label} tone={tag.tone} title={tag.title}>
      {tag.label}
    </ToneBadge>
  ));
}

type AutoFields = Pick<Watch, "autoReason" | "updateType" | "mergeWhenReady" | "dependabot">;

export function AutoBadges({ watch }: { watch: AutoFields }) {
  return <TagBadges tags={autoTags(watch)} />;
}

const checkIcons: Partial<Record<Tone, { icon: LucideIcon; className: string }>> = {
  good: { icon: CircleCheckIcon, className: "text-success" },
  wait: { icon: CircleDotIcon, className: "text-chart-3" },
  bad: { icon: CircleXIcon, className: "text-destructive" },
};

export function ChecksIcon({ watch }: { watch: Pick<Watch, "checkStates" | "greenSha" | "headSha"> }) {
  const checks = checksWord(watch);
  const failed = failedCheckNames(watch);
  const text = failed.length > 0 ? `${checks.label}: ${failed.join(", ")}` : checks.label;
  return <CheckIcon tone={checks.tone} text={text} />;
}

export function CheckIcon({ tone, text }: { tone: Tone; text: string }) {
  const look = checkIcons[tone];
  if (!look) return null;
  return (
    <span title={text} className="inline-flex">
      <look.icon aria-hidden="true" className={cn("size-4", look.className)} />
      <span className="sr-only">{text}</span>
    </span>
  );
}
