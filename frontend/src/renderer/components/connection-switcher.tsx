import { CheckIcon, ChevronsUpDownIcon, LaptopIcon, PlusIcon, RadarIcon, ServerIcon, SettingsIcon } from "lucide-react";
import { LOCAL_CONNECTION_ID, hostOf, type DiscoveredDaemon } from "../../shared/connections";
import type { DaemonStatus } from "../../shared/daemon-status";
import { useConnections, useUnpairedDaemons, useUseConnection } from "@/hooks/useConnections";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar";
import { VersionWarning } from "@/components/version-warning";
import { cn } from "@/lib/utils";

type Props = {
  status: DaemonStatus;
  onPair: (found?: DiscoveredDaemon) => void;
  onManage: () => void;
};

function statusDot(state: DaemonStatus["state"]): string {
  if (state === "ready") return "bg-success";
  if (state === "error") return "bg-destructive";
  return "bg-muted-foreground animate-pulse motion-reduce:animate-none";
}

function statusWord(status: DaemonStatus): string {
  const remote = status.connection?.kind === "remote";
  if (status.state === "ready") return remote ? "Remote daemon" : "Local daemon";
  if (status.state === "error") return remote ? "Unreachable" : "Stopped";
  return "Connecting…";
}

export function ConnectionSwitcher({ status, onPair, onManage }: Props) {
  const list = useConnections();
  const use = useUseConnection();
  const found = useUnpairedDaemons(true, list.remotes);
  const remote = status.connection?.kind === "remote";
  const name = status.connection?.name ?? list.localName;
  const Icon = remote ? ServerIcon : LaptopIcon;
  const remoteVersion = remote ? status.connection?.version : undefined;

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" aria-label={`Daemon: ${name}`} className="data-[state=open]:bg-sidebar-accent">
              <span className="relative flex size-8 shrink-0 items-center justify-center rounded-md border bg-background">
                <Icon className="size-4" />
                <span
                  aria-hidden
                  className={cn(
                    "absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-sidebar",
                    statusDot(status.state),
                  )}
                />
              </span>
              <span className="flex min-w-0 flex-1 flex-col gap-0.5 text-left leading-tight">
                <span className="truncate text-sm font-medium">{name}</span>
                <span className="truncate text-2xs text-muted-foreground">{statusWord(status)}</span>
              </span>
              <VersionWarning name={name} version={remoteVersion} />
              {found.length > 0 ? (
                <span className="rounded-full bg-attention/15 px-1.5 font-mono text-2xs text-attention">
                  {found.length} new
                </span>
              ) : null}
              <ChevronsUpDownIcon className="ml-auto size-4 text-muted-foreground" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-(--radix-dropdown-menu-trigger-width) min-w-64">
            <DropdownMenuLabel className="eyebrow">Show the watches of</DropdownMenuLabel>
            <DropdownMenuGroup>
              <DropdownMenuItem onClick={() => use.mutate(LOCAL_CONNECTION_ID)}>
                <LaptopIcon />
                <span className="flex-1">{list.localName}</span>
                {list.activeId === LOCAL_CONNECTION_ID ? <CheckIcon /> : null}
              </DropdownMenuItem>
              {list.remotes.map((saved) => (
                <DropdownMenuItem key={saved.id} onClick={() => use.mutate(saved.id)}>
                  <ServerIcon />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate">{saved.name}</span>
                    <span className="truncate font-mono text-2xs text-muted-foreground">{hostOf(saved.url)}</span>
                  </span>
                  {list.activeId === saved.id ? (
                    <>
                      <VersionWarning name={saved.name} version={remoteVersion} />
                      <CheckIcon />
                    </>
                  ) : null}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            {found.length > 0 ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel className="eyebrow">Found on your network</DropdownMenuLabel>
                <DropdownMenuGroup>
                  {found.map((daemon) => (
                    <DropdownMenuItem key={daemon.url} onClick={() => onPair(daemon)}>
                      <RadarIcon className="text-attention" />
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="truncate">{daemon.name}</span>
                        <span className="truncate font-mono text-2xs text-muted-foreground">
                          {daemon.address}:{daemon.port}
                        </span>
                      </span>
                      <VersionWarning name={daemon.name} version={daemon.version} />
                      <span className="text-2xs text-muted-foreground">Pair</span>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
              </>
            ) : null}
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem onClick={() => onPair()}>
                <PlusIcon />
                Connect to a remote daemon…
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onManage}>
                <SettingsIcon />
                Manage connections
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
