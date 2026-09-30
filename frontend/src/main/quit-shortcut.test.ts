import { afterEach, beforeEach, expect, test, vi } from "vite-plus/test";
import type { QuitShortcutHint } from "../shared/quit";
import {
  concealWindow,
  QUIT_DOUBLE_PRESS_MS,
  QUIT_HOLD_MS,
  QUIT_RELEASE_GRACE_MS,
  quitShortcut,
  type QuitKeyInput,
  type QuitShortcutHandler,
} from "./quit-shortcut";

const REPEAT_MS = 50;

let hints: QuitShortcutHint[];
let quits: number;
let concealed: number;
let handle: QuitShortcutHandler;

function setup(platform: NodeJS.Platform = "darwin") {
  hints = [];
  quits = 0;
  concealed = 0;
  handle = quitShortcut({
    platform,
    notify: (hint) => hints.push(hint),
    conceal: () => concealed++,
    quit: () => quits++,
  });
}

function key(type: "keyDown" | "keyUp", name: string, input: Partial<QuitKeyInput> = {}): boolean {
  let prevented = false;
  handle(
    { preventDefault: () => (prevented = true) },
    { type, key: name, meta: false, control: false, alt: false, shift: false, isAutoRepeat: false, ...input },
  );
  return prevented;
}

function pressQ(input: Partial<QuitKeyInput> = {}) {
  return key("keyDown", "q", { meta: true, ...input });
}

function holdQ(ms: number) {
  for (let held = REPEAT_MS; held <= ms; held += REPEAT_MS) {
    vi.advanceTimersByTime(REPEAT_MS);
    pressQ({ isAutoRepeat: true });
  }
}

beforeEach(() => {
  vi.useFakeTimers();
  setup();
});

afterEach(() => {
  vi.useRealTimers();
});

test("a single press of Command Q keeps the app open and shows the hint", () => {
  expect(pressQ()).toBe(true);
  key("keyUp", "q", { meta: true });
  vi.advanceTimersByTime(QUIT_HOLD_MS * 2);

  expect(quits).toBe(0);
  expect(hints).toEqual([{ state: "down" }, { state: "up" }]);
});

test("two presses in a short time quit the app", () => {
  pressQ();
  key("keyUp", "q", { meta: true });
  vi.advanceTimersByTime(QUIT_DOUBLE_PRESS_MS - 1);
  pressQ();

  expect(quits).toBe(1);
});

test("two presses with a long gap between them do not quit the app", () => {
  pressQ();
  key("keyUp", "q", { meta: true });
  vi.advanceTimersByTime(QUIT_DOUBLE_PRESS_MS + 1);
  pressQ();

  expect(quits).toBe(0);
});

test("a second press after the command key goes up and down again still quits", () => {
  pressQ();
  key("keyUp", "q", { meta: true });
  key("keyUp", "Meta");
  key("keyDown", "Meta", { meta: true });
  pressQ();

  expect(quits).toBe(1);
});

test("another key between the two presses cancels the double press", () => {
  pressQ();
  key("keyUp", "q", { meta: true });
  key("keyDown", "k", { meta: true });
  pressQ();

  expect(quits).toBe(0);
});

test("a hold hides the window and quits when the key goes up", () => {
  pressQ();
  holdQ(QUIT_HOLD_MS);

  expect(concealed).toBe(1);
  expect(quits).toBe(0);

  key("keyUp", "q", { meta: true });
  expect(quits).toBe(1);
});

test("a hold whose key up never comes quits after the repeats stop", () => {
  pressQ();
  holdQ(QUIT_HOLD_MS);

  vi.advanceTimersByTime(QUIT_RELEASE_GRACE_MS - 1);
  expect(quits).toBe(0);
  vi.advanceTimersByTime(1);
  expect(quits).toBe(1);
});

test("a hold that the user lets go before the time does not quit", () => {
  pressQ();
  holdQ(QUIT_HOLD_MS - REPEAT_MS);
  key("keyUp", "q", { meta: true });
  vi.advanceTimersByTime(QUIT_HOLD_MS * 2);

  expect({ quits, concealed }).toEqual({ quits: 0, concealed: 0 });
});

test("a tap whose key up macOS drops does not quit without repeats", () => {
  pressQ();
  vi.advanceTimersByTime(QUIT_HOLD_MS + QUIT_RELEASE_GRACE_MS);

  expect(quits).toBe(0);
  expect(hints).toEqual([{ state: "down" }, { state: "up" }]);
});

test("the keys that come after a completed hold do not reach the page", () => {
  pressQ();
  holdQ(QUIT_HOLD_MS);

  expect(key("keyDown", "a")).toBe(true);
});

test("Command Q with Shift or Option is not the quit shortcut", () => {
  expect(pressQ({ shift: true })).toBe(false);
  expect(pressQ({ alt: true })).toBe(false);
  expect(hints).toEqual([]);
});

test("off macOS the shortcut is Control Q", () => {
  setup("linux");

  expect(pressQ()).toBe(false);
  expect(key("keyDown", "q", { control: true })).toBe(true);
  key("keyUp", "q", { control: true });
  key("keyDown", "q", { control: true });

  expect(quits).toBe(1);
});

test("the concealed window leaves full screen and turns transparent", () => {
  const win = {
    isDestroyed: () => false,
    isFullScreen: () => true,
    setFullScreen: vi.fn(),
    setOpacity: vi.fn(),
  };

  concealWindow(win);

  expect(win.setFullScreen).toHaveBeenCalledWith(false);
  expect(win.setOpacity).toHaveBeenCalledWith(0);
});
