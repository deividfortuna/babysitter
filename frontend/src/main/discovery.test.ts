import dgram from "node:dgram";
import { expect, test, vi } from "vite-plus/test";
import { daemonsIn, decodeRecords, discoverDaemons, encodeQuery, SERVICE_TYPE } from "./discovery";

function name(value: string): Buffer {
  const labels = value.split(".").map((label) => Buffer.concat([Buffer.from([label.length]), Buffer.from(label)]));
  return Buffer.concat([...labels, Buffer.from([0])]);
}

function record(owner: Buffer, type: number, data: Buffer): Buffer {
  const fixed = Buffer.alloc(10);
  fixed.writeUInt16BE(type, 0);
  fixed.writeUInt16BE(1, 2);
  fixed.writeUInt32BE(120, 4);
  fixed.writeUInt16BE(data.length, 8);
  return Buffer.concat([owner, fixed, data]);
}

function txt(...entries: string[]): Buffer {
  return Buffer.concat(entries.map((entry) => Buffer.concat([Buffer.from([entry.length]), Buffer.from(entry)])));
}

function answer(...records: Buffer[]): Buffer {
  const header = Buffer.alloc(12);
  header.writeUInt16BE(0x8400, 2);
  header.writeUInt16BE(records.length, 6);
  return Buffer.concat([header, ...records]);
}

test("the query asks for the pointer records of the babysitter service", () => {
  const query = encodeQuery(SERVICE_TYPE, 7);
  expect(query.readUInt16BE(0)).toBe(7);
  expect(query.readUInt16BE(4)).toBe(1);
  expect(query.subarray(12).toString("latin1")).toContain("_babysitter");
});

test("a daemon comes from the text record of an answer and the address that sent it", () => {
  const instance = name(`studio-server.${SERVICE_TYPE}`);
  const packet = answer(
    record(name(SERVICE_TYPE), 12, Buffer.from([0xc0, 12])),
    record(instance, 16, txt("name=studio server", "host=studio", "port=7420", "version=1.2.3")),
  );

  expect(daemonsIn(packet, "192.168.1.20")).toEqual([
    {
      name: "studio server",
      host: "studio",
      address: "192.168.1.20",
      port: 7420,
      version: "1.2.3",
      url: "http://192.168.1.20:7420",
    },
  ]);
});

test("a compressed owner name reads like a full one", () => {
  const owner = name(`studio.${SERVICE_TYPE}`);
  const pointer = Buffer.from([0xc0, 12]);
  const packet = answer(record(owner, 12, Buffer.alloc(0)), record(pointer, 16, txt("port=7420")));

  const names = decodeRecords(packet).map((r) => r.name);
  expect(names).toEqual([`studio.${SERVICE_TYPE}`, `studio.${SERVICE_TYPE}`]);
});

test("answers of other services, queries and records without a port give no daemon", () => {
  const other = answer(record(name("tv._airplay._tcp.local"), 16, txt("port=7000")));
  const noPort = answer(record(name(`studio.${SERVICE_TYPE}`), 16, txt("name=studio")));
  const query = encodeQuery(SERVICE_TYPE);

  expect(daemonsIn(other, "192.168.1.30")).toEqual([]);
  expect(daemonsIn(noPort, "192.168.1.30")).toEqual([]);
  expect(daemonsIn(query, "192.168.1.30")).toEqual([]);
});

test("a truncated answer gives no daemon instead of an error", () => {
  const instance = name(`studio.${SERVICE_TYPE}`);
  const whole = answer(record(instance, 16, txt("port=7420")));
  const truncated = whole.subarray(0, 12 + instance.length + 4);

  expect(daemonsIn(truncated, "192.168.1.30")).toEqual([]);
});

test("an error of the socket ends the discovery once", async () => {
  const createSocket = dgram.createSocket;
  let socket: dgram.Socket | undefined;
  const spy = vi.spyOn(dgram, "createSocket").mockImplementation(((options: dgram.SocketOptions) => {
    socket = createSocket(options);
    return socket;
  }) as typeof dgram.createSocket);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  try {
    const done = discoverDaemons({ windowMs: 1_000 });
    await vi.waitFor(() => socket?.address());
    socket?.emit("error", new Error("network is down"));

    await expect(done).resolves.toEqual([]);
    expect(() => vi.runAllTimers()).not.toThrow();
  } finally {
    vi.useRealTimers();
    spy.mockRestore();
  }
});
