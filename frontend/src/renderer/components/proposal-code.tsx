import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import {
  CheckIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  ChevronsDownUpIcon,
  ChevronsUpDownIcon,
  CircleXIcon,
  Columns2Icon,
  CopyIcon,
  ListTreeIcon,
  Maximize2Icon,
  RotateCcwIcon,
  Rows2Icon,
  TriangleAlertIcon,
  WrapTextIcon,
  type LucideIcon,
} from "lucide-react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import { CodeView, WorkerPoolContext, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import type { ProposalDetail, ProposalReply } from "@/hooks/useProposals";
import { useDiffPreferences, type DiffPreferences, type DiffStyle } from "@/hooks/use-diff-preferences";
import { useTheme } from "@/hooks/use-theme";
import {
  DIFF_THEMES,
  PREFERRED_HIGHLIGHTER,
  TOKENIZE_MAX_LINE_LENGTH,
  useDiffWorkerPool,
} from "@/lib/diff-worker-pool";
import { contentKey, viewKey, type DiffFile, type ProposalDiff, type ReplyAnchor } from "@/lib/proposal-diff";
import { readViewedFiles, storeViewedFiles, type ViewedFiles } from "@/lib/viewed-files";
import { Meta } from "@/components/status-badges";
import { count } from "@/components/proposal-dialogs";
import { changeIcon, FileTree, LineCounts, type MissingLoad } from "@/components/proposal-file-tree";
import { Tip } from "@/components/tip";
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
import { TooltipProvider } from "@/components/ui/tooltip";
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
  loads: ReadonlyMap<string, MissingLoad>;
  load: (path: string) => void;
};

type Fold = "folded" | "expanded" | null;

type Reading = {
  viewed: (file: DiffFile) => boolean;
  changedSinceViewed: (file: DiffFile) => boolean;
  collapsed: (file: DiffFile) => boolean;
  toggleViewed: (file: DiffFile) => void;
  toggleOpen: (file: DiffFile) => void;
  foldAll: (folded: boolean) => void;
};

const HEADER_HEIGHT = 40;

const ITEM_METRICS = { diffHeaderHeight: HEADER_HEIGHT, paddingTop: 0, paddingBottom: 8 };

const FLUSH = { paddingTop: 0, paddingBottom: 0, gap: 0 };

const HEADER_SLACK = 8;

const COPIED_MS = 1500;

function flip(paths: ReadonlySet<string>, path: string): ReadonlySet<string> {
  return withPath(paths, path, !paths.has(path));
}

function withPath(paths: ReadonlySet<string>, path: string, kept: boolean): ReadonlySet<string> {
  if (paths.has(path) === kept) return paths;
  const next = new Set(paths);
  if (kept) next.add(path);
  else next.delete(path);
  return next;
}

function foldedByDefault(fold: Fold, file: DiffFile, viewed: boolean): boolean {
  if (fold === null) return file.noisy || viewed;
  return fold === "folded";
}

function without(files: ViewedFiles, path: string): ViewedFiles {
  return Object.fromEntries(Object.entries(files).filter(([p]) => p !== path));
}

function useReading(viewedKey: string): Reading {
  const [viewedFiles, setViewedFiles] = useState<ViewedFiles>(() => readViewedFiles(viewedKey));
  const [fold, setFold] = useState<Fold>(null);
  const [toggled, setToggled] = useState<ReadonlySet<string>>(new Set());

  const viewed = useCallback((file: DiffFile) => viewedFiles[file.path] === viewKey(file), [viewedFiles]);

  const changedSinceViewed = useCallback(
    (file: DiffFile) => {
      const seen = viewedFiles[file.path];
      return seen !== undefined && seen !== viewKey(file);
    },
    [viewedFiles],
  );

  const collapsed = useCallback(
    (file: DiffFile) => toggled.has(file.path) !== foldedByDefault(fold, file, viewed(file)),
    [fold, toggled, viewed],
  );

  const toggleViewed = useCallback(
    (file: DiffFile) => {
      const nowViewed = !viewed(file);
      const next = nowViewed ? { ...viewedFiles, [file.path]: viewKey(file) } : without(viewedFiles, file.path);
      setViewedFiles(next);
      storeViewedFiles(viewedKey, next);
      const foldedAway = foldedByDefault(fold, file, nowViewed);
      setToggled((t) => withPath(t, file.path, foldedAway !== nowViewed));
    },
    [viewed, viewedFiles, viewedKey, fold],
  );

  const toggleOpen = useCallback((file: DiffFile) => setToggled((t) => flip(t, file.path)), []);

  const foldAll = useCallback((folded: boolean) => {
    setFold(folded ? "folded" : "expanded");
    setToggled(new Set());
  }, []);

  return useMemo(
    () => ({ viewed, changedSinceViewed, collapsed, toggleViewed, toggleOpen, foldAll }),
    [viewed, changedSinceViewed, collapsed, toggleViewed, toggleOpen, foldAll],
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
  viewedKey,
  ...card
}: CardProps & {
  title: string;
  actions: ReactNode;
  notice: ReactNode;
  viewedKey: string;
}) {
  const reading = useReading(viewedKey);
  const [full, setFull] = useState(false);
  const pool = useDiffWorkerPool();

  return (
    <WorkerPoolContext.Provider value={pool}>
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
    </WorkerPoolContext.Provider>
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
  const viewedPaths = useMemo(
    () => new Set((diff?.files ?? []).filter((f) => reading.viewed(f)).map((f) => f.path)),
    [diff, reading],
  );
  const loadable = (diff?.missing ?? []).filter((f) => shown.loads.get(f.path) !== "too-large").length;

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
        viewedCount={viewedPaths.size}
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
            viewed={viewedPaths}
            current={shownOrFirst(diff.files, current, reading.collapsed)}
            onJump={jump}
            loads={shown.loads}
            onLoad={shown.load}
          />
        ) : null}
        <div className="flex min-w-0 flex-1 flex-col">
          {shown.truncated && loadable > 0 ? (
            <CutNotice missing={loadable} tree={tree} onTree={() => setTree(true)} />
          ) : null}
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
  viewedCount,
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
  viewedCount: number;
  preferences: DiffPreferences;
  onPreferences: (next: Partial<DiffPreferences>) => void;
  tree: boolean;
  onTree: (tree: boolean) => void;
  onFull?: () => void;
}) {
  const files = diff?.files ?? [];
  const every = [...files, ...(diff?.missing ?? [])];
  const added = every.reduce((sum, f) => sum + f.added, 0);
  const deleted = every.reduce((sum, f) => sum + f.deleted, 0);
  const allFolded = files.length > 0 && files.every((f) => reading.collapsed(f));
  const FoldIcon = allFolded ? ChevronsUpDownIcon : ChevronsDownUpIcon;
  const foldLabel = allFolded ? "Expand all" : "Collapse all";
  return (
    <TooltipProvider delayDuration={300}>
      <div role="toolbar" aria-label="Diff tools" className="flex h-12 shrink-0 items-center gap-3 border-b px-2">
        <CommitMenu commits={commits} commit={commit} onCommit={onCommit} />
        {diff ? (
          <span className="shrink-0 text-sm text-muted-foreground">
            {count(every.length, "file")}
            {viewedCount > 0 ? ` · ${viewedCount} viewed` : ""}
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
              onClick={() => reading.foldAll(!allFolded)}
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

function CutNotice({ missing, tree, onTree }: { missing: number; tree: boolean; onTree: () => void }) {
  return (
    <Alert className="rounded-none border-x-0 border-t-0">
      <TriangleAlertIcon />
      <AlertTitle>The diff is longer than one megabyte.</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-2">
        <span>
          It stops at the last whole file under that size. Load the {count(missing, "file")} after it one at a time from
          the file tree.
        </span>
        {tree ? null : (
          <Button size="xs" variant="outline" onClick={onTree}>
            <ListTreeIcon data-icon="inline-start" />
            Open the file tree
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}

type Collapsed = Reading["collapsed"];

function firstOpen(files: DiffFile[], collapsed: Collapsed): string | undefined {
  return (files.find((f) => !collapsed(f)) ?? files[0])?.path;
}

function shownOrFirst(files: DiffFile[], path: string | undefined, collapsed: Collapsed): string | undefined {
  const shown = files.some((f) => f.path === path);
  return shown ? path : firstOpen(files, collapsed);
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

type HeaderState = { byPath: ReadonlyMap<string, DiffFile>; reading: Reading };

type ReplyState = { byId: ReadonlyMap<number, ProposalReply>; renderReply: RenderReply };

const HeaderContext = createContext<HeaderState | null>(null);

const ReplyContext = createContext<ReplyState | null>(null);

function FileHeaderSlot({ path }: { path: string }) {
  const state = useContext(HeaderContext);
  const file = state?.byPath.get(path);
  if (!state || !file) return null;
  const { reading } = state;
  return (
    <FileHeader
      file={file}
      viewed={reading.viewed(file)}
      changed={reading.changedSinceViewed(file)}
      collapsed={reading.collapsed(file)}
      onViewed={() => reading.toggleViewed(file)}
      onToggle={() => reading.toggleOpen(file)}
    />
  );
}

function InlineReply({ anchor }: { anchor: ReplyAnchor | undefined }) {
  const state = useContext(ReplyContext);
  const reply = anchor ? state?.byId.get(anchor.replyId) : undefined;
  if (!state || !anchor || !reply) return null;
  return (
    <div className="border-y bg-background px-3 py-2.5 font-sans text-foreground">
      {state.renderReply(reply, anchor)}
    </div>
  );
}

function renderHeader(item: CodeViewItem<ReplyAnchor>) {
  return <FileHeaderSlot path={item.id} />;
}

function renderInlineReply(annotation: DiffLineAnnotation<ReplyAnchor>) {
  return <InlineReply anchor={annotation.metadata} />;
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
  const headerState = useMemo(() => ({ byPath, reading }), [byPath, reading]);
  const replyState = useMemo(() => ({ byId, renderReply }), [byId, renderReply]);

  const items = useMemo(
    () =>
      diff.files.map((f): CodeViewItem<ReplyAnchor> => {
        const collapsed = reading.collapsed(f);
        const notes = annotations.get(f.path) ?? [];
        const key = `${contentKey(f.diff)}|${collapsed}|${notes.map((a) => a.metadata.replyId).join(",")}`;
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
      theme: DIFF_THEMES,
      themeType: theme,
      diffStyle: style,
      overflow: wrap ? ("wrap" as const) : ("scroll" as const),
      preferredHighlighter: PREFERRED_HIGHLIGHTER,
      tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH,
      lineDiffType: "word" as const,
      hunkSeparators: "line-info" as const,
      stickyHeaders: true,
      itemMetrics: ITEM_METRICS,
      layout: FLUSH,
    }),
    [theme, style, wrap],
  );

  const onScroll = useCallback(
    (scrollTop: number, view: { getTopForItem: (id: string) => number | undefined }) =>
      onCurrent(fileInView(diff.files, reading.collapsed, (id) => view.getTopForItem(id), scrollTop)),
    [diff.files, reading.collapsed, onCurrent],
  );

  return (
    <HeaderContext.Provider value={headerState}>
      <ReplyContext.Provider value={replyState}>
        <div role="group" aria-label="Diff" className="flex min-h-0 flex-1 flex-col diff-colors">
          <CodeView<ReplyAnchor, undefined>
            ref={ref}
            items={items}
            options={options}
            className="min-h-0 flex-1 overflow-auto"
            onScroll={onScroll}
            renderCustomHeader={renderHeader}
            renderAnnotation={renderInlineReply}
          />
        </div>
      </ReplyContext.Provider>
    </HeaderContext.Provider>
  );
}

function CopyPath({ path }: { path: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () =>
    void navigator.clipboard.writeText(path).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), COPIED_MS);
    });
  const Icon = copied ? CheckIcon : CopyIcon;
  return (
    <Tip label={copied ? "Copied" : "Copy the path"}>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={copied ? `Copied ${path}` : `Copy the path of ${path}`}
        className="shrink-0 text-muted-foreground"
        onClick={copy}
      >
        <Icon className={cn(copied && "text-success")} />
      </Button>
    </Tip>
  );
}

function FileHeader({
  file,
  viewed,
  changed,
  collapsed,
  onViewed,
  onToggle,
}: {
  file: DiffFile;
  viewed: boolean;
  changed: boolean;
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
      <CopyPath path={file.path} />
      {changed ? (
        <Badge
          variant="outline"
          className="shrink-0 font-mono text-attention"
          title="The file changed after you marked it viewed"
        >
          changed since viewed
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
        <LineCounts added={file.added} deleted={file.deleted} binary={file.binary} className="text-xs" />
      </span>
    </div>
  );
}
