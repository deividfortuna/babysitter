import { expect, test, vi } from "vitest";
import { openQueue } from "./pending-open";

test("a request waits for the renderer that takes it", () => {
  const queue = openQueue();
  const deliver = vi.fn();

  queue.request("notifications:open");
  expect(deliver).not.toHaveBeenCalled();

  queue.listening(deliver);
  expect(deliver).toHaveBeenCalledWith("notifications:open", undefined);
});

test("a renderer that already listens takes the request at once", () => {
  const queue = openQueue();
  const deliver = vi.fn();
  queue.listening(deliver);

  queue.request("notifications:open");

  expect(deliver).toHaveBeenCalledWith("notifications:open", undefined);
});

test("a request is taken once", () => {
  const queue = openQueue();
  const deliver = vi.fn();

  queue.request("notifications:open");
  queue.listening(deliver);
  queue.listening(deliver);

  expect(deliver).toHaveBeenCalledTimes(1);
});

test("the window that goes holds the next request for the window that follows", () => {
  const queue = openQueue();
  const gone = vi.fn();
  const next = vi.fn();
  queue.listening(gone);
  queue.gone();

  queue.request("notifications:open");
  expect(gone).not.toHaveBeenCalled();

  queue.listening(next);
  expect(next).toHaveBeenCalledWith("notifications:open", undefined);
});

test("a request carries what the renderer needs with it", () => {
  const queue = openQueue();
  const deliver = vi.fn();

  queue.request("notifications:click", { id: 4, watchId: 42 });
  queue.listening(deliver);

  expect(deliver).toHaveBeenCalledWith("notifications:click", { id: 4, watchId: 42 });
});

test("the requests are taken in the order they came, so the last screen is the one that shows", () => {
  const queue = openQueue();
  const deliver = vi.fn();

  queue.request("notifications:open");
  queue.request("settings:open");
  queue.listening(deliver);

  expect(deliver).toHaveBeenCalledTimes(2);
  expect(deliver).toHaveBeenLastCalledWith("settings:open", undefined);
});

test("a request the window never took goes with that window", () => {
  const queue = openQueue();
  const next = vi.fn();

  queue.request("notifications:open");
  queue.gone();

  queue.listening(next);
  expect(next).not.toHaveBeenCalled();
});

test("every click that waits for the window reaches it, in order", () => {
  const queue = openQueue();
  const deliver = vi.fn();

  queue.request("notifications:click", { id: 4, watchId: 42 });
  queue.request("notifications:click", { id: 5, watchId: 43 });
  queue.listening(deliver);

  expect(deliver).toHaveBeenCalledTimes(2);
  expect(deliver).toHaveBeenNthCalledWith(1, "notifications:click", { id: 4, watchId: 42 });
  expect(deliver).toHaveBeenNthCalledWith(2, "notifications:click", { id: 5, watchId: 43 });
});

test("a renderer that cannot take a request keeps every request for the next one", () => {
  const queue = openQueue();
  const gone = vi.fn(() => {
    throw new Error("Object has been destroyed");
  });
  const next = vi.fn();
  queue.listening(gone);

  expect(() => queue.request("notifications:click", { id: 4, watchId: 42 })).not.toThrow();
  queue.request("notifications:click", { id: 5, watchId: 43 });
  expect(gone).toHaveBeenCalledTimes(1);

  queue.listening(next);
  expect(next).toHaveBeenCalledTimes(2);
  expect(next).toHaveBeenNthCalledWith(1, "notifications:click", { id: 4, watchId: 42 });
  expect(next).toHaveBeenNthCalledWith(2, "notifications:click", { id: 5, watchId: 43 });
});
