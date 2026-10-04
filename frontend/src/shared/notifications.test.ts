import { expect, test } from "vite-plus/test";
import {
  badgeText,
  bounceType,
  clickPlan,
  clickTarget,
  hasKind,
  presentation,
  shouldReplaceBounce,
  shouldSignalAttention,
  shouldToast,
  trayTooltip,
  withKind,
  testNotification,
} from "./notifications";

test("the badge is empty under one and stops at 99+", () => {
  expect(badgeText(0)).toBe("");
  expect(badgeText(-2)).toBe("");
  expect(badgeText(7)).toBe("7");
  expect(badgeText(100)).toBe("99+");
});

test("only what waits on the user asks for attention", () => {
  expect(shouldSignalAttention("agent")).toBe(true);
  expect(shouldSignalAttention("merge")).toBe(true);
  expect(shouldSignalAttention("review")).toBe(false);
  expect(shouldSignalAttention("checks")).toBe(false);
  expect(shouldSignalAttention("watch")).toBe(false);
  expect(shouldSignalAttention(undefined)).toBe(false);
});

test("the agent waiting on you keeps the dock bouncing, the rest bounce once", () => {
  expect(bounceType("agent")).toBe("critical");
  expect(bounceType("merge")).toBe("informational");
  expect(bounceType(undefined)).toBe("informational");
});

test("a bounce that waits on the user is never downgraded", () => {
  expect(shouldReplaceBounce(null)).toBe(true);
  expect(shouldReplaceBounce({ critical: false })).toBe(true);
  expect(shouldReplaceBounce({ critical: true })).toBe(false);
});

test("the menu bar says how many notifications wait", () => {
  expect(trayTooltip(0)).toBe("babysitter: nothing unread");
  expect(trayTooltip(1)).toBe("babysitter: 1 unread notification");
  expect(trayTooltip(4)).toBe("babysitter: 4 unread notifications");
});

const pullRequest = "https://github.com/octo/hello/pull/42";

test.each([
  {
    name: "a click on a notification of a watch opens the watch, raises the app, and the row is read",
    notification: { id: 9, title: "t", body: "b", watchId: 7, url: pullRequest },
    target: { kind: "watch", watchId: 7 },
    plan: { browser: null, raise: true, click: { id: 9, watchId: 7 } },
  },
  {
    name: "a click on the row of a watch with no pull request opens the watch, raises the app, and the row is read",
    notification: { id: 9, title: "t", body: "b", watchId: 7 },
    target: { kind: "watch", watchId: 7 },
    plan: { browser: null, raise: true, click: { id: 9, watchId: 7 } },
  },
  {
    name: "a click on a notification of no watch opens its pull request in the browser, and the row is read",
    notification: { id: 9, title: "t", body: "b", url: pullRequest },
    target: { kind: "url", url: pullRequest },
    plan: { browser: pullRequest, raise: false, click: { id: 9, watchId: undefined } },
  },
  {
    name: "a click with nothing to open only raises the app, and the row is read",
    notification: { id: 9, title: "t", body: "b" },
    target: { kind: "app" },
    plan: { browser: null, raise: true, click: { id: 9, watchId: undefined } },
  },
  {
    name: "a click on a link that is not https only raises the app, and the row is read",
    notification: { id: 9, title: "t", body: "b", url: "file:///etc/passwd" },
    target: { kind: "app" },
    plan: { browser: null, raise: true, click: { id: 9, watchId: undefined } },
  },
])("$name", ({ notification, target, plan }) => {
  expect(clickTarget(notification)).toEqual(target);
  expect(clickPlan(notification)).toEqual(plan);
});

test("a notification with a title toasts, whatever its kind", () => {
  for (const kind of ["agent", "review", "checks", "watch", "merge", "something-new"]) {
    expect(shouldToast({ title: "PR #42", kind }, true)).toBe(true);
  }
  expect(shouldToast({ title: "" }, true)).toBe(false);
  expect(shouldToast({ title: "PR #42" }, false)).toBe(false);
});

test("a kind the settings do not mute reaches the screen", () => {
  expect(hasKind("review", ["review", "checks"])).toBe(true);
  expect(hasKind("merge", ["review", "checks"])).toBe(false);
  expect(hasKind("agent", [])).toBe(false);
  expect(hasKind(undefined, ["review"])).toBe(false);
});

test("a switch mutes one kind and leaves the rest alone", () => {
  expect(withKind([], "checks", true)).toEqual(["checks"]);
  expect(withKind(["merge"], "review", true)).toEqual(["merge", "review"]);
  expect(withKind(["review", "checks"], "review", false)).toEqual(["checks"]);
  expect(withKind(["checks"], "checks", true)).toEqual(["checks"]);
});

test("a switch keeps a muted kind this build does not know", () => {
  expect(withKind(["rumour"], "checks", true)).toEqual(["rumour", "checks"]);
  expect(withKind(["checks", "rumour"], "checks", false)).toEqual(["rumour"]);
});

test.each([
  {
    name: "a window with the focus gets the banner and no call back on macOS",
    notification: { title: "PR #42", kind: "agent" },
    supported: true,
    platform: "darwin",
    focused: true,
    expected: { toast: true, bounce: null, flash: false },
  },
  {
    name: "a window with the focus gets the banner and no call back on Windows",
    notification: { title: "PR #42", kind: "merge" },
    supported: true,
    platform: "win32",
    focused: true,
    expected: { toast: true, bounce: null, flash: false },
  },
  {
    name: "macOS with no banner bounces nothing and flashes nothing",
    notification: { title: "PR #42", kind: "agent" },
    supported: false,
    platform: "darwin",
    focused: false,
    expected: { toast: false, bounce: null, flash: false },
  },
  {
    name: "Windows with no banner bounces nothing and flashes nothing",
    notification: { title: "PR #42", kind: "agent" },
    supported: false,
    platform: "win32",
    focused: false,
    expected: { toast: false, bounce: null, flash: false },
  },
  {
    name: "a banner of macOS for the agent keeps the dock bouncing",
    notification: { title: "PR #42", kind: "agent" },
    supported: true,
    platform: "darwin",
    focused: false,
    expected: { toast: true, bounce: "critical", flash: false },
  },
  {
    name: "a banner of macOS for a review bounces the dock once",
    notification: { title: "PR #42", kind: "review" },
    supported: true,
    platform: "darwin",
    focused: false,
    expected: { toast: true, bounce: "informational", flash: false },
  },
  {
    name: "the taskbar of Windows flashes for the agent, which waits on the user",
    notification: { title: "PR #42", kind: "agent" },
    supported: true,
    platform: "win32",
    focused: false,
    expected: { toast: true, bounce: null, flash: true },
  },
  {
    name: "the taskbar of Linux does not flash for a review, which does not wait on the user",
    notification: { title: "PR #42", kind: "review" },
    supported: true,
    platform: "linux",
    focused: false,
    expected: { toast: true, bounce: null, flash: false },
  },
])("$name", ({ notification, supported, platform, focused, expected }) => {
  expect(presentation(notification, supported, platform, focused)).toEqual(expected);
});

test("the test notification has a sound and no row of the history", () => {
  const notification = testNotification();

  expect(notification.silent).toBe(false);
  expect(notification).not.toHaveProperty("id");
  expect(notification).not.toHaveProperty("watchId");
});
