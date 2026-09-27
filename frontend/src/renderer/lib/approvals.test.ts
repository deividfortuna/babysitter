import { describe, expect, test } from "vite-plus/test";
import { approvalsField, approvalsInvalid, approvalsRequired, wholeNumber } from "./approvals";

describe("wholeNumber", () => {
  test("reads a whole number, and nothing else", () => {
    expect(wholeNumber(" 12 ")).toBe(12);
    expect(wholeNumber("-3")).toBe(-3);
    expect(wholeNumber("")).toBeUndefined();
    expect(wholeNumber("2.5")).toBeUndefined();
    expect(wholeNumber("two")).toBeUndefined();
  });
});

describe("approvalsRequired", () => {
  test("takes zero and above, and leaves an empty field to the caller", () => {
    expect(approvalsRequired("0")).toBe(0);
    expect(approvalsRequired(" 2 ")).toBe(2);
    expect(approvalsRequired("")).toBeUndefined();
    expect(approvalsRequired("-1")).toBeUndefined();
    expect(approvalsRequired("2.5")).toBeUndefined();
  });
});

describe("approvalsInvalid", () => {
  test("an empty field is a choice of its own, not a mistake", () => {
    expect(approvalsInvalid("")).toBe(false);
    expect(approvalsInvalid("  ")).toBe(false);
    expect(approvalsInvalid("2")).toBe(false);
    expect(approvalsInvalid("2.5")).toBe(true);
    expect(approvalsInvalid("-1")).toBe(true);
  });
});

describe("approvalsField", () => {
  test("null opens the field empty, which asks for the rule of the base branch", () => {
    expect(approvalsField(null)).toBe("");
    expect(approvalsField(undefined)).toBe("");
    expect(approvalsField(0)).toBe("0");
    expect(approvalsField(2)).toBe("2");
  });
});
