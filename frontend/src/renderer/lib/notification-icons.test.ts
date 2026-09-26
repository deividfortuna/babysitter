import { expect, test } from "vitest";
import { NOTIFICATION_KINDS } from "../../shared/notifications";
import { KIND_ICON } from "./notification-icons";

test("every kind has an icon", () => {
  for (const kind of NOTIFICATION_KINDS) expect(KIND_ICON[kind]).toBeDefined();
});
