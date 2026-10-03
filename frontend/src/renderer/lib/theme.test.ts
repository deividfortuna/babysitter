import { expect, test, vi } from "vite-plus/test";
import { stubMatchMedia } from "@test/test-utils";
import { applyTheme, readThemePreference, storeThemePreference, systemTheme, watchSystemTheme } from "./theme";

test("reads system when nothing is stored", () => {
  expect(readThemePreference()).toBe("system");
});

test("reads system when the stored value is not a preference", () => {
  window.localStorage.setItem("theme", "sepia");

  expect(readThemePreference()).toBe("system");
});

test("reads back a stored preference", () => {
  storeThemePreference("light");

  expect(readThemePreference()).toBe("light");
});

test("takes the system theme from the colour scheme query", () => {
  stubMatchMedia(true);

  expect(systemTheme()).toBe("dark");
});

test("puts the dark class on the root for dark only", () => {
  applyTheme("dark");
  expect(document.documentElement).toHaveClass("dark");

  applyTheme("light");
  expect(document.documentElement).not.toHaveClass("dark");
});

test("reports a system theme change and stops on unsubscribe", () => {
  const media = stubMatchMedia(false);
  const onChange = vi.fn();

  const stop = watchSystemTheme(onChange);
  media.set(true);

  expect(onChange).toHaveBeenCalled();
  expect(systemTheme()).toBe("dark");

  stop();
  expect(media.listenerCount).toBe(0);
});

test("paints the root canvas in the colour of the theme", () => {
  applyTheme("dark");
  expect(document.documentElement.style.backgroundColor).toBe("rgb(10, 10, 10)");

  applyTheme("light");
  expect(document.documentElement.style.backgroundColor).toBe("rgb(252, 252, 252)");
});
