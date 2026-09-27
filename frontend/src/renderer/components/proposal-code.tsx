import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import {
  CheckIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  Columns2Icon,
  Maximize2Icon,
  RowsIcon,
  ScissorsIcon,
  TriangleAlertIcon,
  WrapTextIcon,
} from "lucide-react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import { CodeView, WorkerPoolContextProvider, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import type { ProposalDetail, ProposalReply } from "@/hooks/useProposals";
import { useDiffPreferences, type DiffStyle } from "@/hooks/use-diff-preferences";
import { useTheme } from "@/hooks/use-theme";
import type { DiffFile, ProposalDiff, ReplyAnchor } from "@/lib/proposal-diff";
import { Meta } from "@/components/status-badges";
import { count } from "@/components/proposal-dialogs";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Toggle } from "@/components/ui/toggle";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { shortSha } from "@/lib/time";
import { cn } from "@/lib/utils";

type Commit = NonNullable<ProposalDetail["commits"]>[number];

export type RenderReply = (reply: ProposalReply, anchor?: ReplyAnchor) => ReactNode;

type Reading = {
  viewed: ReadonlySet<string>;
  collapsed: (file: DiffFile) => boolean;
  toggleViewed: (path: string) => void;
  toggleOpen: (file: DiffFile) => void;
};

const THEMES = { light: "github-light-default", dark: "github-dark-default" } as const;

const POOL = {
  workerFactory: () => new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" }),
  poolSize: 2,
};
const HIGHLIGHTER = { theme: THEMES, preferredHighlighter: "shiki-js" } as const;

function useReading(): Reading {
  const [viewed, setViewed] = useState<ReadonlySet<string>>(new Set());
  const [open, setOpen] = useState<Record<string, boolean>>({});

  const collapsed = useCallback((file: DiffFile) => !(open[file.path] ?? !file.noisy), [open]);

  const toggleViewed = useCallback(
    (path: string) => {
      const nowViewed = !viewed.has(path);
      const next = new Set(viewed);
      if (nowViewed) next.add(path);
      else next.delete(path);
      setViewed(next);
      setOpen((o) => ({ ...o, [path]: !nowViewed }));
    },
    [viewed],
  );

  const toggleOpen = useCallback(
    (file: DiffFile) => setOpen((o) => ({ ...o, [file.path]: !(o[file.path] ?? !file.noisy) })),
    [],
  );

  return { viewed, collapsed, toggleViewed, toggleOpen };
}

function useVersions() {
  const seen = useRef(new Map<string, { key: string; version: number }>());
  return useCallback((id: string, key: string) => {
    const last = seen.current.get(id);
    if (last?.key === key) return last.version;
    const version = (last?.version ?? 0) + 1;
    seen.current.set(id, { key, version });
    return version;
  }, []);
}

export function CodeArea({
  diff,
  commits,
  truncated,
  annotations,
  replies,
  renderReply,
  dimmed,
}: {
  diff: ProposalDiff;
  commits: Commit[];
  truncated: boolean;
  annotations: Map<string, DiffLineAnnotation<ReplyAnchor>[]>;
  replies: ProposalReply[];
  renderReply: RenderReply;
  dimmed: boolean;
}) {
  const reading = useReading();
  const { preferences, change } = useDiffPreferences();
  const [full, setFull] = useState(false);
  const viewer = useRef<CodeViewHandle<ReplyAnchor, undefined>>(null);

  const jump = useCallback((path: string) => {
    viewer.current?.scrollTo({ type: "item", id: path, align: "start", behavior: "smooth" });
  }, []);

  const body = (fill: boolean) => (
    <div className={cn("grid gap-4 md:grid-cols-[300px_1fr]", fill && "min-h-0 flex-1", dimmed && "opacity-50")}>
      <div className={cn("flex min-w-0 flex-col gap-3", fill && "overflow-y-auto")}>
        <CommitList commits={commits} />
        <FileList diff={diff} reading={reading} onJump={jump} />
      </div>
      <div className="flex min-w-0 flex-col gap-2">
        <Toolbar
          diff={diff}
          layout={preferences.style}
          wrap={preferences.wrap}
          full={fill}
          onLayout={(style) => change({ style })}
          onWrap={(wrap) => change({ wrap })}
          onFull={() => setFull(true)}
        />
        {truncated ? <CutNotice missing={diff.missing.length} /> : null}
        <DiffViewer
          ref={viewer}
          diff={diff}
          annotations={annotations}
          replies={replies}
          renderReply={renderReply}
          reading={reading}
          layout={preferences.style}
          wrap={preferences.wrap}
          className={fill ? "min-h-0 flex-1" : "h-[60vh] min-h-72"}
        />
      </div>
    </div>
  );

  return (
    <WorkerPoolContextProvider poolOptions={POOL} highlighterOptions={HIGHLIGHTER}>
      {full ? <p className="text-sm text-muted-foreground">The diff is open in the full window.</p> : body(false)}
      <Dialog open={full} onOpenChange={setFull}>
        <DialogContent className="flex h-[calc(100vh-2rem)] max-h-[calc(100vh-2rem)] flex-col gap-3 p-4 sm:max-w-[calc(100vw-2rem)]">
          <DialogTitle className="text-title font-medium">Files changed</DialogTitle>
          <DialogDescription className="sr-only">The diff of the proposal in the full window.</DialogDescription>
          {body(true)}
        </DialogContent>
      </Dialog>
    </WorkerPoolContextProvider>
  );
}

function CommitList({ commits }: { commits: Commit[] }) {
  const heldBack = commits.filter((c) => c.heldBack).length;
  return (
    <div className="flex flex-col gap-1">
      <span className="eyebrow">{count(commits.length, "commit")} on the work branch</span>
      {commits.map((c) => (
        <div key={c.sha} className="flex items-baseline gap-2 text-sm">
          <Meta>{shortSha(c.sha)}</Meta>
          <span className="truncate">{c.subject}</span>
          {c.heldBack ? (
            <Badge variant="outline" className="shrink-0 font-mono">
              kept off before
            </Badge>
          ) : null}
        </div>
      ))}
      {heldBack > 0 ? (
        <p className="text-sm text-attention">
          {heldBack === 1
            ? "1 commit here is one you kept off the pull request in an earlier decision. Approving pushes it with the rest."
            : `${heldBack} commits here are ones you kept off the pull request in an earlier decision. Approving pushes them with the rest.`}
        </p>
      ) : null}
    </div>
  );
}

function FileList({ diff, reading, onJump }: { diff: ProposalDiff; reading: Reading; onJump: (path: string) => void }) {
  const total = diff.files.length + diff.missing.length;
  const viewed = diff.files.filter((f) => reading.viewed.has(f.path)).length;
  return (
    <div className="flex flex-col gap-0.5">
      <span className="eyebrow">
        {count(total, "changed file")}
        {viewed > 0 ? ` · ${viewed} viewed` : ""}
      </span>
      {diff.files.map((f) => (
        <button
          key={f.path}
          type="button"
          onClick={() => onJump(f.path)}
          className="flex items-baseline justify-between gap-2 rounded-md px-2 py-1 text-left hover:bg-muted"
        >
          <span className="flex min-w-0 items-baseline gap-1.5">
            {reading.viewed.has(f.path) ? (
              <>
                <CheckIcon aria-hidden className="size-3 shrink-0 self-center text-success" />
                <span className="sr-only">Viewed: </span>
              </>
            ) : null}
            <span className="truncate font-mono text-xs/normal">{f.path}</span>
          </span>
          <Meta className="shrink-0">
            +{f.added} −{f.deleted}
          </Meta>
        </button>
      ))}
      {diff.missing.map((f) => (
        <div key={f.path} className="flex items-baseline justify-between gap-2 px-2 py-1 text-muted-foreground">
          <span className="truncate font-mono text-xs/normal">{f.path}</span>
          <Meta className="shrink-0">not in the diff</Meta>
        </div>
      ))}
    </div>
  );
}

function Toolbar({
  diff,
  layout,
  wrap,
  full,
  onLayout,
  onWrap,
  onFull,
}: {
  diff: ProposalDiff;
  layout: DiffStyle;
  wrap: boolean;
  full: boolean;
  onLayout: (layout: DiffStyle) => void;
  onWrap: (wrap: boolean) => void;
  onFull: () => void;
}) {
  const added = diff.files.reduce((sum, f) => sum + f.added, 0);
  const deleted = diff.files.reduce((sum, f) => sum + f.deleted, 0);
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Meta>
        <span className="text-success">+{added}</span> <span className="text-destructive">−{deleted}</span>
      </Meta>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        className="ml-auto"
        aria-label="Diff layout"
        value={layout}
        onValueChange={(value) => {
          if (value === "unified" || value === "split") onLayout(value);
        }}
      >
        <ToggleGroupItem value="unified" aria-label="Unified">
          <RowsIcon />
          Unified
        </ToggleGroupItem>
        <ToggleGroupItem value="split" aria-label="Split">
          <Columns2Icon />
          Split
        </ToggleGroupItem>
      </ToggleGroup>
      <Toggle variant="outline" size="sm" aria-label="Wrap lines" pressed={wrap} onPressedChange={onWrap}>
        <WrapTextIcon />
        Wrap
      </Toggle>
      {full ? null : (
        <Button variant="outline" size="sm" onClick={onFull}>
          <Maximize2Icon data-icon="inline-start" />
          Full window
        </Button>
      )}
    </div>
  );
}

function CutNotice({ missing }: { missing: number }) {
  return (
    <Alert>
      <TriangleAlertIcon />
      <AlertTitle>The diff is longer than one megabyte and was cut.</AlertTitle>
      <AlertDescription>
        {missing > 0
          ? `The last file shown stops where the cut is, and ${count(missing, "file")} after it ${missing === 1 ? "is" : "are"} not in the diff.`
          : "The last file shown stops where the cut is."}
      </AlertDescription>
    </Alert>
  );
}

function DiffViewer({
  ref,
  diff,
  annotations,
  replies,
  renderReply,
  reading,
  layout,
  wrap,
  className,
}: {
  ref: React.Ref<CodeViewHandle<ReplyAnchor, undefined>>;
  diff: ProposalDiff;
  annotations: Map<string, DiffLineAnnotation<ReplyAnchor>[]>;
  replies: ProposalReply[];
  renderReply: RenderReply;
  reading: Reading;
  layout: DiffStyle;
  wrap: boolean;
  className: string;
}) {
  const { theme } = useTheme();
  const version = useVersions();
  const byPath = useMemo(() => new Map(diff.files.map((f) => [f.path, f])), [diff.files]);
  const byId = useMemo(() => new Map(replies.map((r) => [r.id, r])), [replies]);

  const items = useMemo(
    () =>
      diff.files.map((f): CodeViewItem<ReplyAnchor> => {
        const collapsed = reading.collapsed(f);
        const notes = annotations.get(f.path) ?? [];
        const key = `${collapsed}|${notes.map((a) => a.metadata.replyId).join(",")}`;
        return {
          id: f.path,
          type: "diff",
          fileDiff: f.diff,
          annotations: notes,
          collapsed,
          version: version(f.path, key),
        };
      }),
    [diff.files, annotations, reading, version],
  );

  const options = useMemo(
    () => ({
      theme: THEMES,
      themeType: theme,
      diffStyle: layout,
      overflow: wrap ? ("wrap" as const) : ("scroll" as const),
      preferredHighlighter: "shiki-js" as const,
      lineDiffType: "word" as const,
      hunkSeparators: "line-info" as const,
      stickyHeaders: true,
    }),
    [theme, layout, wrap],
  );

  return (
    <div
      role="group"
      aria-label="Diff"
      className={cn(
        "flex min-h-0 flex-col [--diffs-font-family:var(--font-mono)] [--diffs-font-size:12px] [--diffs-header-font-family:var(--font-sans)] [--diffs-line-height:18px]",
        className,
      )}
    >
      <CodeView<ReplyAnchor, undefined>
        ref={ref}
        items={items}
        options={options}
        className="min-h-0 flex-1 overflow-auto rounded-md border"
        renderHeaderPrefix={(item) => {
          const file = byPath.get(item.id);
          if (!file) return null;
          const Chevron = reading.collapsed(file) ? ChevronRightIcon : ChevronDownIcon;
          return (
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={reading.collapsed(file) ? `Expand ${file.path}` : `Collapse ${file.path}`}
              onClick={() => reading.toggleOpen(file)}
            >
              <Chevron />
            </Button>
          );
        }}
        renderHeaderFilenameSuffix={(item) =>
          byPath.get(item.id)?.cut ? (
            <Badge variant="outline" className="ml-2 font-mono text-attention">
              <ScissorsIcon data-icon="inline-start" />
              cut here
            </Badge>
          ) : null
        }
        renderHeaderMetadata={(item) => {
          const file = byPath.get(item.id);
          if (!file) return null;
          return (
            <FileHeaderActions
              file={file}
              viewed={reading.viewed.has(file.path)}
              collapsed={reading.collapsed(file)}
              onViewed={() => reading.toggleViewed(file.path)}
              onLoad={() => reading.toggleOpen(file)}
            />
          );
        }}
        renderAnnotation={(annotation) => {
          const anchor = annotation.metadata;
          const reply = anchor ? byId.get(anchor.replyId) : undefined;
          if (!anchor || !reply) return null;
          return (
            <div className="border-y bg-background px-3 py-2.5 font-sans text-foreground">
              {renderReply(reply, anchor)}
            </div>
          );
        }}
      />
    </div>
  );
}

function FileHeaderActions({
  file,
  viewed,
  collapsed,
  onViewed,
  onLoad,
}: {
  file: DiffFile;
  viewed: boolean;
  collapsed: boolean;
  onViewed: () => void;
  onLoad: () => void;
}) {
  const id = `viewed-${file.path}`;
  return (
    <span className="flex items-center gap-3 font-sans text-xs text-muted-foreground">
      {collapsed && file.noisy && !viewed ? (
        <>
          <span>large or generated</span>
          <Button variant="outline" size="xs" onClick={onLoad}>
            Load diff
          </Button>
        </>
      ) : null}
      <span className="flex items-center gap-1.5">
        <Checkbox id={id} checked={viewed} onCheckedChange={onViewed} aria-label={`Viewed ${file.path}`} />
        <label htmlFor={id}>Viewed</label>
      </span>
    </span>
  );
}
