import { type ReactNode, useMemo, useState } from "react";
import {
  ArchiveIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  BellIcon,
  CircleAlertIcon,
  EyeIcon,
  FolderGitIcon,
  GitPullRequestIcon,
  type LucideIcon,
  PanelLeftIcon,
  PlusIcon,
  RefreshCwIcon,
  ServerIcon,
  UserRoundIcon,
} from "lucide-react";
import { SETTINGS_PAGES, type SettingsCategory } from "@/components/settings-dialog";
import { Meta } from "@/components/status-badges";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command";
import { useSidebar } from "@/components/ui/sidebar";
import { historyShortcuts } from "@/hooks/use-history-shortcuts";
import type { HistoryControls } from "@/hooks/use-view-history";
import { usePulls, type PullRequest } from "@/hooks/usePulls";
import { useRepos, useRequestSync } from "@/hooks/useRepos";
import { useWatches, type Watch } from "@/hooks/useWatches";
import type { Navigate, View } from "@/lib/navigation";
import { actionValue, paletteMode } from "@/lib/palette";
import { sidebarShortcut } from "@/lib/shortcuts";
import { watchSearchText } from "@/lib/watch-filter";
import { isTakenOver, needsAttention, watchedLabels, watchLabel } from "@/lib/watch-status";

type Action = {
  id: string;
  label: string;
  Icon: LucideIcon;
  shortcut?: string;
  available?: boolean;
  run: () => void;
};

type ViewPlace = { view: View; label: string; Icon: LucideIcon };

type WatchState = { word: string; icon: ReactNode };

type Run = (fn: () => void) => void;

const VIEW_PLACES: ViewPlace[] = [
  { view: { kind: "watching" }, label: "Watching", Icon: EyeIcon },
  { view: { kind: "notifications" }, label: "Notifications", Icon: BellIcon },
  { view: { kind: "stopped" }, label: "Stopped", Icon: ArchiveIcon },
];

const PLACES_PLACEHOLDER = "Go to a watch, repository or view. Type > for actions.";
const ACTIONS_PLACEHOLDER = "Run an action";

function watchState(watch: Watch): WatchState {
  if (needsAttention(watch)) return { word: "needs you", icon: <CircleAlertIcon className="text-attention" /> };
  if (isTakenOver(watch)) return { word: "with you", icon: <UserRoundIcon /> };
  return { word: "watching", icon: <EyeIcon /> };
}

function isAvailable(action: Action): boolean {
  return action.available ?? true;
}

function PullRow({ number, title, detail, repo }: { number: number; title: string; detail: string; repo: string }) {
  return (
    <>
      <span className="min-w-0 flex-1 truncate">
        #{number} {title}
      </span>
      <Meta className="max-w-[45%] shrink-0 truncate">
        {detail} · {repo}
      </Meta>
    </>
  );
}

type PaletteHandlers = {
  enabled: boolean;
  onNavigate: Navigate;
  onWatchPR: () => void;
  onWatchPull: (pr: PullRequest) => void;
  onAddRepo: () => void;
  onPair: () => void;
  onOpenSettings: (category: SettingsCategory) => void;
  history: HistoryControls;
};

type CommandPaletteProps = PaletteHandlers & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CommandPalette({ open, onOpenChange, ...handlers }: CommandPaletteProps) {
  function run(fn: () => void) {
    onOpenChange(false);
    fn();
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Command palette"
      description={PLACES_PLACEHOLDER}
      showCloseButton={false}
      className="top-[15%] w-(--size-dialog-medium) translate-y-0 sm:max-w-(--size-dialog-max)"
    >
      <PaletteContent {...handlers} run={run} />
    </CommandDialog>
  );
}

function PaletteContent({ enabled, onNavigate, onWatchPull, run, ...actions }: PaletteHandlers & { run: Run }) {
  const [query, setQuery] = useState("");
  const places = paletteMode(query) === "places";

  return (
    <>
      <CommandInput
        placeholder={places ? PLACES_PLACEHOLDER : ACTIONS_PLACEHOLDER}
        value={query}
        onValueChange={setQuery}
      />
      <CommandList className="max-h-[min(28rem,60vh)]">
        <CommandEmpty className="px-3 py-6 text-center text-sm text-muted-foreground">
          {places ? "Nothing matches. Type > for actions." : "No action matches."}
        </CommandEmpty>
        {places ? (
          <Places enabled={enabled} onNavigate={onNavigate} onWatchPull={onWatchPull} run={run} />
        ) : (
          <Actions enabled={enabled} run={run} {...actions} />
        )}
      </CommandList>
    </>
  );
}

type PlacesProps = {
  enabled: boolean;
  onNavigate: Navigate;
  onWatchPull: (pr: PullRequest) => void;
  run: Run;
};

function Places({ enabled, onNavigate, onWatchPull, run }: PlacesProps) {
  const watches = useWatches(enabled);
  const repos = useRepos(enabled);
  const pulls = usePulls(enabled);

  const active = watches.data ?? [];
  const unwatched = useMemo(() => {
    const watched = watchedLabels(watches.data ?? []);
    return (pulls.data ?? []).filter((pr) => !watched.has(watchLabel(pr)));
  }, [watches.data, pulls.data]);

  return (
    <>
      {active.length > 0 ? (
        <CommandGroup heading="Watches">
          {active.map((watch) => {
            const state = watchState(watch);
            return (
              <CommandItem
                key={watch.id}
                value={watchSearchText(watch)}
                onSelect={() => run(() => onNavigate({ kind: "watch", id: watch.id }))}
              >
                {state.icon}
                <PullRow number={watch.number} title={watch.title} detail={state.word} repo={watch.repo} />
              </CommandItem>
            );
          })}
        </CommandGroup>
      ) : null}

      {repos.data && repos.data.length > 0 ? (
        <CommandGroup heading="Repositories">
          {repos.data.map((repo) => (
            <CommandItem
              key={repo.id}
              value={repo.fullName}
              onSelect={() => run(() => onNavigate({ kind: "repo", name: repo.fullName }))}
            >
              {repo.lastError ? <CircleAlertIcon className="text-destructive" /> : <FolderGitIcon />}
              <span className="truncate">{repo.fullName}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      ) : null}

      {unwatched.length > 0 ? (
        <CommandGroup heading="Open pull requests">
          {unwatched.map((pr) => (
            <CommandItem key={watchLabel(pr)} value={watchSearchText(pr)} onSelect={() => run(() => onWatchPull(pr))}>
              <GitPullRequestIcon />
              <PullRow number={pr.number} title={pr.title} detail={pr.author} repo={pr.repo} />
            </CommandItem>
          ))}
        </CommandGroup>
      ) : null}

      <CommandGroup heading="Views">
        {VIEW_PLACES.map(({ view, label, Icon }) => (
          <CommandItem key={view.kind} value={label} onSelect={() => run(() => onNavigate(view))}>
            <Icon />
            <span>{label}</span>
          </CommandItem>
        ))}
      </CommandGroup>
    </>
  );
}

type ActionsProps = {
  enabled: boolean;
  onWatchPR: () => void;
  onAddRepo: () => void;
  onPair: () => void;
  onOpenSettings: (category: SettingsCategory) => void;
  history: HistoryControls;
  run: Run;
};

function Actions({ enabled, onWatchPR, onAddRepo, onPair, onOpenSettings, history, run }: ActionsProps) {
  const { toggleSidebar } = useSidebar();
  const requestSync = useRequestSync();
  const moves = historyShortcuts();

  const general: Action[] = [
    { id: "watch", label: "Watch a pull request", Icon: PlusIcon, available: enabled, run: onWatchPR },
    { id: "add-repo", label: "Add a repository", Icon: FolderGitIcon, available: enabled, run: onAddRepo },
    {
      id: "sync",
      label: "Sync now",
      Icon: RefreshCwIcon,
      available: enabled,
      run: () => requestSync.mutate(),
    },
    { id: "pair", label: "Connect to a remote daemon", Icon: ServerIcon, run: onPair },
    {
      id: "back",
      label: "Go back",
      Icon: ArrowLeftIcon,
      shortcut: moves.back.label,
      available: history.canGoBack,
      run: history.onBack,
    },
    {
      id: "forward",
      label: "Go forward",
      Icon: ArrowRightIcon,
      shortcut: moves.forward.label,
      available: history.canGoForward,
      run: history.onForward,
    },
    { id: "sidebar", label: "Toggle sidebar", Icon: PanelLeftIcon, shortcut: sidebarShortcut(), run: toggleSidebar },
  ];

  const settings: Action[] = SETTINGS_PAGES.map((page) => ({
    id: `settings-${page.id}`,
    label: `Settings: ${page.label}`,
    Icon: page.Icon,
    run: () => onOpenSettings(page.id),
  }));

  return (
    <>
      <ActionGroup heading="Actions" actions={general} run={run} />
      <ActionGroup heading="Settings" actions={settings} run={run} />
    </>
  );
}

function ActionGroup({ heading, actions, run }: { heading: string; actions: Action[]; run: Run }) {
  return (
    <CommandGroup heading={heading}>
      {actions.filter(isAvailable).map((action) => (
        <CommandItem key={action.id} value={actionValue(action.label)} onSelect={() => run(action.run)}>
          <action.Icon />
          <span>{action.label}</span>
          {action.shortcut ? <CommandShortcut>{action.shortcut}</CommandShortcut> : null}
        </CommandItem>
      ))}
    </CommandGroup>
  );
}
