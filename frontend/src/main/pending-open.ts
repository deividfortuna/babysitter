type Deliver = (channel: string, payload?: unknown) => void;

type Pending = { channel: string; payload?: unknown };

export function openQueue() {
  let pending: Pending[] = [];
  let deliver: Deliver | null = null;

  function drain() {
    if (deliver === null) return;
    const waiting = pending;
    pending = [];
    for (const [index, { channel, payload }] of waiting.entries()) {
      try {
        deliver(channel, payload);
      } catch {
        deliver = null;
        pending = waiting.slice(index);
        return;
      }
    }
  }

  return {
    request(channel: string, payload?: unknown) {
      pending.push({ channel, payload });
      drain();
    },
    listening(to: Deliver) {
      deliver = to;
      drain();
    },
    gone() {
      deliver = null;
      pending = [];
    },
  };
}
