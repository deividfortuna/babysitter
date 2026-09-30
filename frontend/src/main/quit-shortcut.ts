import type { BrowserWindow } from "electron";
import type { QuitShortcutHint } from "../shared/quit";

export const QUIT_HOLD_MS = 1200;
export const QUIT_DOUBLE_PRESS_MS = 500;
export const QUIT_RELEASE_GRACE_MS = 600;
const REPEAT_CADENCES_TO_WAIT = 2;

export type QuitKeyInput = Pick<Electron.Input, "type" | "key" | "meta" | "control" | "alt" | "shift" | "isAutoRepeat">;

export type QuitShortcutOptions = {
  platform: NodeJS.Platform;
  notify(hint: QuitShortcutHint): void;
  conceal(): void;
  quit(): void;
};

export type QuitShortcutHandler = (event: { preventDefault(): void }, input: QuitKeyInput) => void;

// macOS drops the keyUp of a letter while the command key is down, so only
// the auto repeats of the key prove that it is still held.
export function quitShortcut(options: QuitShortcutOptions): QuitShortcutHandler {
  const modifierKey = options.platform === "darwin" ? "meta" : "control";
  let timer: ReturnType<typeof setTimeout> | undefined;
  let hinted = false;
  let holding = false;
  let quitOnRelease = false;
  let heldSince = 0;
  let lastPressAt = 0;
  let lastRepeatAt = 0;
  let repeatCadence = 0;

  function clearTimer() {
    if (timer === undefined) return;
    clearTimeout(timer);
    timer = undefined;
  }

  function release() {
    clearTimer();
    holding = false;
    quitOnRelease = false;
    lastRepeatAt = 0;
    repeatCadence = 0;
    if (!hinted) return;
    hinted = false;
    options.notify({ state: "up" });
  }

  function quitNow() {
    release();
    lastPressAt = 0;
    options.quit();
  }

  function quitWhenQuiet() {
    clearTimer();
    timer = setTimeout(quitNow, Math.max(QUIT_RELEASE_GRACE_MS, repeatCadence * REPEAT_CADENCES_TO_WAIT));
  }

  function modifierDown(input: QuitKeyInput) {
    return options.platform === "darwin" ? input.meta : input.control;
  }

  function onlyModifierDown(input: QuitKeyInput) {
    const pressed = [input.meta, input.control, input.alt, input.shift].filter(Boolean).length;
    return modifierDown(input) && pressed === 1;
  }

  function isQuitKey(input: QuitKeyInput) {
    return input.key.toLowerCase() === "q" && onlyModifierDown(input);
  }

  function isModifierAlone(input: QuitKeyInput) {
    return input.key.toLowerCase() === modifierKey && !input.alt && !input.shift;
  }

  function trackRepeat() {
    const now = Date.now();
    repeatCadence = now - (lastRepeatAt === 0 ? heldSince : lastRepeatAt);
    lastRepeatAt = now;
  }

  function press() {
    const now = Date.now();
    const previousPressAt = lastPressAt;
    lastPressAt = now;
    release();
    if (previousPressAt !== 0 && now - previousPressAt <= QUIT_DOUBLE_PRESS_MS) {
      quitNow();
      return;
    }
    holding = true;
    heldSince = now;
    hinted = true;
    options.notify({ state: "down" });
    timer = setTimeout(release, QUIT_HOLD_MS + QUIT_RELEASE_GRACE_MS);
  }

  function repeat() {
    trackRepeat();
    if (!holding || Date.now() - heldSince < QUIT_HOLD_MS) return;
    holding = false;
    quitOnRelease = true;
    options.conceal();
    quitWhenQuiet();
  }

  function keyUp(input: QuitKeyInput) {
    const key = input.key.toLowerCase();
    if (key === "q") {
      if (quitOnRelease) quitNow();
      else release();
      return;
    }
    if (key !== modifierKey) return;
    if (quitOnRelease) quitWhenQuiet();
    else release();
  }

  function keyDown(event: { preventDefault(): void }, input: QuitKeyInput) {
    if (quitOnRelease) {
      event.preventDefault();
      if (input.key.toLowerCase() !== "q") return;
      if (input.isAutoRepeat) trackRepeat();
      quitWhenQuiet();
      return;
    }
    if (!isQuitKey(input)) {
      if (isModifierAlone(input) || input.isAutoRepeat) return;
      lastPressAt = 0;
      release();
      return;
    }
    event.preventDefault();
    if (input.isAutoRepeat) repeat();
    else press();
  }

  return (event, input) => {
    if (input.type === "keyUp") keyUp(input);
    else if (input.type === "keyDown") keyDown(event, input);
  };
}

export function concealWindow(
  win: Pick<BrowserWindow, "isDestroyed" | "isFullScreen" | "setFullScreen" | "setOpacity">,
) {
  if (win.isDestroyed()) return;
  if (win.isFullScreen()) win.setFullScreen(false);
  win.setOpacity(0);
}
