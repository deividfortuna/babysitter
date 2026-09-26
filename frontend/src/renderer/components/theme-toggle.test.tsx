import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";
import { renderWithProviders } from "@test/test-utils";
import { ThemeToggle } from "./theme-toggle";

test("marks the preference in force", () => {
  window.localStorage.setItem("theme", "dark");

  renderWithProviders(<ThemeToggle />);

  expect(screen.getByRole("button", { name: "Dark theme" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("button", { name: "Light theme" })).toHaveAttribute("aria-pressed", "false");
  expect(screen.getByRole("button", { name: "Match the system" })).toHaveAttribute("aria-pressed", "false");
});

test("picking light drops the dark class and stores the choice", async () => {
  window.localStorage.setItem("theme", "dark");
  const user = userEvent.setup();

  renderWithProviders(<ThemeToggle />);
  await user.click(screen.getByRole("button", { name: "Light theme" }));

  expect(document.documentElement).not.toHaveClass("dark");
  expect(window.localStorage.getItem("theme")).toBe("light");
  expect(screen.getByRole("button", { name: "Light theme" })).toHaveAttribute("aria-pressed", "true");
});

test("picking the system hands the choice back to the colour scheme", async () => {
  window.localStorage.setItem("theme", "dark");
  const user = userEvent.setup();

  renderWithProviders(<ThemeToggle />);
  await user.click(screen.getByRole("button", { name: "Match the system" }));

  expect(document.documentElement).not.toHaveClass("dark");
  expect(window.localStorage.getItem("theme")).toBe("system");
});

test("carries an icon alone, with the label for assistive technology", () => {
  renderWithProviders(<ThemeToggle />);

  for (const label of ["Match the system", "Light theme", "Dark theme"]) {
    const button = screen.getByRole("button", { name: label });
    expect(button.textContent).toBe("");
    expect(button.querySelector("svg")).toBeInTheDocument();
  }
});
