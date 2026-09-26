import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiBaseUrl } from "./api-client";
import { connectEventTransport } from "./event-transport";
import {
  notificationsQueryKey,
  providersQueryKey,
  rateLimitQueryKey,
  viewerQueryKey,
  watchesQueryKey,
} from "./query-keys";

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  listeners = new Map<string, Set<(event: MessageEvent) => void>>();
  closed = false;

  constructor(public url: string) {
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(listener);
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.get(type)?.delete(listener);
  }

  close() {
    this.closed = true;
  }

  dispatch(type: string, data: unknown = {}) {
    const event = { data: JSON.stringify(data) } as MessageEvent;
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }
}

afterEach(() => {
  FakeEventSource.instances = [];
});

describe("connectEventTransport", () => {
  it("invalidates the provider catalog when the feed becomes ready", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");

    const dispose = connectEventTransport(queryClient);
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("ready");

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: providersQueryKey });

    dispose();
  });

  it("invalidates the account when the feed becomes ready", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");

    const dispose = connectEventTransport(queryClient);
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("ready");

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: viewerQueryKey });

    dispose();
  });

  it("refetches the history when a notification arrives", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");

    const dispose = connectEventTransport(queryClient);
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("notification_added");

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: notificationsQueryKey });

    dispose();
  });

  it("tells the daemon that the app shows the notifications itself", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");

    const dispose = connectEventTransport(new QueryClient(), () => undefined, { present: true });

    expect(FakeEventSource.instances.at(-1)!.url).toBe("http://localhost:1234/events?present=1");

    dispose();
  });

  it("leaves the showing to the daemon outside the app", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");

    const dispose = connectEventTransport(new QueryClient());

    expect(FakeEventSource.instances.at(-1)!.url).toBe("http://localhost:1234/events");

    dispose();
  });

  it("refetches the watches when the readiness of one changes", () => {
    vi.useFakeTimers();
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");

    const dispose = connectEventTransport(queryClient);
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("watch_ready");
    vi.runAllTimers();

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: watchesQueryKey });

    dispose();
    vi.useRealTimers();
  });

  it.each(["repo_synced", "watch_activity", "watch_changed"])(
    "refetches the rate limit on %s, which follows calls to GitHub",
    (type) => {
      vi.useFakeTimers();
      vi.stubGlobal("EventSource", FakeEventSource);
      setApiBaseUrl("http://localhost:1234");
      const queryClient = new QueryClient();
      const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");

      const dispose = connectEventTransport(queryClient);
      FakeEventSource.instances.at(-1)!.dispatch(type);
      vi.runAllTimers();

      expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: rateLimitQueryKey });

      dispose();
      vi.useRealTimers();
    },
  );

  it.each(["watch_proposal", "watch_changed"])("refetches the watches on %s", (type) => {
    vi.useFakeTimers();
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");

    const dispose = connectEventTransport(queryClient);
    FakeEventSource.instances.at(-1)!.dispatch(type);
    vi.runAllTimers();

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: watchesQueryKey });

    dispose();
    vi.useRealTimers();
  });
});

describe("the ready frame", () => {
  it("hands over the newest notification the daemon holds", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const onConnection = vi.fn();

    const dispose = connectEventTransport(queryClient, onConnection, { present: true });
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("ready", { seq: 12, lastNotificationId: 7 });

    expect(onConnection).toHaveBeenLastCalledWith("open", { lastNotificationId: 7 });

    dispose();
  });

  it("names no row when the daemon kept the showing", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const onConnection = vi.fn();

    const dispose = connectEventTransport(queryClient, onConnection, { present: true });
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("ready", { seq: 12, lastNotificationId: null });

    expect(onConnection).toHaveBeenLastCalledWith("open", { lastNotificationId: null });

    dispose();
  });

  it("names no row when the frame cannot be read", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    setApiBaseUrl("http://localhost:1234");
    const queryClient = new QueryClient();
    const onConnection = vi.fn();

    const dispose = connectEventTransport(queryClient, onConnection, { present: true });
    const es = FakeEventSource.instances.at(-1)!;
    es.dispatch("ready");

    expect(onConnection).toHaveBeenLastCalledWith("open", { lastNotificationId: null });

    dispose();
  });
});
