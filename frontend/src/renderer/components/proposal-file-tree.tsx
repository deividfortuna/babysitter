import { useMemo, useState } from "react";
import {
  CheckIcon,
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
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export type Change = "new" | "deleted" | "change" | "missing";

type Listed = { path: string; added: number; deleted: number; change: Change };

export const CHANGE_ICONS: Record<Change, { icon: LucideIcon; tone: string; label: string }> = {
  new: { icon: SquarePlusIcon, tone: "text-success", label: "added" },
  deleted: { icon: SquareMinusIcon, tone: "text-destructive", label: "deleted" },
  change: { icon: SquareDotIcon, tone: "text-attention", label: "changed" },
  missing: { icon: SquareIcon, tone: "text-muted-foreground", label: "not in the diff" },
};

export function changeOf(type: string): Change {
  if (type === "new" || type === "deleted") return type;
  return "change";
}

export function LineCounts({ added, deleted, className }: { added: number; deleted: number; className?: string }) {
  return (
    <span className={cn("shrink-0 font-mono", className)}>
      <span className="text-success">+{added}</span> <span className="text-destructive">−{deleted}</span>
    </span>
  );
}

function matches(path: string, filter: string): boolean {
  return path.toLowerCase().includes(filter.trim().toLowerCase());
}

export function FileTree({
  diff,
  viewed,
  current,
  onJump,
}: {
  diff: ProposalDiff;
  viewed: ReadonlySet<string>;
  current: string | undefined;
  onJump: (path: string) => void;
}) {
  const [filter, setFilter] = useState("");
  const tree = useMemo(() => {
    const listed: Listed[] = [
      ...diff.files.map((f) => ({ path: f.path, added: f.added, deleted: f.deleted, change: changeOf(f.diff.type) })),
      ...diff.missing.map((f) => ({ path: f.path, added: f.added, deleted: f.deleted, change: "missing" as const })),
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
        <Level nodes={tree} viewed={viewed} current={current} onJump={onJump} />
      ) : (
        <p className="px-1.5 text-muted-foreground">No file matches.</p>
      )}
    </nav>
  );
}

function Level({
  nodes,
  viewed,
  current,
  onJump,
  nested = false,
}: {
  nodes: TreeNode<Listed>[];
  viewed: ReadonlySet<string>;
  current: string | undefined;
  onJump: (path: string) => void;
  nested?: boolean;
}) {
  return (
    <ul className={cn("flex flex-col gap-px", nested && "ml-[11px] border-l pl-1.5")}>
      {nodes.map((node) => (
        <li key={node.path}>
          {node.type === "dir" ? (
            <Folder name={node.name}>
              <Level nodes={node.children} viewed={viewed} current={current} onJump={onJump} nested />
            </Folder>
          ) : (
            <FileRow
              file={node.item}
              name={node.name}
              viewed={viewed.has(node.path)}
              current={current === node.path}
              onJump={onJump}
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

function FileRow({
  file,
  name,
  viewed,
  current,
  onJump,
}: {
  file: Listed;
  name: string;
  viewed: boolean;
  current: boolean;
  onJump: (path: string) => void;
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
        <span className="shrink-0 font-mono text-2xs">not in the diff</span>
      ) : (
        <LineCounts added={file.added} deleted={file.deleted} className="text-2xs" />
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
