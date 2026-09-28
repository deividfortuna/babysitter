import { useMemo, useState } from "react";
import {
  CheckIcon,
  RotateCcwIcon,
  ChevronRightIcon,
  FolderIcon,
  FolderOpenIcon,
  SearchIcon,
  SquareDotIcon,
  SquareIcon,
  SquareMinusIcon,
  SquarePlusIcon,
  type LucideIcon,
} from "lucide-react";
import { fileTree, type TreeNode } from "@/lib/file-tree";
import type { ProposalDiff } from "@/lib/proposal-diff";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import { count } from "@/components/proposal-dialogs";

type Change = "new" | "deleted" | "change" | "missing";

export type MissingLoad = "loading" | "too-large" | "failed";

type Listed = { path: string; added: number; deleted: number; binary: boolean; change: Change };

const CHANGE_ICONS: Record<Change, { icon: LucideIcon; tone: string; label: string }> = {
  new: { icon: SquarePlusIcon, tone: "text-success", label: "added" },
  deleted: { icon: SquareMinusIcon, tone: "text-destructive", label: "deleted" },
  change: { icon: SquareDotIcon, tone: "text-attention", label: "changed" },
  missing: { icon: SquareIcon, tone: "text-muted-foreground", label: "not in the diff" },
};

function changeOf(type: string): Change {
  if (type === "new" || type === "deleted") return type;
  return "change";
}

export function changeIcon(type: string) {
  return CHANGE_ICONS[changeOf(type)];
}

function spokenCounts(added: number, deleted: number): string {
  return `${count(added, "line")} added, ${count(deleted, "line")} deleted`;
}

export function LineCounts({
  added,
  deleted,
  binary = false,
  className,
}: {
  added: number;
  deleted: number;
  binary?: boolean;
  className?: string;
}) {
  if (binary) return <span className={cn("shrink-0 font-mono", className)}>binary</span>;
  return (
    <span role="group" aria-label={spokenCounts(added, deleted)} className={cn("shrink-0 font-mono", className)}>
      <span aria-hidden className="text-success">
        +{added}
      </span>{" "}
      <span aria-hidden className="text-destructive">
        −{deleted}
      </span>
    </span>
  );
}

function matches(path: string, filter: string): boolean {
  return path.toLowerCase().includes(filter.trim().toLowerCase());
}

type Rows = {
  viewed: ReadonlySet<string>;
  current: string | undefined;
  onJump: (path: string) => void;
  loads: ReadonlyMap<string, MissingLoad>;
  onLoad: (path: string) => void;
};

export function FileTree({ diff, ...rows }: Rows & { diff: ProposalDiff }) {
  const [filter, setFilter] = useState("");
  const tree = useMemo(() => {
    const listed: Listed[] = [
      ...diff.files.map((f) => ({
        path: f.path,
        added: f.added,
        deleted: f.deleted,
        binary: f.binary,
        change: changeOf(f.diff.type),
      })),
      ...diff.missing.map((f) => ({
        path: f.path,
        added: f.added,
        deleted: f.deleted,
        binary: f.binary ?? false,
        change: "missing" as const,
      })),
    ];
    return fileTree(
      listed.filter((f) => matches(f.path, filter)),
      (f) => f.path,
    );
  }, [diff, filter]);
  return (
    <nav
      aria-label="Changed files"
      className="flex w-65 shrink-0 flex-col gap-2 overflow-y-auto border-r p-2 text-body"
    >
      <div className="relative">
        <SearchIcon
          aria-hidden
          className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          type="search"
          aria-label="Filter files"
          placeholder="Filter files"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="h-8 pl-7 text-body md:text-body"
        />
      </div>
      {tree.length > 0 ? (
        <Level nodes={tree} {...rows} />
      ) : (
        <p className="px-1.5 text-muted-foreground">No file matches.</p>
      )}
    </nav>
  );
}

function Level({ nodes, nested = false, ...rows }: Rows & { nodes: TreeNode<Listed>[]; nested?: boolean }) {
  return (
    <ul className={cn("flex flex-col gap-px", nested && "ml-[11px] border-l pl-1.5")}>
      {nodes.map((node) => (
        <li key={node.path}>
          {node.type === "dir" ? (
            <Folder name={node.name}>
              <Level nodes={node.children} {...rows} nested />
            </Folder>
          ) : (
            <FileRow
              file={node.item}
              name={node.name}
              viewed={rows.viewed.has(node.path)}
              current={rows.current === node.path}
              onJump={rows.onJump}
              load={rows.loads.get(node.path)}
              onLoad={rows.onLoad}
            />
          )}
        </li>
      ))}
    </ul>
  );
}

function Folder({ name, children }: { name: string; children: React.ReactNode }) {
  return (
    <Collapsible defaultOpen className="group/folder">
      <CollapsibleTrigger className="flex w-full min-w-0 items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-muted-foreground hover:bg-muted hover:text-foreground">
        <ChevronRightIcon
          aria-hidden
          className="size-3.5 shrink-0 transition-transform group-data-[state=open]/folder:rotate-90"
        />
        <FolderOpenIcon aria-hidden className="hidden size-3.5 shrink-0 group-data-[state=open]/folder:block" />
        <FolderIcon aria-hidden className="size-3.5 shrink-0 group-data-[state=open]/folder:hidden" />
        <span className="truncate font-mono text-xs/normal">{name}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>{children}</CollapsibleContent>
    </Collapsible>
  );
}

function MissingAction({
  path,
  load,
  onLoad,
}: {
  path: string;
  load: MissingLoad | undefined;
  onLoad: (path: string) => void;
}) {
  if (load === "loading") return <Spinner aria-label={`Loading ${path}`} className="size-3.5 shrink-0" />;
  if (load === "too-large") return <span className="shrink-0 font-mono text-2xs">too large to show</span>;
  const failed = load === "failed";
  return (
    <Button
      variant="outline"
      size="xs"
      className="h-5 shrink-0 px-1.5 text-2xs"
      aria-label={failed ? `Load ${path} again` : `Load ${path}`}
      onClick={() => onLoad(path)}
    >
      {failed ? <RotateCcwIcon data-icon="inline-start" /> : null}
      {failed ? "Retry" : "Load"}
    </Button>
  );
}

function FileRow({
  file,
  name,
  viewed,
  current,
  onJump,
  load,
  onLoad,
}: {
  file: Listed;
  name: string;
  viewed: boolean;
  current: boolean;
  onJump: (path: string) => void;
  load: MissingLoad | undefined;
  onLoad: (path: string) => void;
}) {
  const { icon: Icon, tone, label } = CHANGE_ICONS[file.change];
  const missing = file.change === "missing";
  const body = (
    <>
      {viewed ? (
        <>
          <CheckIcon aria-hidden className="size-3.5 shrink-0 text-success" />
          <span className="sr-only">Viewed: </span>
        </>
      ) : (
        <Icon aria-hidden className={cn("size-3.5 shrink-0", tone)} />
      )}
      <span className={cn("min-w-0 flex-1 truncate", viewed && "text-muted-foreground")}>{name}</span>
      <span className="sr-only">, {label}</span>
      {missing ? (
        <MissingAction path={file.path} load={load} onLoad={onLoad} />
      ) : (
        <LineCounts added={file.added} deleted={file.deleted} binary={file.binary} className="text-2xs" />
      )}
    </>
  );
  if (missing) {
    return (
      <div title={file.path} className="flex items-center gap-1.5 px-1.5 py-1 text-muted-foreground">
        {body}
      </div>
    );
  }
  return (
    <button
      type="button"
      title={file.path}
      aria-current={current ? "location" : undefined}
      onClick={() => onJump(file.path)}
      className="flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left hover:bg-muted aria-[current=location]:bg-accent"
    >
      {body}
    </button>
  );
}
