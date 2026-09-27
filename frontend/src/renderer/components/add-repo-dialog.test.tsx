import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildRepo } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { AddRepoDialog } from "./add-repo-dialog";

function renderDialog() {
  return renderWithProviders(<AddRepoDialog open enabled onOpenChange={vi.fn()} />);
}

test("does not allow adding a repository until its name is entered", async () => {
  renderDialog();
  const user = userEvent.setup();

  expect(screen.getByRole("button", { name: "Add" })).toBeDisabled();

  await user.type(screen.getByLabelText("Repository"), "octo/babysitter");

  expect(screen.getByRole("button", { name: "Add" })).toBeEnabled();
});

test("adds the entered repository and closes after the daemon accepts it", async () => {
  const onOpenChange = vi.fn();
  let requestBody: unknown;
  serveApi({ addedRepo: buildRepo() });
  server.use(
    http.post(apiUrl("/api/v1/repos"), async ({ request }) => {
      requestBody = await request.json();
      return HttpResponse.json(buildRepo());
    }),
  );
  const user = userEvent.setup();

  renderWithProviders(<AddRepoDialog open enabled onOpenChange={onOpenChange} />);

  await user.type(screen.getByLabelText("Repository"), "  octo/babysitter  ");
  await user.click(screen.getByRole("button", { name: "Add" }));

  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  expect(requestBody).toEqual({ fullName: "octo/babysitter" });
});

test("puts the cursor in the name field when it opens", () => {
  renderDialog();

  expect(screen.getByLabelText("Repository")).toHaveFocus();
});

test("opens again with an empty name and without the error of the last try", async () => {
  serveApi();
  server.use(
    http.post(apiUrl("/api/v1/repos"), () =>
      HttpResponse.json({ error: { code: "not_found", message: "repository not found" } }, { status: 404 }),
    ),
  );
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(<AddRepoDialog open enabled onOpenChange={vi.fn()} />);
  await user.type(screen.getByLabelText("Repository"), "octo/missing");
  await user.click(screen.getByRole("button", { name: "Add" }));
  expect(await screen.findByText("repository not found")).toBeVisible();

  rerender(<AddRepoDialog open={false} enabled onOpenChange={vi.fn()} />);
  rerender(<AddRepoDialog open enabled onOpenChange={vi.fn()} />);

  expect(await screen.findByLabelText("Repository")).toHaveValue("");
  expect(screen.queryByText("repository not found")).toBeNull();
});
