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

test("a click opens the watch of the notification", () => {
  const url = "https://github.com/octo/hello/pull/42";
  expect(clickTarget({ id: 1, title: "t", body: "b", watchId: 7, url })).toEqual({ kind: "watch", watchId: 7 });
});

test("a click on a notification of no watch opens its pull request", () => {
  const url = "https://github.com/octo/hello/pull/42";
  expect(clickTarget({ id: 1, title: "t", body: "b", url })).toEqual({ kind: "url", url });
});

test("a click with nothing to open only brings the app up", () => {
  expect(clickTarget({ id: 1, title: "t", body: "b" })).toEqual({ kind: "app" });
  expect(clickTarget({ id: 1, title: "t", body: "b", url: "file:///etc/passwd" })).toEqual({ kind: "app" });
});

test("a click on the pull request of no watch goes to the browser, and the row is read", () => {
  const url = "https://github.com/octo/hello/pull/42";

  expect(clickPlan({ id: 9, title: "t", body: "b", url })).toEqual({
    browser: url,
    raise: false,
    click: { id: 9, watchId: undefined },
  });
});

test("a click on the row of a watch raises the app, and the row is read", () => {
  expect(clickPlan({ id: 9, title: "t", body: "b", watchId: 7 })).toEqual({
    browser: null,
    raise: true,
    click: { id: 9, watchId: 7 },
  });
});

test("a click with nothing to open raises the app, and the row is read", () => {
  expect(clickPlan({ id: 9, title: "t", body: "b" })).toEqual({
    browser: null,
    raise: true,
    click: { id: 9, watchId: undefined },
  });
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

test("a platform that shows no banner bounces nothing and flashes nothing", () => {
  expect(presentation({ title: "PR #42", kind: "agent" }, false, "darwin")).toEqual({
    toast: false,
    bounce: null,
    flash: false,
  });
  expect(presentation({ title: "PR #42", kind: "agent" }, false, "win32")).toEqual({
    toast: false,
    bounce: null,
    flash: false,
  });
});

test("a banner of macOS carries the bounce its kind asks for", () => {
  expect(presentation({ title: "PR #42", kind: "agent" }, true, "darwin")).toEqual({
    toast: true,
    bounce: "critical",
    flash: false,
  });
  expect(presentation({ title: "PR #42", kind: "review" }, true, "darwin")).toEqual({
    toast: true,
    bounce: "informational",
    flash: false,
  });
});

test("the taskbar of the other platforms flashes only for what waits on the user", () => {
  expect(presentation({ title: "PR #42", kind: "agent" }, true, "win32")).toEqual({
    toast: true,
    bounce: null,
    flash: true,
  });
  expect(presentation({ title: "PR #42", kind: "review" }, true, "linux")).toEqual({
    toast: true,
    bounce: null,
    flash: false,
  });
});

test("the test notification has a sound and no row of the history", () => {
  expect(testNotification()).toEqual({
    title: "babysitter",
    body: "A notification of the system looks like this.",
    silent: false,
  });
});
