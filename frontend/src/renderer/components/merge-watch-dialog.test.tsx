import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildWatch } from "@test/fixtures";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { MergeWatchDialog } from "./merge-watch-dialog";

test("opens again on the merge method of the watch and without the error of the last try", async () => {
  serveApi();
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/merge"), () =>
      HttpResponse.json({ error: { code: "not_ready", message: "the pull request changed" } }, { status: 409 }),
    ),
  );
  const watch = buildWatch({ id: 7, mergeMethod: "squash" });
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(
    <MergeWatchDialog open onOpenChange={vi.fn()} watch={watch} onMerged={vi.fn()} />,
  );

  await chooseOption(user, screen.getByLabelText("Merge method"), "Rebase");
  await user.click(screen.getByRole("button", { name: "Merge" }));
  expect(await screen.findByText("the pull request changed")).toBeVisible();

  rerender(<MergeWatchDialog open={false} onOpenChange={vi.fn()} watch={watch} onMerged={vi.fn()} />);
  rerender(<MergeWatchDialog open onOpenChange={vi.fn()} watch={watch} onMerged={vi.fn()} />);

  expect(await screen.findByLabelText("Merge method")).toHaveTextContent("Squash");
  expect(screen.queryByText("the pull request changed")).toBeNull();
});
