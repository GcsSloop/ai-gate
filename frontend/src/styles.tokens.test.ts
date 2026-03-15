import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const stylesPath = resolve(__dirname, "styles.css");
const css = readFileSync(stylesPath, "utf8");

describe("shared control geometry", () => {
  it("keeps menu-like controls aligned to the theme-mode control metrics", () => {
    expect(css).toContain("--menu-control-height: 32px;");
    expect(css).toContain("--menu-control-radius: 14px;");
    expect(css).toContain("--menu-control-font-size: 14px;");
    expect(css).toContain("--menu-control-font-weight: 400;");
    expect(css).toContain("--menu-control-padding-x: 15px;");
  });

  it("keeps the add button fixed-size so it cannot collapse in the top toolbar", () => {
    expect(css).toMatch(/\.global-add-button\s*\{[^}]*width:\s*var\(--menu-control-height\);/s);
    expect(css).toMatch(/\.global-add-button\s*\{[^}]*min-width:\s*var\(--menu-control-height\);/s);
    expect(css).toMatch(/\.global-add-button\s*\{[^}]*flex:\s*0\s+0\s+auto;/s);
    expect(css).toMatch(/\.global-add-button\s*\{[^}]*padding:\s*0;/s);
  });

  it("keeps account cards visually compact after the top menu was reduced", () => {
    expect(css).toMatch(/\.account-cards\s*\{[^}]*gap:\s*12px;/s);
    expect(css).toMatch(/\.account-card-surface\.ant-card\s*\.ant-card-body\s*\{[^}]*padding:\s*16px\s+18px;/s);
    expect(css).toMatch(/\.account-card-shell\s*\{[^}]*min-height:\s*60px;/s);
    expect(css).toMatch(/\.account-enable-button\s*\{[^}]*height:\s*32px;/s);
    expect(css).toMatch(/\.account-action-button\s*\{[^}]*width:\s*32px;[^}]*height:\s*32px;/s);
  });
});
