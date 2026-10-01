import { readFileSync } from "node:fs";
import { mergeFeeds, parseFeed, renderFeed } from "./update-feed.ts";

const sources = process.argv.slice(2);

if (sources.length === 0) {
  console.error("usage: merge-feed.mjs <feed.yml>...");
  process.exit(2);
}

const feeds = sources.map((source) => parseFeed(readFileSync(source, "utf8"), source));

process.stdout.write(renderFeed(mergeFeeds(feeds)));
