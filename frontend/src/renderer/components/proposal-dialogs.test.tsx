import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildWatch } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import { RejectProposalDialog } from "./proposal-dialogs";

test("the reject dialog opens again without the reason and the discard of the last time", async () => {
  const props = {
    onOpenChange: vi.fn(),
    watch: buildWatch(),
    number: 3,
    head: "abc1234def5678",
    pending: false,
    error: null,
    onConfirm: vi.fn(),
  };
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(<RejectProposalDialog open {...props} />);
  await user.type(screen.getByLabelText("Reason (optional)"), "try the other fix");
  await user.click(screen.getByLabelText("Discard the commits"));

  rerender(<RejectProposalDialog open={false} {...props} />);
  rerender(<RejectProposalDialog open {...props} />);

  expect(await screen.findByLabelText("Reason (optional)")).toHaveValue("");
  expect(screen.getByLabelText("Discard the commits")).not.toBeChecked();
});
