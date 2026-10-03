import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { AddRepoDialog } from "@/components/add-repo-dialog";
import { TitlebarNav } from "@/components/app-header";
import { AppSidebar } from "@/components/app-sidebar";
import { CommandPalette } from "@/components/command-palette";
import { DaemonDown } from "@/components/daemon-down";
import { LoadingScreen } from "@/components/loading-screen";
import { NotificationsView } from "@/components/notifications-view";
import { PairDialog } from "@/components/pair-dialog";
import { RepoView } from "@/components/repo-view";
import { SettingsDialog, type SettingsCategory } from "@/components/settings-dialog";
import { StartWatchDialog } from "@/components/start-watch-dialog";
import { StopSummaryDialog } from "@/components/stop-summary-dialog";
import { StoppedView } from "@/components/stopped-view";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { WatchDetail } from "@/components/watch-detail";
import { WatchingView } from "@/components/watching-view";
import type { PullRequest } from "@/hooks/usePulls";
import type { Watch } from "@/hooks/useWatches";
import { useDaemonStatus } from "@/hooks/useDaemonStatus";
import { useNativeNotifications } from "@/hooks/useNativeNotifications";
import { useNotificationsPresent } from "@/hooks/useNotificationsPresent";
import { useSettings } from "@/hooks/useSettings";
import { useCommandPaletteShortcut } from "@/hooks/use-command-palette-shortcut";
import { useHistoryShortcuts } from "@/hooks/use-history-shortcuts";
import { useSidebarWidth } from "@/hooks/use-sidebar-width";
import { useViewHistory } from "@/hooks/use-view-history";
import { connectEventTransport, type EventsConnection, type ReadyFrame } from "@/lib/event-transport";
import { isMac, isWindows } from "@/lib/platform";
import { presents } from "@/lib/presenting";
import { forgetDaemon } from "@/lib/forget-daemon";
import type { DiscoveredDaemon } from "../shared/connections";
import { cn } from "@/lib/utils";
import {
  TITLEBAR_HEIGHT,
  TITLEBAR_NAV_INSET,
  TITLEBAR_NAV_LEFT,
  TITLEBAR_NAV_WIDTH,
  TITLEBAR_NAV_WIDTH_WITH_MENU,
  titlebarNavClearance,
} from "../shared/titlebar";

function titlebarNavLeft(): number {
  return isMac ? TITLEBAR_NAV_LEFT : TITLEBAR_NAV_INSET;
}

function titlebarNavWidth(): number {
  return isWindows ? TITLEBAR_NAV_WIDTH_WITH_MENU : TITLEBAR_NAV_WIDTH;
}

export function App() {
  const status = useDaemonStatus();
  const queryClient = useQueryClient();
  const [, setEvents] = useState<EventsConnection>("closed");
  const [presentedFrom, setPresentedFrom] = useState<number | null>(null);
  const onConnection = useCallback((state: EventsConnection, frame?: ReadyFrame) => {
    setEvents(state);
    setPresentedFrom(state === "open" && frame ? frame.lastNotificationId : null);
  }, []);
  const ready = status.state === "ready";
  const [daemonAnswered, setDaemonAnswered] = useState(false);
  const sidebar = useSidebarWidth();
  const [resizing, setResizing] = useState(false);

  const { view, navigate, controls: history } = useViewHistory({ kind: "watching" });
  useHistoryShortcuts(history.onBack, history.onForward);
  const [startOpen, setStartOpen] = useState(false);
  const [startPull, setStartPull] = useState<PullRequest | null>(null);
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsCategory, setSettingsCategory] = useState<SettingsCategory>("appearance");
  const [stopped, setStopped] = useState<Watch | null>(null);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [pairing, setPairing] = useState<{ open: boolean; found: DiscoveredDaemon | null }>({
    open: false,
    found: null,
  });

  const shownDaemon = status.connection ? `${status.connection.id} ${status.connection.url ?? ""}` : null;
  const lastShownDaemon = useRef(shownDaemon);
  useEffect(() => {
    const previous = lastShownDaemon.current;
    lastShownDaemon.current = shownDaemon;
    if (previous === null || previous === shownDaemon) return;
    forgetDaemon(queryClient);
    navigate({ kind: "watching" });
  }, [shownDaemon, queryClient, navigate]);

  const supported = useNotificationsPresent();
  const settings = useSettings(ready);
  const present = presents(supported, settings);
  useEffect(() => {
    if (present === null) return;
    return connectEventTransport(queryClient, onConnection, { present });
  }, [queryClient, present, onConnection]);

  useNativeNotifications(ready && present === true, navigate, presentedFrom);

  const openStart = useCallback(() => {
    setStartPull(null);
    setStartOpen(true);
  }, []);
  const watchPull = useCallback((pr: PullRequest) => {
    setStartPull(pr);
    setStartOpen(true);
  }, []);
  const openAddRepo = useCallback(() => setAddRepoOpen(true), []);
  const openPair = useCallback((found?: DiscoveredDaemon) => setPairing({ open: true, found: found ?? null }), []);
  const openSettings = useCallback((category: SettingsCategory = "appearance") => {
    setSettingsCategory(category);
    setSettingsOpen(true);
  }, []);
  const firstStart = !daemonAnswered && status.state === "starting";
  const openPalette = useCallback(() => setPaletteOpen(true), []);
  useCommandPaletteShortcut(openPalette, !firstStart);

  function screen() {
    switch (view.kind) {
      case "watching":
        return <WatchingView enabled={ready} onNavigate={navigate} onWatchPR={openStart} onAddRepo={openAddRepo} />;
      case "repo":
        return (
          <RepoView key={view.name} enabled={ready} name={view.name} onNavigate={navigate} onWatchPull={watchPull} />
        );
      case "stopped":
        return <StoppedView enabled={ready} onNavigate={navigate} />;
      case "notifications":
        return <NotificationsView enabled={ready} onNavigate={navigate} />;
      case "watch":
        return (
          <WatchDetail
            id={view.id}
            enabled={ready}
            onNavigate={navigate}
            onStopped={setStopped}
            onWatchPR={openStart}
            onThisMachine={status.connection?.kind === "local"}
          />
        );
    }
  }

  if (!daemonAnswered && status.state !== "starting") setDaemonAnswered(true);
  if (firstStart) return <LoadingScreen status={status} />;

  return (
    <SidebarProvider
      style={
        {
          "--sidebar-width": `${sidebar.width}px`,
          "--titlebar-height": `${TITLEBAR_HEIGHT}px`,
          "--titlebar-nav-left": `${titlebarNavLeft()}px`,
          "--titlebar-nav-width": `${titlebarNavWidth()}px`,
          "--titlebar-nav-clearance": `${titlebarNavClearance(titlebarNavLeft(), titlebarNavWidth())}px`,
        } as CSSProperties
      }
      className={cn(
        resizing &&
          "select-none **:data-[slot=sidebar-container]:transition-none **:data-[slot=sidebar-gap]:transition-none",
      )}
    >
      <AppSidebar
        enabled={ready}
        view={view}
        onNavigate={navigate}
        onWatchPR={openStart}
        onAddRepo={openAddRepo}
        onOpenSettings={openSettings}
        status={status}
        onPair={openPair}
        width={sidebar.width}
        onResize={sidebar.setWidth}
        onResetWidth={sidebar.resetWidth}
        onResizingChange={setResizing}
      />
      <SidebarInset className="h-svh overflow-hidden">
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
          {ready ? screen() : <DaemonDown status={status} onManage={() => openSettings("connections")} />}
        </div>
      </SidebarInset>
      <TitlebarNav {...history} />

      <StartWatchDialog
        open={startOpen}
        onOpenChange={setStartOpen}
        enabled={ready}
        initial={startPull}
        onStarted={(watch) => navigate({ kind: "watch", id: watch.id })}
      />
      <AddRepoDialog open={addRepoOpen} onOpenChange={setAddRepoOpen} enabled={ready} />
      <PairDialog
        open={pairing.open}
        found={pairing.found}
        onOpenChange={(open) => setPairing((current) => ({ ...current, open }))}
      />
      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        enabled={ready}
        onNavigate={navigate}
        onWatchPR={openStart}
        onWatchPull={watchPull}
        onAddRepo={openAddRepo}
        onPair={openPair}
        onOpenSettings={openSettings}
        history={history}
      />
      <SettingsDialog category={settingsCategory} open={settingsOpen} onOpenChange={setSettingsOpen} />
      <StopSummaryDialog
        watch={stopped}
        onClose={() => setStopped(null)}
        onWatchAnother={() => {
          setStopped(null);
          setStartOpen(true);
        }}
      />
    </SidebarProvider>
  );
}
