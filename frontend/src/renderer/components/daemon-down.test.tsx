import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { bridge } from "@/lib/bridge";
import { renderWithProviders } from "@test/test-utils";
import { DaemonDown } from "./daemon-down";

afterEach(() => vi.restoreAllMocks());

test("explains the daemon error and starts it on request", async () => {
  const restart = vi.spyOn(bridge.daemon, "restart").mockResolvedValue();
  const user = userEvent.setup();

  renderWithProviders(
    <DaemonDown
      status={{
        state: "error",
        message: "The daemon exited unexpectedly.",
        details: "address already in use",
      }}
    />,
  );

  expect(screen.getByText("The daemon exited unexpectedly.")).toBeVisible();
  expect(screen.getByText("address already in use")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Start the daemon" }));

  expect(restart).toHaveBeenCalledOnce();
});

test("keeps a header to drag the window while the daemon starts again or is down", () => {
  const { unmount } = renderWithProviders(<DaemonDown status={{ state: "starting" }} />);
  expect(screen.getByRole("banner")).toBeInTheDocument();
  unmount();

  renderWithProviders(<DaemonDown status={{ state: "error", message: "The daemon exited unexpectedly." }} />);

  expect(screen.getByRole("banner")).toBeInTheDocument();
});
