import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vite-plus/test";
import { useDraftField } from "./use-draft-field";

function parse(text: string): number | undefined {
  const value = Number(text);
  return text.trim() !== "" && Number.isInteger(value) ? value : undefined;
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

function harness(value = 5) {
  const commit = vi.fn();
  const hook = renderHook((props: { value: number }) => useDraftField({ ...props, format: String, parse, commit }), {
    initialProps: { value },
  });
  return { commit, hook };
}

test("a valid value is committed once the typing stops", () => {
  const { commit, hook } = harness();

  act(() => hook.result.current.change("7"));
  act(() => {
    vi.advanceTimersByTime(300);
  });
  act(() => hook.result.current.change("72"));
  act(() => {
    vi.advanceTimersByTime(599);
  });
  expect(commit).not.toHaveBeenCalled();

  act(() => {
    vi.advanceTimersByTime(1);
  });
  expect(commit).toHaveBeenCalledExactlyOnceWith(72);
});

test("an invalid value is kept as text and never committed", () => {
  const { commit, hook } = harness();

  act(() => hook.result.current.change("x"));
  act(() => {
    vi.advanceTimersByTime(1000);
  });

  expect(hook.result.current.text).toBe("x");
  expect(hook.result.current.invalid).toBe(true);
  expect(commit).not.toHaveBeenCalled();
});

test("a flush commits at once", () => {
  const { commit, hook } = harness();

  act(() => hook.result.current.change("9"));
  act(() => hook.result.current.flush());

  expect(commit).toHaveBeenCalledExactlyOnceWith(9);
  act(() => {
    vi.advanceTimersByTime(1000);
  });
  expect(commit).toHaveBeenCalledOnce();
});

test("the value it already holds is not committed again", () => {
  const { commit, hook } = harness(5);

  act(() => hook.result.current.change("5"));
  act(() => hook.result.current.flush());

  expect(commit).not.toHaveBeenCalled();
});

test("a new value from outside replaces the text", () => {
  const { hook } = harness(5);

  hook.rerender({ value: 30 });

  expect(hook.result.current.text).toBe("30");
});

test("a new value from outside that matches the text keeps what was typed", () => {
  const { hook } = harness(5);

  act(() => hook.result.current.change("030"));
  hook.rerender({ value: 30 });

  expect(hook.result.current.text).toBe("030");
});

test("a new value from outside drops the value still waiting", () => {
  const { commit, hook } = harness(180);

  act(() => hook.result.current.change("90"));
  hook.rerender({ value: 60 });
  act(() => {
    vi.advanceTimersByTime(1000);
  });
  act(() => hook.result.current.flush());

  expect(hook.result.current.text).toBe("60");
  expect(commit).not.toHaveBeenCalled();
});

test("a value still waiting is committed when the field goes away", () => {
  const { commit, hook } = harness();

  act(() => hook.result.current.change("11"));
  hook.unmount();

  expect(commit).toHaveBeenCalledExactlyOnceWith(11);
});
