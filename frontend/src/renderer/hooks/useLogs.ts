import { useEffect, useState, useSyncExternalStore } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import type { LogLevel, LogRecord } from "../../shared/logs";
import { api, apiErrorMessage, getApiBaseUrl, subscribeApiBaseUrl } from "../lib/api-client";
import { bridge } from "../lib/bridge";
import { logLevelQueryKey } from "../lib/query-keys";

type DaemonLogRecord = components["schemas"]["HttpdLogRecord"];

export const LOG_VIEW_LIMIT = 2000;
const FLUSH_MS = 200;

export type LogStream = {
  records: LogRecord[];
  connected: boolean;
};

function fromDaemon(record: DaemonLogRecord): LogRecord {
  return { ...record, attrs: record.attrs ?? [] };
}

function lastOf(records: LogRecord[]): LogRecord[] {
  return records.length > LOG_VIEW_LIMIT ? records.slice(-LOG_VIEW_LIMIT) : records;
}

export function useDaemonLogs(enabled: boolean): LogStream {
  const base = useApiBaseUrl();
  const [stream, setStream] = useState<LogStream>({ records: [], connected: false });

  useEffect(() => {
    if (!enabled || !base) return;
    let pending: LogRecord[] = [];
    let restart = false;
    const flush = setInterval(() => {
      if (!restart && pending.length === 0) return;
      const arrived = pending;
      const from = restart;
      pending = [];
      restart = false;
      setStream((current) => ({
        connected: true,
        records: lastOf(from ? arrived : [...current.records, ...arrived]),
      }));
    }, FLUSH_MS);

    const source = new EventSource(`${base}/logs/stream?after=0`);
    source.addEventListener("ready", () => {
      pending = [];
      restart = true;
    });
    source.addEventListener("log", (event: MessageEvent) => {
      pending.push(fromDaemon(JSON.parse(String(event.data)) as DaemonLogRecord));
    });
    source.addEventListener("error", () => {
      setStream((current) => ({ ...current, connected: false }));
    });

    return () => {
      clearInterval(flush);
      source.close();
    };
  }, [enabled, base]);

  return stream;
}

export function useAppLogs(enabled: boolean): LogRecord[] {
  const [records, setRecords] = useState<LogRecord[]>([]);

  useEffect(() => {
    if (!enabled) return;
    let arrived: LogRecord[] = [];
    let loaded = false;
    const stop = bridge.logs.onAppRecord((record) => {
      if (!loaded) {
        arrived.push(record);
        return;
      }
      setRecords((current) => lastOf([...current, record]));
    });
    void bridge.logs.appRecords().then((kept) => {
      const last = kept.at(-1)?.seq ?? 0;
      setRecords(lastOf([...kept, ...arrived.filter((r) => r.seq > last)]));
      arrived = [];
      loaded = true;
    });
    return stop;
  }, [enabled]);

  return records;
}

function useApiBaseUrl(): string | null {
  return useSyncExternalStore(subscribeApiBaseUrl, getApiBaseUrl);
}

export function useLogLevel() {
  const base = useApiBaseUrl();
  return useQuery({
    queryKey: logLevelQueryKey,
    enabled: base !== null,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/logs/level");
      if (error) throw new Error(apiErrorMessage(error, "Could not read the log level of the daemon."));
      return data.level;
    },
  });
}

export function useSetLogLevel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (level: LogLevel) => {
      const { data, error } = await api().PUT("/api/v1/logs/level", { body: { level } });
      if (error) throw new Error(apiErrorMessage(error, "Could not change the log level of the daemon."));
      return data.level;
    },
    onSuccess: (level) => queryClient.setQueryData(logLevelQueryKey, level),
  });
}
