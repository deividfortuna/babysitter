import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Button } from "@/components/ui/button";
import { cn } from "./utils";

test.each(["text-3xs", "text-2xs", "text-body", "text-title", "text-body/4.5"])(
  "keeps %s as a font size next to a text colour",
  (size) => {
    expect(cn(size, "text-muted-foreground")).toBe(`${size} text-muted-foreground`);
  },
);

test.each(["text-3xs", "text-2xs", "text-body", "text-title"])("%s replaces the font size of a component", (size) => {
  expect(cn("text-sm", size)).toBe(size);
});

test("a shadcn component takes a theme font size from its class name", () => {
  render(<Button className="text-body">Download</Button>);

  const button = screen.getByRole("button", { name: "Download" });
  expect(button).toHaveClass("text-body");
  expect(button).not.toHaveClass("text-sm");
});
