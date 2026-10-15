import { expect, test, vi } from "vite-plus/test";
import { notarizeApp, type Command, type CommandResult } from "./notarize";

const key = { key: "/tmp/AuthKey.p8", keyId: "KEYID", issuer: "issuer-uuid" };
const app = "/build/mac/Babysitter.app";
const auth = ["--key", "/tmp/AuthKey.p8", "--key-id", "KEYID", "--issuer", "issuer-uuid", "--output-format", "json"];

const ok = (output = ""): CommandResult => ({ status: 0, output });
const down = (output = "The network connection was lost."): CommandResult => ({ status: 1, output });
const submitted = ok(JSON.stringify({ id: "sub-1", message: "Successfully uploaded file" }));
const accepted = ok(JSON.stringify({ id: "sub-1", status: "Accepted" }));

function tool(command: Command): string {
  return command.command === "xcrun" ? (command.args[1] ?? "") : command.command;
}

function notary(script: Record<string, CommandResult[]>) {
  const commands: Command[] = [];
  const pause = vi.fn(async () => {});
  const log = vi.fn();
  const run = async (command: Command) => {
    commands.push(command);
    const results = script[tool(command)] ?? [];
    const result = results.length > 1 ? results.shift() : results[0];
    return result ?? ok();
  };
  return { commands, pause, log, options: { run, pause, log, attempts: 3, pauseMs: 10 } };
}

test("zips the app, submits the zip, waits for the submission and staples the app", async () => {
  const n = notary({ submit: [submitted], wait: [accepted] });

  await notarizeApp(app, key, n.options);

  expect(n.commands.map(tool)).toEqual(["ditto", "submit", "wait", "staple"]);
  const [zip, submit, wait, staple] = n.commands;
  expect(zip).toMatchObject({ command: "ditto", cwd: "/build/mac" });
  expect(zip?.args.at(-2)).toBe("Babysitter.app");
  expect(zip?.args.at(-1)).toMatch(/Babysitter\.zip$/);
  expect(submit?.args).toEqual(["notarytool", "submit", zip?.args.at(-1), ...auth]);
  expect(wait?.args).toEqual(["notarytool", "wait", "sub-1", ...auth]);
  expect(staple).toEqual({ command: "xcrun", args: ["stapler", "staple", "-v", "Babysitter.app"], cwd: "/build/mac" });
  expect(n.pause).not.toHaveBeenCalled();
});

test("a lost connection during the wait waits again for the same submission, without a new upload", async () => {
  const n = notary({ submit: [submitted], wait: [down(), down(), accepted] });

  await notarizeApp(app, key, n.options);

  expect(n.commands.map(tool)).toEqual(["ditto", "submit", "wait", "wait", "wait", "staple"]);
  expect(n.pause).toHaveBeenCalledTimes(2);
  expect(n.pause).toHaveBeenCalledWith(10);
});

test("a lost connection during the staple staples again, without a new submission", async () => {
  const n = notary({ submit: [submitted], wait: [accepted], staple: [down(), ok()] });

  await notarizeApp(app, key, n.options);

  expect(n.commands.map(tool)).toEqual(["ditto", "submit", "wait", "staple", "staple"]);
  expect(n.log).toHaveBeenCalledWith(expect.stringContaining("staple failed on attempt 1 of 3"));
});

test("a step that fails every attempt fails the build and names the step", async () => {
  const n = notary({ submit: [down("offline")] });

  await expect(notarizeApp(app, key, n.options)).rejects.toThrow("submit failed 3 times");
  expect(n.commands.map(tool)).toEqual(["ditto", "submit", "submit", "submit"]);
  expect(n.pause).toHaveBeenCalledTimes(2);
});

test("a submission Apple refuses fails the build with the log of the notary and does not staple", async () => {
  const invalid = ok(JSON.stringify({ id: "sub-1", status: "Invalid" }));
  const n = notary({ submit: [submitted], wait: [invalid], log: [ok("issues: unsigned binary")] });

  await expect(notarizeApp(app, key, n.options)).rejects.toThrow(/ended Invalid\nissues: unsigned binary/);
  expect(n.commands.map(tool)).toEqual(["ditto", "submit", "wait", "log"]);
});
