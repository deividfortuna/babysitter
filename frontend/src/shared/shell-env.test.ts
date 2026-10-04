import { describe, expect, test, vi } from "vite-plus/test";
import {
  SHELL_ENV_MARKER,
  daemonEnvOnce,
  loginShellProbe,
  parseShellEnv,
  type DaemonEnvOptions,
  withFallbackPath,
  type ShellRunner,
} from "./shell-env";

const launchdEnv = { HOME: "/Users/ana", PATH: "/usr/bin:/bin:/usr/sbin:/sbin" };

function shellOutput(...records: string[]): string {
  return `Last login: today\n${SHELL_ENV_MARKER}${records.join("\0")}\0`;
}

function daemonEnv(options: DaemonEnvOptions) {
  return daemonEnvOnce(options)();
}

const probeCommand = `printf '%s' '${SHELL_ENV_MARKER}'; env -0`;

describe("loginShellProbe", () => {
  test("runs the shell of the user as an interactive login shell", () => {
    expect(loginShellProbe({ SHELL: "/opt/homebrew/bin/fish" }, "darwin")).toEqual({
      shell: "/opt/homebrew/bin/fish",
      args: ["-ilc", probeCommand],
    });
  });

  test("runs zsh when launchd gives no SHELL", () => {
    expect(loginShellProbe({ SHELL: " " }, "darwin").shell).toBe("/bin/zsh");
  });

  test("runs sh on Linux when the session gives no SHELL, because most distributions have no zsh", () => {
    expect(loginShellProbe({}, "linux").shell).toBe("/bin/sh");
  });

  test("runs tcsh and csh as interactive shells only, because they refuse -l with other flags", () => {
    expect(loginShellProbe({ SHELL: "/bin/tcsh" }, "darwin").args).toEqual(["-ic", probeCommand]);
    expect(loginShellProbe({ SHELL: "/bin/csh" }, "darwin").args).toEqual(["-ic", probeCommand]);
  });

  test("runs pwsh as a login shell with its own flags, because it refuses -ilc", () => {
    expect(loginShellProbe({ SHELL: "/usr/local/bin/pwsh" }, "darwin").args).toEqual([
      "-Login",
      "-Command",
      probeCommand,
    ]);
  });
});

describe("parseShellEnv", () => {
  test("reads the variables after the marker and drops what the shell printed before it", () => {
    const env = parseShellEnv(shellOutput("PATH=/opt/homebrew/bin:/usr/bin", "GITHUB_TOKEN=ghp_x", "EMPTY="));

    expect(env).toEqual({ PATH: "/opt/homebrew/bin:/usr/bin", GITHUB_TOKEN: "ghp_x", EMPTY: "" });
  });

  test("keeps a value that holds a newline or an equals sign", () => {
    const env = parseShellEnv(shellOutput("PATH=/usr/bin", "PS1=a\nb", "OPTS=x=1"));

    expect(env?.PS1).toBe("a\nb");
    expect(env?.OPTS).toBe("x=1");
  });

  test("reads the whole output when the marker is missing", () => {
    expect(parseShellEnv("PATH=/usr/bin\0HOME=/Users/ana\0")).toEqual({ PATH: "/usr/bin", HOME: "/Users/ana" });
  });

  test("reads one variable a line when the output has no NUL", () => {
    expect(parseShellEnv(`${SHELL_ENV_MARKER}PATH=/usr/bin\r\nHOME=/Users/ana\n`)).toEqual({
      PATH: "/usr/bin",
      HOME: "/Users/ana",
    });
  });

  test("gives nothing when the output has no PATH", () => {
    expect(parseShellEnv(shellOutput("HOME=/Users/ana"))).toBeNull();
  });
});

describe("daemonEnvOnce", () => {
  test("runs the login shell on the first call only, and gives every call the same environment", async () => {
    const run = vi.fn(async () => shellOutput("PATH=/opt/homebrew/bin"));
    const env = daemonEnvOnce({ platform: "darwin", env: launchdEnv, home: "/Users/ana", run });

    expect(run).not.toHaveBeenCalled();
    const [first, second] = await Promise.all([env(), env()]);

    expect(run).toHaveBeenCalledOnce();
    expect(second).toBe(first);
  });

  test("reads the login shell again after a read that failed, and keeps the first one that works", async () => {
    const run = vi
      .fn<ShellRunner>()
      .mockResolvedValueOnce(null)
      .mockResolvedValue(shellOutput("PATH=/opt/homebrew/bin", "GITHUB_TOKEN=ghp_shell"));
    const env = daemonEnvOnce({ platform: "darwin", env: launchdEnv, home: "/Users/ana", run });

    expect((await env()).GITHUB_TOKEN).toBeUndefined();
    expect((await env()).GITHUB_TOKEN).toBe("ghp_shell");
    await env();

    expect(run).toHaveBeenCalledTimes(2);
  });
});

describe("withFallbackPath", () => {
  test("appends the directories where the tools of a Mac usually are, after the ones it has", () => {
    expect(withFallbackPath("/usr/bin:/custom/bin:/bin", "/Users/ana")).toBe(
      [
        "/usr/bin",
        "/custom/bin",
        "/bin",
        "/opt/homebrew/bin",
        "/opt/homebrew/sbin",
        "/usr/local/bin",
        "/Users/ana/.local/bin",
        "/usr/sbin",
        "/sbin",
      ].join(":"),
    );
  });

  test("builds the path from the fallback alone when there is none", () => {
    expect(withFallbackPath(undefined, "/Users/ana").split(":")[0]).toBe("/opt/homebrew/bin");
  });
});

describe("the environment of the daemon", () => {
  test("gives the daemon the environment of the login shell under the variables of the app", async () => {
    const run = vi.fn(async () =>
      shellOutput(
        "PATH=/opt/homebrew/bin:/Users/ana/.local/bin:/usr/bin",
        "GITHUB_TOKEN=ghp_shell",
        "HOME=/Users/other",
      ),
    );

    const env = await daemonEnv({
      platform: "darwin",
      env: { ...launchdEnv, SHELL: "/bin/zsh" },
      home: "/Users/ana",
      run,
    });

    expect(run).toHaveBeenCalledWith("/bin/zsh", loginShellProbe({ SHELL: "/bin/zsh" }, "darwin").args);
    expect(env.GITHUB_TOKEN).toBe("ghp_shell");
    expect(env.HOME).toBe("/Users/ana");
    expect(env.PATH?.startsWith("/opt/homebrew/bin:/Users/ana/.local/bin:/usr/bin:")).toBe(true);
  });

  test("gives the daemon the variables of the shell over those launchd gives the app, except the ones the app owns", async () => {
    const run = async () =>
      shellOutput(
        "PATH=/usr/bin",
        "SSH_AUTH_SOCK=/Users/ana/.1password/agent.sock",
        "HOME=/Users/other",
        "TMPDIR=/tmp/shell/",
        "BABYSITTER_DATA_DIR=/shell",
      );
    const appEnv = {
      ...launchdEnv,
      SSH_AUTH_SOCK: "/private/tmp/com.apple.launchd.x/Listeners",
      TMPDIR: "/var/folders/x/T/",
      BABYSITTER_DATA_DIR: "/app",
      XPC_SERVICE_NAME: "0",
    };

    const env = await daemonEnv({ platform: "darwin", env: appEnv, home: "/Users/ana", run });

    expect(env.SSH_AUTH_SOCK).toBe("/Users/ana/.1password/agent.sock");
    expect(env.HOME).toBe("/Users/ana");
    expect(env.TMPDIR).toBe("/var/folders/x/T/");
    expect(env.BABYSITTER_DATA_DIR).toBe("/app");
    expect(env.XPC_SERVICE_NAME).toBe("0");
  });

  test("falls back to the path of the app with the usual directories when the shell fails, and says so", async () => {
    const log = vi.fn();

    const env = await daemonEnv({
      platform: "darwin",
      env: launchdEnv,
      home: "/Users/ana",
      run: async () => null,
      log,
    });

    expect(env.PATH).toBe(withFallbackPath(launchdEnv.PATH, "/Users/ana"));
    expect(log).toHaveBeenCalledWith(expect.stringContaining("login shell"));
  });

  test("falls back when the shell throws", async () => {
    const env = await daemonEnv({
      platform: "linux",
      env: launchdEnv,
      home: "/home/ana",
      run: async () => {
        throw new Error("spawn ENOENT");
      },
    });

    expect(env.PATH).toContain("/home/ana/.local/bin");
  });

  test("leaves the environment of the app as it is on Windows", async () => {
    const run = vi.fn();
    const env = { Path: "C:\\Windows" };

    expect(await daemonEnv({ platform: "win32", env, home: "C:\\Users\\ana", run })).toEqual(env);
    expect(run).not.toHaveBeenCalled();
  });
});
