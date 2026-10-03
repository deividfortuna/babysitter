import { useState, type KeyboardEvent } from "react";
import { MoreHorizontalIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
import { pressesPlainKey } from "@/hooks/use-plain-shortcut";
import { useRemoveRepo, useRequestSync, type Repo } from "@/hooks/useRepos";
import { Tip } from "@/components/tip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarMenuAction } from "@/components/ui/sidebar";
import { REFRESH_KEY, REMOVE_KEY } from "@/lib/shortcuts";

export function RepoActionsMenu({ repo }: { repo: Repo }) {
  const [open, setOpen] = useState(false);
  const sync = useRequestSync();
  const remove = useRemoveRepo();

  function run(action: () => void) {
    setOpen(false);
    action();
  }

  const refresh = () => run(() => sync.mutate());
  const removeRepo = () => run(() => remove.mutate(repo.id));

  function onKeyDown(event: KeyboardEvent) {
    const action = [
      { key: REFRESH_KEY, run: refresh, enabled: !sync.isPending },
      { key: REMOVE_KEY, run: removeRepo, enabled: !remove.isPending },
    ].find((candidate) => pressesPlainKey(event.nativeEvent, candidate.key));
    if (!action) return;
    event.preventDefault();
    if (action.enabled) action.run();
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Tip label="More actions">
          <SidebarMenuAction showOnHover>
            <MoreHorizontalIcon />
            <span className="sr-only">More</span>
          </SidebarMenuAction>
        </Tip>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="start" className="min-w-48" onKeyDown={onKeyDown}>
        <DropdownMenuGroup>
          <DropdownMenuItem className="text-body" disabled={sync.isPending} onClick={refresh}>
            <RefreshCwIcon />
            Sync now
            <DropdownMenuShortcut>{REFRESH_KEY.toUpperCase()}</DropdownMenuShortcut>
          </DropdownMenuItem>
          <DropdownMenuItem
            className="text-body"
            variant="destructive"
            disabled={remove.isPending}
            onClick={removeRepo}
          >
            <Trash2Icon />
            Remove
            <DropdownMenuShortcut>{REMOVE_KEY.toUpperCase()}</DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
