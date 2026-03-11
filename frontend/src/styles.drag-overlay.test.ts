import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

describe("account drag overlay styles", () => {
  it("keeps the drag overlay minimal without rotation or fixed max width", () => {
    const stylesheet = readFileSync(resolve(import.meta.dirname, "styles.css"), "utf8");
    const ruleMatch = stylesheet.match(/\.account-drag-overlay\s*\{[^}]*\}/);

    expect(ruleMatch?.[0]).toBeTruthy();
    expect(ruleMatch?.[0]).not.toContain("rotate(");
    expect(ruleMatch?.[0]).not.toContain("width: min(100%, 960px)");
  });
});
