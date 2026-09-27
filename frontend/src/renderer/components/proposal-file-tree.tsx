import { useMemo } from "react";
import {
  CheckIcon,
  ChevronRightIcon,
  FileDiffIcon,
  FileIcon,
  FileMinusIcon,
  FilePlusIcon,
  FolderIcon,
  FolderOpenIcon,
  type LucideIcon,
} from "lucide-react";
import { fileTree, type TreeNode } from "@/lib/file-tree";
import type { ProposalDiff } from "@/lib/proposal-diff";
import { Meta } from "@/components/status-badges";
import { count } from "@/components/proposal-dialogs";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";

type Change = "new" | "deleted" | "change" | "missing";

type Listed = { path: string; added: number; deleted: number; change: Change };

const ICONS: Record<Change, { icon: LucideIcon; tone: string; label: string }> = {
  new: { icon: FilePlusIcon, tone: "text-success", label: "added" },
  deleted: { icon: FileMinusIcon, tone: "text-destructive", label: "deleted" },
  change: { icon: FileDiffIcon, tone: "text-attention", label: "changed" },
  missing: { icon: FileIcon, tone: "text-muted-foreground", label: "not in the diff" },
};

function changeOf(type: string): Change {
  if (type === "new" || type === "deleted") return type;
  return "change";
}

export function FileTree({
  diff,
  viewed,
  onJump,
}: {
  diff: ProposalDiff;
  viewed: ReadonlySet<string>;
  onJump: (path: string) => void;
}) {
  const tree = useMemo(() => {
    const listed: Listed[] = [
      ...diff.files.map((f) => ({ path: f.path, added: f.added, deleted: f.deleted, change: changeOf(f.diff.type) })),
      ...diff.missing.map((f) => ({ path: f.path, added: f.added, deleted: f.deleted, change: "missing" as const })),
    ];
    return fileTree(listed, (f) => f.path);
  }, [diff]);
  const total = diff.files.length + diff.missing.length;
  const seen = diff.files.filter((f) => viewed.has(f.path)).length;
  return (
    <nav aria-label="Changed files" className="flex flex-col gap-1">
      <span className="eyebrow">
        {count(total, "changed file")}
        {seen > 0 ? ` · ${seen} viewed` : ""}
      </span>
      <Level nodes={tree} viewed={viewed} onJump={onJump} />
    </nav>
  );
}

function Level({
  nodes,
  viewed,
  onJump,
  nested = false,
}: {
  nodes: TreeNode<Listed>[];
  viewed: ReadonlySet<string>;
  onJump: (path: string) => void;
  nested?: boolean;
}) {
  return (
    <ul className={cn("flex flex-col gap-px", nested && "ml-[11px] border-l pl-1.5")}>
      {nodes.map((node) => (
        <li key={node.path}>
          {node.type === "dir" ? (
            <Folder name={node.name}>
              <Level nodes={node.children} viewed={viewed} onJump={onJump} nested />
            </Folder>
          ) : (
            <FileRow file={node.item} name={node.name} viewed={viewed.has(node.path)} onJump={onJump} />
          )}
        </li>
      ))}
    </ul>
  );
}

function Folder({ name, children }: { name: string; children: React.ReactNode }) {
  return (
    <Collapsible defaultOpen className="group/folder">
      <CollapsibleTrigger className="flex w-full min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-muted-foreground hover:bg-muted hover:text-foreground">
        <ChevronRightIcon
          aria-hidden
          className="size-3 shrink-0 transition-transform group-data-[state=open]/folder:rotate-90"
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
  onJump,
}: {
  file: Listed;
  name: string;
  viewed: boolean;
  onJump: (path: string) => void;
}) {
  const { icon: Icon, tone, label } = ICONS[file.change];
  const body = (
    <>
      <span className="flex min-w-0 items-center gap-1.5">
        {viewed ? (
          <>
            <CheckIcon aria-hidden className="size-3.5 shrink-0 text-success" />
            <span className="sr-only">Viewed: </span>
          </>
        ) : (
          <Icon aria-hidden className={cn("size-3.5 shrink-0", tone)} />
        )}
        <span className={cn("truncate font-mono text-xs/normal", viewed && "text-muted-foreground")}>{name}</span>
        <span className="sr-only">, {label}</span>
      </span>
      <Meta className="shrink-0">
        {file.change === "missing" ? "not in the diff" : `+${file.added} −${file.deleted}`}
      </Meta>
    </>
  );
  if (file.change === "missing") {
    return (
      <div title={file.path} className="flex items-center justify-between gap-2 px-1 py-0.5 text-muted-foreground">
        {body}
      </div>
    );
  }
  return (
    <button
      type="button"
      title={file.path}
      onClick={() => onJump(file.path)}
      className="flex w-full items-center justify-between gap-2 rounded-md px-1 py-0.5 text-left hover:bg-muted"
    >
      {body}
    </button>
  );
}
