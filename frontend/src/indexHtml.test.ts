import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("frontend index.html", () => {
  it("references the colored 128px favicon", () => {
    const html = readFileSync(resolve(__dirname, "../index.html"), "utf8");

    expect(html).toContain('rel="icon"');
    expect(html).toContain('href="/aigate-icon.png"');
  });
});
