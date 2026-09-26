import { createCn } from "cn/config";

export const cn = createCn({
  extend: { classGroups: { "font-size": [{ text: ["3xs", "2xs", "body", "title"] }] } },
});
