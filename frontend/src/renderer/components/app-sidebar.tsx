import { type ComponentProps, useMemo, useState } from "react";
import {
  ArchiveIcon,
  BellIcon,
  CircleAlertIcon,
  EyeIcon,
  FolderGitIcon,
  MessageCircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
  RefreshCwIcon,
  SettingsIcon,
  Trash2Icon,
  UserRoundIcon,
} from "lucide-react";
import { useNotifications } from "@/hooks/useNotifications";
import { usePulls } from "@/hooks/usePulls";
import { useRemoveRepo, useRepos, useRequestSync } from "@/hooks/useRepos";
import { useViewer, type Viewer } from "@/hooks/useViewer";
import { useWatches } from "@/hooks/useWatches";
import { RateLimitCard } from "@/components/rate-limit-card";
import { UpdateCard } from "@/components/update-card";
import type { SettingsCategory } from "@/components/settings-dialog";
import { SidebarResizeHandle } from "@/components/sidebar-resize-handle";
import { initials } from "@/lib/initials";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
} from "@/components/ui/sidebar";
import type { Navigate, View } from "@/lib/navigation";
import { needsAttention } from "@/lib/watch-status";
import { cn } from "@/lib/utils";

const SKELETON_ROWS = 3;

const issuesURL = "https://github.com/deividfortuna/babysitter/issues";

function unreadTitle(count: number): string {
  return `${count} unread ${count === 1 ? "notification" : "notifications"}`;
}

function watchedTitle(count: number, needsYou: boolean): string {
  const noun = count === 1 ? "watched pull request" : "watched pull requests";
  if (!needsYou) return `${count} ${noun}`;
  return `${count} ${noun} ${count === 1 ? "needs" : "need"} you`;
}

const attentionFill =
  "bg-attention text-attention-foreground peer-hover/menu-button:text-attention-foreground peer-data-[active=true]/menu-button:text-attention-foreground";

function WatchingBadge({ active, attention }: { active: number; attention: number }) {
  if (active === 0) return null;
  const needsYou = attention > 0;
  const count = needsYou ? attention : active;
  return (
    <SidebarMenuBadge title={watchedTitle(count, needsYou)} className={cn("font-mono", attentionFill)}>
      {count}
    </SidebarMenuBadge>
  );
}

type AppSidebarProps = ComponentProps<typeof Sidebar> & {
  enabled: boolean;
  view: View;
  onNavigate: Navigate;
  onWatchPR: () => void;
  onAddRepo: () => void;
  onOpenSettings: (category?: SettingsCategory) => void;
  width: number;
  onResize: (width: number) => void;
  onResetWidth: () => void;
  onResizingChange?: (resizing: boolean) => void;
};

export function AppSidebar({
  enabled,
  view,
  onNavigate,
  onWatchPR,
  onAddRepo,
  onOpenSettings,
  width,
  onResize,
  onResetWidth,
  onResizingChange,
  ...props
}: AppSidebarProps) {
  const watches = useWatches(enabled, "all");
  const repos = useRepos(enabled);
  const pulls = usePulls(enabled);
  const notifications = useNotifications(enabled);
  const unreadCount = notifications.data?.unreadCount ?? 0;
  const removeRepo = useRemoveRepo();
  const requestSync = useRequestSync();
  const viewer = useViewer(enabled);

  const activeCount = watches.data?.filter((w) => w.status === "active").length ?? 0;
  const stoppedCount = watches.data?.filter((w) => w.status === "stopped").length ?? 0;
  const attentionCount = watches.data?.filter(needsAttention).length ?? 0;

  const pullCount = useMemo(() => {
    const counts = new Map<string, number>();
    for (const pr of pulls.data ?? []) counts.set(pr.repo, (counts.get(pr.repo) ?? 0) + 1);
    return counts;
  }, [pulls.data]);

  return (
    <Sidebar {...props}>
      <SidebarHeader className="h-titlebar" />

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Watching"
                  isActive={view.kind === "watching" || view.kind === "watch"}
                  onClick={() => onNavigate({ kind: "watching" })}
                >
                  <EyeIcon />
                  <span>Watching</span>
                </SidebarMenuButton>
                <WatchingBadge active={activeCount} attention={attentionCount} />
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Notifications"
                  isActive={view.kind === "notifications"}
                  onClick={() => onNavigate({ kind: "notifications" })}
                >
                  <BellIcon />
                  <span>Notifications</span>
                </SidebarMenuButton>
                {unreadCount > 0 ? (
                  <SidebarMenuBadge title={unreadTitle(unreadCount)} className={cn("font-mono", attentionFill)}>
                    {unreadCount}
                  </SidebarMenuBadge>
                ) : null}
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Stopped"
                  isActive={view.kind === "stopped"}
                  onClick={() => onNavigate({ kind: "stopped" })}
                >
                  <ArchiveIcon />
                  <span>Stopped</span>
                </SidebarMenuButton>
                {stoppedCount > 0 ? (
                  <SidebarMenuBadge className="font-mono text-muted-foreground">{stoppedCount}</SidebarMenuBadge>
                ) : null}
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>Repositories</SidebarGroupLabel>
          <SidebarGroupAction title="Add a repository" disabled={!enabled} onClick={onAddRepo}>
            <PlusIcon />
            <span className="sr-only">Add a repository</span>
          </SidebarGroupAction>
          <SidebarGroupContent>
            <SidebarMenu>
              {repos.isPending && enabled
                ? Array.from({ length: SKELETON_ROWS }, (_, index) => (
                    <SidebarMenuItem key={index}>
                      <SidebarMenuSkeleton showIcon />
                    </SidebarMenuItem>
                  ))
                : null}

              {repos.data?.map((repo) => {
                const count = pullCount.get(repo.fullName) ?? 0;
                return (
                  <SidebarMenuItem key={repo.id}>
                    <SidebarMenuButton
                      tooltip={repo.lastError ? `${repo.fullName}: ${repo.lastError}` : repo.fullName}
                      isActive={view.kind === "repo" && view.name === repo.fullName}
                      onClick={() => onNavigate({ kind: "repo", name: repo.fullName })}
                    >
                      {repo.lastError ? <CircleAlertIcon className="text-destructive" /> : <FolderGitIcon />}
                      <span className="truncate">{repo.fullName}</span>
                      {count > 0 ? (
                        <span className="ml-auto font-mono text-2xs text-muted-foreground">{count}</span>
                      ) : null}
                    </SidebarMenuButton>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <SidebarMenuAction showOnHover>
                          <MoreHorizontalIcon />
                          <span className="sr-only">More</span>
                        </SidebarMenuAction>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent side="right" align="start">
                        <DropdownMenuGroup>
                          <DropdownMenuItem disabled={requestSync.isPending} onClick={() => requestSync.mutate()}>
                            <RefreshCwIcon />
                            Sync now
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            variant="destructive"
                            disabled={removeRepo.isPending}
                            onClick={() => removeRepo.mutate(repo.id)}
                          >
                            <Trash2Icon />
                            Remove
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </SidebarMenuItem>
                );
              })}

              {repos.data && repos.data.length === 0 ? (
                <SidebarMenuItem>
                  <SidebarMenuButton onClick={onAddRepo}>
                    <PlusIcon />
                    <span>Add a repository</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ) : null}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="relative gap-2 bg-sidebar">
        <div
          aria-hidden
          className="pointer-events-none absolute inset-x-0 -top-10 h-10 bg-linear-to-b from-transparent to-sidebar"
        />
        <Button variant="outline" size="sm" className="w-full border-dashed" disabled={!enabled} onClick={onWatchPR}>
          <PlusIcon data-icon="inline-start" />
          Watch a pull request
        </Button>
        <RateLimitCard enabled={enabled} onPollLessOften={() => onOpenSettings("polling")} />
        <UpdateCard />
        <AccountRow viewer={viewer.data} onOpenSettings={() => onOpenSettings()} />
      </SidebarFooter>
      <SidebarResizeHandle
        width={width}
        onResize={onResize}
        onReset={onResetWidth}
        onResizingChange={onResizingChange}
      />
    </Sidebar>
  );
}

function Avatar({ viewer }: { viewer?: Viewer }) {
  const [broken, setBroken] = useState(false);
  const label = viewer ? initials(viewer.name?.trim() || viewer.login) : null;
  return (
    <span className="flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-full bg-accent text-3xs font-semibold text-muted-foreground">
      {viewer?.avatarUrl && !broken ? (
        <img src={viewer.avatarUrl} alt="" className="size-full object-cover" onError={() => setBroken(true)} />
      ) : label ? (
        label
      ) : (
        <UserRoundIcon className="size-3.5" />
      )}
    </span>
  );
}

type AccountRowProps = {
  viewer?: Viewer;
  onOpenSettings: () => void;
};

function AccountRow({ viewer, onOpenSettings }: AccountRowProps) {
  const name = viewer?.name?.trim() || viewer?.login || "No account";
  return (
    <div className="flex items-center gap-2.5 px-0.5 pt-1">
      <Avatar viewer={viewer} />
      <span
        className={cn("min-w-0 flex-1 truncate text-sm", !viewer && "text-muted-foreground")}
        title={viewer?.login ? `@${viewer.login}` : undefined}
      >
        {name}
      </span>
      <Button asChild variant="ghost" size="icon-sm" className="text-muted-foreground">
        <a href={issuesURL} target="_blank" rel="noreferrer" aria-label="Report an issue">
          <MessageCircleIcon />
        </a>
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        className="text-muted-foreground"
        aria-label="Settings"
        onClick={onOpenSettings}
      >
        <SettingsIcon />
      </Button>
    </div>
  );
}
