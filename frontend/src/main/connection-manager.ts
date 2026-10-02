import { randomUUID } from "node:crypto";
import {
  apiBase,
  isLocal,
  LOCAL_CONNECTION_ID,
  parsePairingLink,
  type ConnectionList,
  type Connections,
  type DiscoveredDaemon,
  type PairRequest,
  type PairResult,
  type RemoteConnection,
} from "../shared/connections";
import type { DaemonConnection, DaemonStatus } from "../shared/daemon-status";
import type { RemoteCheck } from "./remote-check";

const PROBE_MS = 15_000;

export type LocalDaemon = {
  getStatus(): DaemonStatus;
  onStatus(listener: (status: DaemonStatus) => void): () => void;
  start(): Promise<void>;
  restart(): Promise<void>;
  stop(): Promise<void>;
  stopAnyOwner(): Promise<void>;
};

export type ConnectionManagerOptions = {
  local: LocalDaemon;
  localName: string;
  read: () => Connections;
  write: (connections: Connections) => void;
  check: (url: string, token: string) => Promise<RemoteCheck>;
  discover: () => Promise<DiscoveredDaemon[]>;
  log?: (msg: string) => void;
  probeMs?: number;
};

export function localName(platform: NodeJS.Platform): string {
  if (platform === "darwin") return "This Mac";
  if (platform === "win32") return "This PC";
  return "This computer";
}

export class ConnectionManager {
  private connections: Connections;
  private remoteStatus: DaemonStatus = { state: "starting" };
  private readonly statusListeners = new Set<(status: DaemonStatus) => void>();
  private readonly listListeners = new Set<(list: ConnectionList) => void>();
  private probe: ReturnType<typeof setInterval> | null = null;
  private generation = 0;
  private changes: Promise<void> = Promise.resolve();
  private readonly log: (msg: string) => void;

  constructor(private readonly opts: ConnectionManagerOptions) {
    this.connections = opts.read();
    this.log = opts.log ?? (() => undefined);
    opts.local.onStatus(() => {
      if (this.localActive) this.emitStatus();
    });
  }

  private get localActive(): boolean {
    return isLocal(this.connections.activeId);
  }

  private get activeRemote(): RemoteConnection | undefined {
    return this.connections.remotes.find((remote) => remote.id === this.connections.activeId);
  }

  start(): Promise<void> {
    return this.inTurn(async () => {
      if (this.localActive) return this.opts.local.start();
      await this.opts.local.stopAnyOwner();
      return this.connectRemote();
    });
  }

  retry(): Promise<void> {
    return this.inTurn(async () => {
      if (this.localActive) return this.opts.local.restart();
      return this.connectRemote();
    });
  }

  getStatus(): DaemonStatus {
    if (this.localActive) return { ...this.opts.local.getStatus(), connection: this.localConnection() };
    return this.remoteStatus;
  }

  onStatus(listener: (status: DaemonStatus) => void): () => void {
    this.statusListeners.add(listener);
    return () => this.statusListeners.delete(listener);
  }

  list(): ConnectionList {
    return {
      activeId: this.connections.activeId,
      localName: this.opts.localName,
      remotes: this.connections.remotes.map(({ id, name, url }) => ({ id, name, url })),
    };
  }

  onList(listener: (list: ConnectionList) => void): () => void {
    this.listListeners.add(listener);
    return () => this.listListeners.delete(listener);
  }

  discover(): Promise<DiscoveredDaemon[]> {
    return this.opts.discover();
  }

  use(id: string): Promise<void> {
    return this.inTurn(() => this.switchTo(id));
  }

  private inTurn(change: () => Promise<void>): Promise<void> {
    const turn = this.changes.then(change);
    this.changes = turn.catch(() => undefined);
    return turn;
  }

  private async switchTo(id: string): Promise<void> {
    if (id === this.connections.activeId) return;
    const known = isLocal(id) || this.connections.remotes.some((remote) => remote.id === id);
    if (!known) return;
    this.save({ ...this.connections, activeId: id });
    this.log(`connections: now showing ${this.describe(id)}`);
    if (isLocal(id)) {
      this.stopProbe();
      this.generation++;
      this.emitStatus();
      return this.opts.local.start();
    }
    await this.opts.local.stopAnyOwner();
    return this.connectRemote();
  }

  async pair(request: PairRequest): Promise<PairResult> {
    const target = parsePairingLink(request.link, request.token);
    if (!target) {
      return { ok: false, error: "Paste a link that babysitter daemon pair printed, or an address and its token." };
    }
    const check = await this.opts.check(target.url, target.token);
    if (!check.ok) return check;
    const existing = this.connections.remotes.find((remote) => remote.url === target.url);
    const connection: RemoteConnection = {
      id: existing?.id ?? randomUUID(),
      name: request.name?.trim() || check.name,
      url: target.url,
      token: target.token,
    };
    const others = this.connections.remotes.filter((remote) => remote.id !== connection.id);
    this.save({ ...this.connections, remotes: [...others, connection] });
    this.log(`connections: paired with ${connection.name} at ${connection.url}`);
    if (connection.id === this.connections.activeId) await this.inTurn(() => this.connectRemote());
    else await this.use(connection.id);
    return { ok: true, connection: { id: connection.id, name: connection.name, url: connection.url } };
  }

  async remove(id: string): Promise<void> {
    if (id === this.connections.activeId) await this.use(LOCAL_CONNECTION_ID);
    this.save({ ...this.connections, remotes: this.connections.remotes.filter((remote) => remote.id !== id) });
  }

  dispose(): void {
    this.stopProbe();
    this.generation++;
  }

  private async connectRemote(): Promise<void> {
    const remote = this.activeRemote;
    if (!remote) return;
    const generation = ++this.generation;
    this.stopProbe();
    this.setRemoteStatus({ state: "starting", connection: this.remoteConnection(remote) });
    await this.checkRemote(remote, generation);
    if (generation !== this.generation) return;
    this.probe = setInterval(() => void this.checkRemote(remote, generation), this.opts.probeMs ?? PROBE_MS);
  }

  private async checkRemote(remote: RemoteConnection, generation: number): Promise<void> {
    const result = await this.opts.check(remote.url, remote.token);
    if (generation !== this.generation) return;
    const connection = this.remoteConnection(remote);
    if (result.ok) {
      const ready = { ...connection, version: result.version };
      this.setRemoteStatus({ state: "ready", baseUrl: apiBase(remote.url), token: remote.token, connection: ready });
      return;
    }
    this.setRemoteStatus({ state: "error", message: result.error, connection });
  }

  private setRemoteStatus(next: DaemonStatus) {
    const unchanged = next.state === this.remoteStatus.state && next.message === this.remoteStatus.message;
    const sameRemote =
      next.connection?.id === this.remoteStatus.connection?.id &&
      next.connection?.version === this.remoteStatus.connection?.version;
    this.remoteStatus = next;
    if (unchanged && sameRemote) return;
    if (next.state === "error") this.log(`connections: ${next.connection?.name}: ${next.message}`);
    this.emitStatus();
  }

  private stopProbe() {
    if (this.probe === null) return;
    clearInterval(this.probe);
    this.probe = null;
  }

  private save(next: Connections) {
    this.opts.write(next);
    this.connections = next;
    const list = this.list();
    for (const listener of this.listListeners) listener(list);
  }

  private emitStatus() {
    const status = this.getStatus();
    for (const listener of this.statusListeners) listener(status);
  }

  private describe(id: string): string {
    return isLocal(id) ? this.opts.localName : (this.connections.remotes.find((r) => r.id === id)?.name ?? id);
  }

  private localConnection(): DaemonConnection {
    return { id: LOCAL_CONNECTION_ID, kind: "local", name: this.opts.localName };
  }

  private remoteConnection(remote: RemoteConnection): DaemonConnection {
    return { id: remote.id, kind: "remote", name: remote.name, url: remote.url };
  }
}
