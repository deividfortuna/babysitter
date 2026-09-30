import { expect, test } from "vite-plus/test";
import { avatarSource } from "./avatar";

test("uses the avatar GitHub sent, at the size of the row", () => {
  expect(avatarSource("dependabot", true, "https://avatars.githubusercontent.com/in/29110?v=4")).toBe(
    "https://avatars.githubusercontent.com/in/29110?v=4&s=36",
  );
  expect(avatarSource("octocat", false, "https://avatars.githubusercontent.com/u/583231?v=4")).toBe(
    "https://avatars.githubusercontent.com/u/583231?v=4&s=36",
  );
});

test("falls back to the profile image of a user when GitHub sent no avatar", () => {
  expect(avatarSource("octocat", false)).toBe("https://github.com/octocat.png?size=36");
});

test("gives a bot with no avatar no image, since its login has no profile image", () => {
  expect(avatarSource("dependabot", true)).toBeUndefined();
});
