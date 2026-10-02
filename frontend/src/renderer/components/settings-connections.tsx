import { useState } from "react";
import { LaptopIcon, PlusIcon, RadarIcon, RefreshCwIcon, ServerIcon, Trash2Icon } from "lucide-react";
import { LOCAL_CONNECTION_ID, hostOf, type DiscoveredDaemon } from "../../shared/connections";
import {
  useConnections,
  useDiscoveredDaemons,
  useRemoveConnection,
  useUnpairedDaemons,
  useUseConnection,
} from "@/hooks/useConnections";
import { useDaemonStatus } from "@/hooks/useDaemonStatus";
import { PairDialog } from "@/components/pair-dialog";
import { VersionWarning } from "@/components/version-warning";
import { SettingsCard, SettingsRow, SettingsSection } from "@/components/settings-page";
import { Tip } from "@/components/tip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

function RowIcon({ remote }: { remote: boolean }) {
  const Icon = remote ? ServerIcon : LaptopIcon;
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background">
      <Icon className="size-4" />
    </span>
  );
}

function ActiveOrUse({ active, onUse, busy }: { active: boolean; onUse: () => void; busy: boolean }) {
  if (active) return <Badge variant="secondary">Shown now</Badge>;
  return (
    <Button variant="outline" size="sm" disabled={busy} onClick={onUse}>
      Show
    </Button>
  );
}

export function ConnectionsPanel() {
  const list = useConnections();
  const use = useUseConnection();
  const remove = useRemoveConnection();
  const discovery = useDiscoveredDaemons(true);
  const found = useUnpairedDaemons(true, list.remotes);
  const shownVersion = useDaemonStatus().connection?.version;
  const [pairing, setPairing] = useState<{ open: boolean; found: DiscoveredDaemon | null }>({
    open: false,
    found: null,
  });

  const openPair = (daemon: DiscoveredDaemon | null) => setPairing({ open: true, found: daemon });

  return (
    <div className="flex flex-col gap-4.5">
      <SettingsSection label="Daemons">
        <SettingsCard>
          <SettingsRow
            icon={<RowIcon remote={false} />}
            label={list.localName}
            description="The app starts a daemon here and stops it when you quit."
          >
            <ActiveOrUse
              active={list.activeId === LOCAL_CONNECTION_ID}
              busy={use.isPending}
              onUse={() => use.mutate(LOCAL_CONNECTION_ID)}
            />
          </SettingsRow>
          {list.remotes.map((remote) => (
            <SettingsRow
              key={remote.id}
              icon={<RowIcon remote />}
              label={remote.name}
              description={<span className="font-mono text-2xs">{hostOf(remote.url)}</span>}
            >
              {list.activeId === remote.id ? (
                <VersionWarning name={remote.name} version={shownVersion} focusable />
              ) : null}
              <ActiveOrUse
                active={list.activeId === remote.id}
                busy={use.isPending}
                onUse={() => use.mutate(remote.id)}
              />
              <Tip label="Forget this daemon">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Forget ${remote.name}`}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(remote.id)}
                >
                  <Trash2Icon />
                </Button>
              </Tip>
            </SettingsRow>
          ))}
        </SettingsCard>
        <div>
          <Button variant="outline" size="sm" className="border-dashed" onClick={() => openPair(null)}>
            <PlusIcon data-icon="inline-start" />
            Connect to a remote daemon
          </Button>
        </div>
      </SettingsSection>

      <SettingsSection label="Found on your network">
        <SettingsCard>
          {found.map((daemon) => (
            <SettingsRow
              key={daemon.url}
              icon={<RadarIcon className="size-5 text-attention" />}
              label={daemon.name}
              description={
                <span className="font-mono text-2xs">
                  {daemon.address}:{daemon.port}
                  {daemon.version ? ` · ${daemon.version}` : ""}
                </span>
              }
            >
              <VersionWarning name={daemon.name} version={daemon.version} focusable />
              <Button size="sm" onClick={() => openPair(daemon)}>
                Pair
              </Button>
            </SettingsRow>
          ))}
          <SettingsRow
            label={found.length > 0 ? "Looking for more" : "No new daemon found"}
            description={
              <>
                A daemon started with <code className="font-mono text-2xs">babysitter daemon start --remote :7420</code>{" "}
                announces itself here.
              </>
            }
          >
            <Button variant="ghost" size="sm" disabled={discovery.isFetching} onClick={() => void discovery.refetch()}>
              {discovery.isFetching ? <Spinner data-icon="inline-start" /> : <RefreshCwIcon data-icon="inline-start" />}
              Look again
            </Button>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <PairDialog
        open={pairing.open}
        found={pairing.found}
        onOpenChange={(open) => setPairing((current) => ({ ...current, open }))}
      />
    </div>
  );
}
