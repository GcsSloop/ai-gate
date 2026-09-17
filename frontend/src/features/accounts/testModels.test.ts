import { describe, expect, it } from "vitest";

import {
  BUILT_IN_TEST_MODELS,
  buildTestModelOptions,
  pickPreferredTestModel,
} from "./testModels";

describe("pickPreferredTestModel", () => {
  it("returns the fallback when the catalog is empty", () => {
    expect(pickPreferredTestModel([], "gpt-5.4")).toBe("gpt-5.4");
  });

  it("prefers the newest catalog entry", () => {
    expect(pickPreferredTestModel(["gpt-5.4", "gpt-6-astra"], "gpt-5.4")).toBe(
      "gpt-6-astra",
    );
    expect(
      pickPreferredTestModel(["gpt-5.6-terra", "gpt-5.6-sol"], "gpt-5.4"),
    ).toBe("gpt-5.6-sol");
  });

  it("matches exact slugs before prefixed variants", () => {
    expect(pickPreferredTestModel(["gpt-5.4-mini", "gpt-5.4"], "gpt-5.4")).toBe(
      "gpt-5.4",
    );
  });

  it("accepts dated variants of a preferred model", () => {
    expect(
      pickPreferredTestModel(["gpt-5.6-luna-2026-01-01"], "gpt-5.4"),
    ).toBe("gpt-5.6-luna-2026-01-01");
  });

  it("keeps the fallback when no catalog entry matches", () => {
    expect(pickPreferredTestModel(["claude-opus-4"], "gpt-5.4")).toBe("gpt-5.4");
  });
});

describe("buildTestModelOptions", () => {
  it("lists the account catalog before the built-in suggestions", () => {
    const options = buildTestModelOptions([{ id: "gpt-6-astra" }]);

    expect(options[0].value).toBe("gpt-6-astra");
    expect(options.map((option) => option.value)).toEqual([
      ...new Set(["gpt-6-astra", ...BUILT_IN_TEST_MODELS]),
    ]);
  });

  it("keeps distinct upstream display names and drops redundant ones", () => {
    const options = buildTestModelOptions([
      { id: "gpt-6-astra", display_name: "GPT-6-Astra" },
      { id: "vendor/model-x", display_name: "Vendor Model X" },
    ]);

    expect(options.find((option) => option.value === "gpt-6-astra")?.label).toBe(
      "gpt-6-astra",
    );
    expect(
      options.find((option) => option.value === "vendor/model-x")?.label,
    ).toBe("vendor/model-x · Vendor Model X");
  });

  it("still offers selectable models when the account catalog is empty", () => {
    expect(buildTestModelOptions([])).toHaveLength(BUILT_IN_TEST_MODELS.length);
  });
});
