import dgram from "node:dgram";
import type { DiscoveredDaemon } from "../shared/connections";

export const SERVICE_TYPE = "_babysitter._tcp.local";

const MDNS_GROUP = "224.0.0.251";
const MDNS_PORT = 5353;
const TYPE_PTR = 12;
const TYPE_TXT = 16;
const CLASS_IN = 1;
const QUERY_ID = 0x6273;
const DEFAULT_WINDOW_MS = 1_500;

export function encodeQuery(name: string, id = QUERY_ID): Buffer {
  const header = Buffer.alloc(12);
  header.writeUInt16BE(id, 0);
  header.writeUInt16BE(1, 4);
  const question = Buffer.alloc(4);
  question.writeUInt16BE(TYPE_PTR, 0);
  question.writeUInt16BE(CLASS_IN, 2);
  return Buffer.concat([header, encodeName(name), question]);
}

function encodeName(name: string): Buffer {
  const labels = name.replace(/\.$/, "").split(".");
  const parts = labels.map((label) => {
    const bytes = Buffer.from(label, "utf8");
    return Buffer.concat([Buffer.from([bytes.length]), bytes]);
  });
  return Buffer.concat([...parts, Buffer.from([0])]);
}

type Record = { name: string; type: number; data: Buffer };

function readName(packet: Buffer, start: number): { name: string; next: number } {
  const labels: string[] = [];
  let offset = start;
  let next = -1;
  for (let jumps = 0; jumps < 32; jumps++) {
    const length = packet[offset];
    if (length === undefined) break;
    if (length === 0) {
      offset += 1;
      break;
    }
    if ((length & 0xc0) === 0xc0) {
      if (next < 0) next = offset + 2;
      offset = ((length & 0x3f) << 8) | packet[offset + 1];
      continue;
    }
    labels.push(packet.toString("utf8", offset + 1, offset + 1 + length));
    offset += 1 + length;
  }
  return { name: labels.join("."), next: next < 0 ? offset : next };
}

export function decodeRecords(packet: Buffer): Record[] {
  if (packet.length < 12) return [];
  const flags = packet.readUInt16BE(2);
  if ((flags & 0x8000) === 0) return [];
  const questions = packet.readUInt16BE(4);
  const total = packet.readUInt16BE(6) + packet.readUInt16BE(8) + packet.readUInt16BE(10);
  let offset = 12;
  for (let i = 0; i < questions; i++) offset = readName(packet, offset).next + 4;
  const records: Record[] = [];
  for (let i = 0; i < total && offset < packet.length; i++) {
    const { name, next } = readName(packet, offset);
    const type = packet.readUInt16BE(next);
    const length = packet.readUInt16BE(next + 8);
    const dataOffset = next + 10;
    records.push({ name, type, data: packet.subarray(dataOffset, dataOffset + length) });
    offset = dataOffset + length;
  }
  return records;
}

function txtPairs(data: Buffer): Map<string, string> {
  const pairs = new Map<string, string>();
  let offset = 0;
  while (offset < data.length) {
    const length = data[offset];
    const entry = data.toString("utf8", offset + 1, offset + 1 + length);
    const split = entry.indexOf("=");
    if (split > 0) pairs.set(entry.slice(0, split), entry.slice(split + 1));
    offset += 1 + length;
  }
  return pairs;
}

export function daemonsIn(packet: Buffer, address: string): DiscoveredDaemon[] {
  const found: DiscoveredDaemon[] = [];
  for (const record of decodeRecords(packet)) {
    const ofService = record.name.toLowerCase().endsWith(SERVICE_TYPE);
    if (record.type !== TYPE_TXT || !ofService) continue;
    const txt = txtPairs(record.data);
    const port = Number(txt.get("port"));
    if (!Number.isInteger(port) || port < 1 || port > 65535) continue;
    found.push({
      name: txt.get("name") || txt.get("host") || address,
      host: txt.get("host") ?? "",
      address,
      port,
      version: txt.get("version") ?? "",
      url: `http://${address}:${port}`,
    });
  }
  return found;
}

export type DiscoverOptions = {
  windowMs?: number;
  log?: (msg: string) => void;
};

export function discoverDaemons(options: DiscoverOptions = {}): Promise<DiscoveredDaemon[]> {
  const windowMs = options.windowMs ?? DEFAULT_WINDOW_MS;
  const log = options.log ?? (() => undefined);
  return new Promise((resolve) => {
    const found = new Map<string, DiscoveredDaemon>();
    const socket = dgram.createSocket({ type: "udp4" });
    const finish = () => {
      socket.close();
      resolve([...found.values()]);
    };
    socket.on("message", (packet, from) => {
      for (const daemon of daemonsIn(packet, from.address)) found.set(daemon.url, daemon);
    });
    socket.on("error", (err) => {
      log(`discovery: ${err.message}`);
      finish();
    });
    socket.bind(0, () => {
      socket.send(encodeQuery(SERVICE_TYPE), MDNS_PORT, MDNS_GROUP, (err) => {
        if (err) log(`discovery: could not send the query: ${err.message}`);
      });
      setTimeout(finish, windowMs);
    });
  });
}
