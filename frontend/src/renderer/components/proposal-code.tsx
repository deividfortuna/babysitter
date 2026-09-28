import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import {
  ChevronDownIcon,
  ChevronRightIcon,
  ChevronsDownUpIcon,
  ChevronsUpDownIcon,
  CircleXIcon,
  Columns2Icon,
  ListTreeIcon,
  Maximize2Icon,
  RotateCcwIcon,
  Rows2Icon,
  ScissorsIcon,
  TriangleAlertIcon,
  WrapTextIcon,
  type LucideIcon,
} from "lucide-react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import { CodeView, WorkerPoolContextProvider, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import type { ProposalDetail, ProposalReply } from "@/hooks/useProposals";
import { useDiffPreferences, type DiffPreferences, type DiffStyle } from "@/hooks/use-diff-preferences";
import { useTheme } from "@/hooks/use-theme";
import type { DiffFile, ProposalDiff, ReplyAnchor } from "@/lib/proposal-diff";
import { Meta } from "@/components/status-badges";
import { count } from "@/components/proposal-dialogs";
import { changeIcon, FileTree, LineCounts } from "@/components/proposal-file-tree";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Spinner } from "@/components/ui/spinner";
import { Toggle } from "@/components/ui/toggle";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { shortSha } from "@/lib/time";
import { cn } from "@/lib/utils";

export type Commit = NonNullable<ProposalDetail["commits"]>[number];

export type RenderReply = (reply: ProposalReply, anchor?: ReplyAnchor) => ReactNode;

export type ShownCode = {
  diff: ProposalDiff | null;
  base: string | undefined;
  truncated: boolean;
  loading: boolean;
  error: string | null;
  reload: () => void;
};

type Reading = {
  viewed: ReadonlySet<string>;
  collapsed: (file: DiffFile) => boolean;
  toggleViewed: (path: string) => void;
  toggleOpen: (file: DiffFile) => void;
  foldAll: (files: DiffFile[], folded: boolean) => void;
};

const THEMES = { light: "github-light-default", dark: "github-dark-default" } as const;

const POOL = {
  workerFactory: () => new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" }),
  poolSize: 2,
};
const HIGHLIGHTER = { theme: THEMES, preferredHighlighter: "shiki-js" } as const;

const HEADER_SLACK = 8;

const FLUSH = { paddingTop: 0, paddingBottom: 0, gap: 0 };

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

  const foldAll = useCallback(
    (files: DiffFile[], folded: boolean) =>
      setOpen((o) => ({ ...o, ...Object.fromEntries(files.map((f) => [f.path, !folded])) })),
    [],
  );

  return useMemo(
    () => ({ viewed, collapsed, toggleViewed, toggleOpen, foldAll }),
    [viewed, collapsed, toggleViewed, toggleOpen, foldAll],
  );
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

type CardProps = {
  shown: ShownCode;
  commits: Commit[];
  commit: string | null;
  onCommit: (sha: string | null) => void;
  annotations: Map<string, DiffLineAnnotation<ReplyAnchor>[]>;
  replies: ProposalReply[];
  renderReply: RenderReply;
  dimmed: boolean;
};

export function CodeArea({
  title,
  actions,
  notice,
  ...card
}: CardProps & {
  title: string;
  actions: ReactNode;
  notice: ReactNode;
}) {
  const reading = useReading();
  const [full, setFull] = useState(false);

  return (
    <WorkerPoolContextProvider poolOptions={POOL} highlighterOptions={HIGHLIGHTER}>
      {full ? (
        <p className="text-sm text-muted-foreground">The diff is open in the full window.</p>
      ) : (
        <DiffCard {...card} reading={reading} onFull={() => setFull(true)} />
      )}
      <Dialog open={full} onOpenChange={setFull}>
        <DialogContent className="flex h-[calc(100svh-var(--size-dialog-gutter))] max-h-[calc(100svh-var(--size-dialog-gutter))] w-[calc(100vw-var(--size-dialog-gutter))] max-w-none flex-col gap-4 overflow-hidden p-4 sm:max-w-none">
          <div className="flex min-w-0 items-center gap-2 pr-8">
            <DialogTitle className="shrink-0 text-title font-medium">Files changed</DialogTitle>
            <Meta className="truncate">{title}</Meta>
            <div className="ml-auto flex shrink-0 items-center gap-2">{actions}</div>
          </div>
          <DialogDescription className="sr-only">The diff of the proposal in the full window.</DialogDescription>
          {notice}
          <DiffCard {...card} reading={reading} fill />
        </DialogContent>
      </Dialog>
    </WorkerPoolContextProvider>
  );
}

function DiffCard({
  shown,
  commits,
  commit,
  onCommit,
  annotations,
  replies,
  renderReply,
  dimmed,
  reading,
  fill = false,
  onFull,
}: CardProps & {
  reading: Reading;
  fill?: boolean;
  onFull?: () => void;
}) {
  const { preferences, change } = useDiffPreferences();
  const [tree, setTree] = useState(fill);
  const [current, setCurrent] = useState<string>();
  const viewer = useRef<CodeViewHandle<ReplyAnchor, undefined>>(null);
  const { diff } = shown;

  const jump = useCallback((path: string) => {
    setCurrent(path);
    viewer.current?.scrollTo({ type: "item", id: path, align: "start", behavior: "smooth" });
  }, []);

  return (
    <div
      className={cn(
        "flex flex-col overflow-hidden rounded-md border bg-background",
        fill ? "min-h-0 flex-1" : "h-[min(760px,75vh)] min-h-80",
        dimmed && "opacity-50",
      )}
    >
      <Toolbar
        diff={diff}
        commits={commits}
        commit={commit}
        onCommit={onCommit}
        reading={reading}
        preferences={preferences}
        onPreferences={change}
        tree={tree}
        onTree={setTree}
        onFull={onFull}
      />
      <div className="flex min-h-0 flex-1">
        {tree && diff ? (
          <FileTree
            diff={diff}
            viewed={reading.viewed}
            current={current ?? firstOpen(diff.files, reading.collapsed)}
            onJump={jump}
          />
        ) : null}
        <div className="flex min-w-0 flex-1 flex-col">
          {shown.truncated && diff ? <CutNotice missing={diff.missing.length} /> : null}
          {diff ? (
            <DiffViewer
              ref={viewer}
              diff={diff}
              annotations={annotations}
              replies={replies}
              renderReply={renderReply}
              reading={reading}
              preferences={preferences}
              onCurrent={setCurrent}
            />
          ) : (
            <CommitCodeState shown={shown} />
          )}
        </div>
      </div>
    </div>
  );
}

function CommitCodeState({ shown }: { shown: ShownCode }) {
  if (!shown.error) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner />
      </div>
    );
  }
  return (
    <Alert variant="destructive" className="m-3 w-auto">
      <CircleXIcon />
      <AlertTitle>The code of this commit could not be read.</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-2">
        <span>{shown.error}</span>
        <Button size="xs" variant="outline" disabled={shown.loading} onClick={shown.reload}>
          {shown.loading ? <Spinner data-icon="inline-start" /> : <RotateCcwIcon data-icon="inline-start" />}
          Read it again
        </Button>
      </AlertDescription>
    </Alert>
  );
}

function Tip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

const LAYOUTS: { value: DiffStyle; label: string; icon: LucideIcon }[] = [
  { value: "unified", label: "Unified", icon: Rows2Icon },
  { value: "split", label: "Split", icon: Columns2Icon },
];

function ToolToggle({
  label,
  icon: Icon,
  pressed,
  onPressedChange,
}: {
  label: string;
  icon: LucideIcon;
  pressed: boolean;
  onPressedChange: (pressed: boolean) => void;
}) {
  return (
    <Tip label={label}>
      <Toggle
        size="sm"
        aria-label={label}
        className="size-8 data-[state=on]:bg-muted data-[state=on]:text-foreground"
        pressed={pressed}
        onPressedChange={onPressedChange}
      >
        <Icon />
      </Toggle>
    </Tip>
  );
}

function isStyle(value: string): value is DiffStyle {
  return LAYOUTS.some((l) => l.value === value);
}

function Toolbar({
  diff,
  commits,
  commit,
  onCommit,
  reading,
  preferences,
  onPreferences,
  tree,
  onTree,
  onFull,
}: {
  diff: ProposalDiff | null;
  commits: Commit[];
  commit: string | null;
  onCommit: (sha: string | null) => void;
  reading: Reading;
  preferences: DiffPreferences;
  onPreferences: (next: Partial<DiffPreferences>) => void;
  tree: boolean;
  onTree: (tree: boolean) => void;
  onFull?: () => void;
}) {
  const files = diff?.files ?? [];
  const listed = files.length + (diff?.missing.length ?? 0);
  const seen = files.filter((f) => reading.viewed.has(f.path)).length;
  const added = files.reduce((sum, f) => sum + f.added, 0);
  const deleted = files.reduce((sum, f) => sum + f.deleted, 0);
  const allFolded = files.length > 0 && files.every((f) => reading.collapsed(f));
  const FoldIcon = allFolded ? ChevronsUpDownIcon : ChevronsDownUpIcon;
  const foldLabel = allFolded ? "Expand all" : "Collapse all";
  return (
    <TooltipProvider delayDuration={300}>
      <div role="toolbar" aria-label="Diff tools" className="flex h-12 shrink-0 items-center gap-3 border-b px-2">
        <CommitMenu commits={commits} commit={commit} onCommit={onCommit} />
        {diff ? (
          <span className="shrink-0 text-sm text-muted-foreground">
            {count(listed, "file")}
            {seen > 0 ? ` · ${seen} viewed` : ""}
          </span>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {diff ? <LineCounts added={added} deleted={deleted} className="text-xs" /> : null}
          <Tip label={foldLabel}>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={foldLabel}
              disabled={!diff}
              onClick={() => reading.foldAll(files, !allFolded)}
            >
              <FoldIcon />
            </Button>
          </Tip>
          <ToggleGroup
            type="single"
            size="sm"
            className="gap-0.5 rounded-lg bg-muted p-0.5"
            aria-label="Diff layout"
            value={preferences.style}
            onValueChange={(value) => {
              if (isStyle(value)) onPreferences({ style: value });
            }}
          >
            {LAYOUTS.map(({ value, label, icon: Icon }) => (
              <Tip key={value} label={label}>
                <ToggleGroupItem
                  value={value}
                  aria-label={label}
                  className="size-8 rounded-md! data-[state=on]:bg-background data-[state=on]:shadow-xs"
                >
                  <Icon />
                </ToggleGroupItem>
              </Tip>
            ))}
          </ToggleGroup>
          <ToolToggle
            label="Wrap lines"
            icon={WrapTextIcon}
            pressed={preferences.wrap}
            onPressedChange={(wrap) => onPreferences({ wrap })}
          />
          <ToolToggle label="File tree" icon={ListTreeIcon} pressed={tree} onPressedChange={onTree} />
          {onFull ? (
            <Tip label="Full window">
              <Button variant="ghost" size="icon-sm" aria-label="Full window" onClick={onFull}>
                <Maximize2Icon />
              </Button>
            </Tip>
          ) : null}
        </div>
      </div>
    </TooltipProvider>
  );
}

function CommitMenu({
  commits,
  commit,
  onCommit,
}: {
  commits: Commit[];
  commit: string | null;
  onCommit: (sha: string | null) => void;
}) {
  const chosen = commits.find((c) => c.sha === commit);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="secondary" size="sm" className="min-w-0 px-2.5" aria-label="Commits">
          <span className="max-w-72 truncate">{chosen ? chosen.subject : "All commits"}</span>
          <ChevronDownIcon className="text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-85 rounded-lg">
        <CommitItem current={commit === null} onSelect={() => onCommit(null)}>
          <span className="min-w-0 flex-1 truncate">All commits</span>
          <Meta>{count(commits.length, "commit")}</Meta>
        </CommitItem>
        {commits.map((c) => (
          <CommitItem key={c.sha} current={c.sha === commit} onSelect={() => onCommit(c.sha)}>
            <span className="min-w-0 flex-1 truncate">{c.subject}</span>
            {c.heldBack ? (
              <Badge variant="outline" className="shrink-0 font-mono">
                kept off before
              </Badge>
            ) : null}
            <Meta>{shortSha(c.sha)}</Meta>
          </CommitItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function CommitItem({ current, onSelect, children }: { current: boolean; onSelect: () => void; children: ReactNode }) {
  return (
    <DropdownMenuItem
      aria-current={current ? "true" : undefined}
      onSelect={onSelect}
      className="gap-3 rounded-md px-2.5 aria-current:bg-accent"
    >
      {children}
    </DropdownMenuItem>
  );
}

function CutNotice({ missing }: { missing: number }) {
  return (
    <Alert className="rounded-none border-x-0 border-t-0">
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

type Collapsed = Reading["collapsed"];

function firstOpen(files: DiffFile[], collapsed: Collapsed): string | undefined {
  return (files.find((f) => !collapsed(f)) ?? files[0])?.path;
}

function fileInView(
  files: DiffFile[],
  collapsed: Collapsed,
  top: (id: string) => number | undefined,
  scrollTop: number,
): string | undefined {
  const edge = scrollTop + HEADER_SLACK;
  const shown = files.find((f, i) => {
    const next = files[i + 1];
    const bottom = next ? (top(next.path) ?? Infinity) : Infinity;
    return !collapsed(f) && bottom > edge;
  });
  return shown?.path ?? firstOpen(files, collapsed);
}

function DiffViewer({
  ref,
  diff,
  annotations,
  replies,
  renderReply,
  reading,
  preferences,
  onCurrent,
}: {
  ref: React.Ref<CodeViewHandle<ReplyAnchor, undefined>>;
  diff: ProposalDiff;
  annotations: Map<string, DiffLineAnnotation<ReplyAnchor>[]>;
  replies: ProposalReply[];
  renderReply: RenderReply;
  reading: Reading;
  preferences: DiffPreferences;
  onCurrent: (path: string | undefined) => void;
}) {
  const { style, wrap } = preferences;
  const { theme } = useTheme();
  const version = useVersions();
  const byPath = useMemo(() => new Map(diff.files.map((f) => [f.path, f])), [diff.files]);
  const byId = useMemo(() => new Map(replies.map((r) => [r.id, r])), [replies]);

  const items = useMemo(
    () =>
      diff.files.map((f): CodeViewItem<ReplyAnchor> => {
        const collapsed = reading.collapsed(f);
        const viewed = reading.viewed.has(f.path);
        const notes = annotations.get(f.path) ?? [];
        const key = `${collapsed}|${viewed}|${notes.map((a) => a.metadata.replyId).join(",")}`;
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
      diffStyle: style,
      overflow: wrap ? ("wrap" as const) : ("scroll" as const),
      preferredHighlighter: "shiki-js" as const,
      lineDiffType: "word" as const,
      hunkSeparators: "line-info" as const,
      stickyHeaders: true,
      layout: FLUSH,
    }),
    [theme, style, wrap],
  );

  return (
    <div role="group" aria-label="Diff" className="flex min-h-0 flex-1 flex-col diff-colors">
      <CodeView<ReplyAnchor, undefined>
        ref={ref}
        items={items}
        options={options}
        className="min-h-0 flex-1 overflow-auto"
        onScroll={(scrollTop, view) =>
          onCurrent(fileInView(diff.files, reading.collapsed, (id) => view.getTopForItem(id), scrollTop))
        }
        renderCustomHeader={(item) => {
          const file = byPath.get(item.id);
          if (!file) return null;
          return (
            <FileHeader
              file={file}
              viewed={reading.viewed.has(file.path)}
              collapsed={reading.collapsed(file)}
              onViewed={() => reading.toggleViewed(file.path)}
              onToggle={() => reading.toggleOpen(file)}
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

function FileHeader({
  file,
  viewed,
  collapsed,
  onViewed,
  onToggle,
}: {
  file: DiffFile;
  viewed: boolean;
  collapsed: boolean;
  onViewed: () => void;
  onToggle: () => void;
}) {
  const { icon: Icon, tone } = changeIcon(file.diff.type);
  const Chevron = collapsed ? ChevronRightIcon : ChevronDownIcon;
  const id = `viewed-${file.path}`;
  const waiting = collapsed && file.noisy && !viewed;
  return (
    <div className="flex h-10 w-full items-center gap-2.5 border-b bg-background pr-4 pl-2.5 font-sans text-sm text-foreground">
      <button
        type="button"
        aria-label={collapsed ? `Expand ${file.path}` : `Collapse ${file.path}`}
        aria-expanded={!collapsed}
        onClick={onToggle}
        className="flex min-w-0 items-center gap-2.5 rounded-sm text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <Chevron aria-hidden className="size-4 shrink-0 text-muted-foreground" />
        <Icon aria-hidden className={cn("size-4 shrink-0", tone)} />
        <span className={cn("truncate", viewed && "text-muted-foreground")}>{file.path}</span>
      </button>
      {file.cut ? (
        <Badge variant="outline" className="shrink-0 font-mono text-attention">
          <ScissorsIcon data-icon="inline-start" />
          cut here
        </Badge>
      ) : null}
      <span className="ml-auto flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
        {waiting ? (
          <>
            <span>large or generated</span>
            <Button variant="outline" size="xs" onClick={onToggle}>
              Load diff
            </Button>
          </>
        ) : null}
        <span className="flex items-center gap-1.5">
          <Checkbox id={id} checked={viewed} onCheckedChange={onViewed} aria-label={`Viewed ${file.path}`} />
          <label htmlFor={id}>Viewed</label>
        </span>
        <LineCounts added={file.added} deleted={file.deleted} className="text-xs" />
      </span>
    </div>
  );
}
