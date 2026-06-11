import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("frontend index.html", () => {
  it("references the colored 128px favicon", () => {
    const html = readFileSync(resolve(__dirname, "../index.html"), "utf8");

    expect(html).toContain('rel="icon"');
    expect(html).toContain('href="/aigate-icon.png"');
  });

  it("uses ai-gate webui base for server builds", () => {
    const config = readFileSync(resolve(__dirname, "../vite.config.ts"), "utf8");

    expect(config).toContain('mode === "server"');
    expect(config).toContain('"/ai-gate/webui/"');
    expect(config).toContain('"/ai-router/webui/"');
    expect(config).toContain('"./"');
  });
});
