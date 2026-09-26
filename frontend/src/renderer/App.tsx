import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { AddRepoDialog } from "@/components/add-repo-dialog";
import { AppHeader, TitlebarSidebarTrigger } from "@/components/app-header";
import { AppSidebar } from "@/components/app-sidebar";
import { DaemonDown } from "@/components/daemon-down";
import { LoadingScreen } from "@/components/loading-screen";
import { NotificationsView } from "@/components/notifications-view";
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
import { useSidebarWidth } from "@/hooks/use-sidebar-width";
import { connectEventTransport, type EventsConnection, type ReadyFrame } from "@/lib/event-transport";
import { sameView, type View } from "@/lib/navigation";
import { presents } from "@/lib/presenting";
import { cn } from "@/lib/utils";
import { TITLEBAR_HEIGHT } from "../shared/titlebar";

const HISTORY_LIMIT = 20;

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

  const [view, setView] = useState<View>({ kind: "watching" });
  const history = useRef<View[]>([]);
  const [startOpen, setStartOpen] = useState(false);
  const [startPull, setStartPull] = useState<PullRequest | null>(null);
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsCategory, setSettingsCategory] = useState<SettingsCategory>("general");
  const [stopped, setStopped] = useState<Watch | null>(null);

  const supported = useNotificationsPresent();
  const settings = useSettings(ready);
  const present = presents(supported, settings);
  useEffect(() => {
    if (present === null) return;
    return connectEventTransport(queryClient, onConnection, { present });
  }, [queryClient, present, onConnection]);

  const navigate = useCallback((next: View) => {
    setView((current) => {
      if (sameView(current, next)) return current;
      history.current = [...history.current, current].slice(-HISTORY_LIMIT);
      return next;
    });
  }, []);

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
  const openSettings = useCallback((category: SettingsCategory = "general") => {
    setSettingsCategory(category);
    setSettingsOpen(true);
  }, []);

  function screen() {
    switch (view.kind) {
      case "watching":
        return (
          <WatchingView
            enabled={ready}
            repo={view.repo}
            onNavigate={navigate}
            onWatchPR={openStart}
            onAddRepo={openAddRepo}
          />
        );
      case "repo":
        return (
          <RepoView
            key={view.name}
            enabled={ready}
            name={view.name}
            onNavigate={navigate}
            onWatchPR={openStart}
            onWatchPull={watchPull}
          />
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
          />
        );
    }
  }

  if (!daemonAnswered && status.state !== "starting") setDaemonAnswered(true);
  const firstStart = !daemonAnswered && status.state === "starting";
  if (firstStart) return <LoadingScreen status={status} />;

  return (
    <SidebarProvider
      style={{ "--sidebar-width": `${sidebar.width}px`, "--titlebar-height": `${TITLEBAR_HEIGHT}px` } as CSSProperties}
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
        width={sidebar.width}
        onResize={sidebar.setWidth}
        onResetWidth={sidebar.resetWidth}
        onResizingChange={setResizing}
      />
      <SidebarInset className="h-svh overflow-hidden">
        <AppHeader />
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
          {ready ? screen() : <DaemonDown status={status} />}
        </div>
      </SidebarInset>
      <TitlebarSidebarTrigger />

      <StartWatchDialog
        open={startOpen}
        onOpenChange={setStartOpen}
        enabled={ready}
        initial={startPull}
        onStarted={(watch) => navigate({ kind: "watch", id: watch.id })}
      />
      <AddRepoDialog open={addRepoOpen} onOpenChange={setAddRepoOpen} enabled={ready} />
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
