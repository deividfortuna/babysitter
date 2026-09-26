import type { ReactNode } from "react";
import type { Watch } from "@/hooks/useWatches";
import { Meta } from "@/components/status-badges";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { duration, shortSha } from "@/lib/time";
import { watchLabel, worktreeText } from "@/lib/watch-status";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <span className="self-center eyebrow">{label}</span>
      <span className="text-sm">{children}</span>
    </>
  );
}

type Props = {
  watch: Watch | null;
  onClose: () => void;
  onWatchAnother: () => void;
};

export function StopSummaryDialog({ watch, onClose, onWatchAnother }: Props) {
  const sum = watch?.summary;
  const activity = sum?.activity ?? {};
  const comments = (activity.comment ?? 0) + (activity.review_comment ?? 0) + (activity.review ?? 0);
  return (
    <Dialog open={watch !== null} onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        {watch ? (
          <>
            <DialogHeader>
              <DialogTitle>Stopped watching {watchLabel(watch)}</DialogTitle>
              <DialogDescription className="font-mono text-xs">
                reason: {sum?.reason || watch.stopReason || "user"} · watched{" "}
                {duration(watch.startedAt, watch.stoppedAt)}
              </DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-[110px_minmax(0,1fr)] gap-x-3 gap-y-2">
              <Row label="PR state">{sum?.prState || watch.prState || "unknown"}</Row>
              <Row label="Head">
                <span className="font-mono text-xs">{shortSha(sum?.headSha || watch.headSha)}</span>
                {watch.title ? ` · ${watch.title}` : ""}
              </Row>
              <Row label="Checks">{sum?.checks || "unknown"}</Row>
              <Row label="Agent">
                {sum ? `${sum.messages} ${sum.messages === 1 ? "message" : "messages"}` : "unknown"}
                {comments ? ` · ${comments} review ${comments === 1 ? "item" : "items"}` : ""}
                {activity.commit ? ` · ${activity.commit} ${activity.commit === 1 ? "commit" : "commits"}` : ""}
              </Row>
              <Row label="Worktree">{worktreeText(watch)}</Row>
            </div>
            {sum?.detail ? (
              <div className="flex flex-col gap-1 rounded-md border px-3 py-2.5">
                <Meta>{sum.detail}</Meta>
              </div>
            ) : null}
            <DialogFooter className="sm:justify-start">
              <Button onClick={onClose}>Done</Button>
              <Button variant="outline" onClick={onWatchAnother}>
                Watch another PR
              </Button>
              <Meta className="self-center sm:ml-auto">the transcript stays under Stopped</Meta>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
