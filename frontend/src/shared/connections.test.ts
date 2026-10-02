import { expect, test } from "vite-plus/test";
import { originOf, parsePairingLink, parsePairRequest } from "./connections";

test("a pairing link gives the origin of the daemon and the token in its fragment", () => {
  expect(parsePairingLink("http://studio.local:7420/#token=abc")).toEqual({
    url: "http://studio.local:7420",
    token: "abc",
  });
});

test("a token typed apart wins over the one of the link", () => {
  expect(parsePairingLink("http://studio.local:7420/#token=old", " new ")).toEqual({
    url: "http://studio.local:7420",
    token: "new",
  });
});

test("an address without a scheme or a port takes http and the default remote port", () => {
  expect(originOf("studio.local")).toBe("http://studio.local:7420");
  expect(originOf("https://babysitter.example.com")).toBe("https://babysitter.example.com");
});

test("an address without a token or with another scheme is no pairing target", () => {
  expect(parsePairingLink("http://studio.local:7420")).toBeNull();
  expect(parsePairingLink("ftp://studio.local/#token=abc")).toBeNull();
  expect(parsePairingLink("   ")).toBeNull();
});

test("a pair request from the renderer keeps only its text fields", () => {
  expect(parsePairRequest({ link: "studio.local", token: 3, name: "studio" })).toEqual({
    link: "studio.local",
    token: undefined,
    name: "studio",
  });
  expect(parsePairRequest(null)).toEqual({ link: "", token: undefined, name: undefined });
});
