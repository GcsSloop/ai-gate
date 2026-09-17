import type { AccountModelEntry } from "../../lib/api";

/**
 * Built-in fallback catalog, newest first. It mirrors the models bundled with the pinned Codex
 * reference (references/openai-codex, rust-v0.154.0, codex-rs/models-manager/models.json) and
 * keeps older slugs that third-party relays still serve.
 */
export const BUILT_IN_TEST_MODELS = [
  "gpt-6-astra",
  "gpt-5.6-sol",
  "gpt-5.6-terra",
  "gpt-5.6-luna",
  "gpt-5.5",
  "gpt-5.4",
  "gpt-5.4-mini",
  "gpt-5.3-codex",
  "gpt-5.2",
  "gpt-5.2-codex",
  "gpt-5.1",
  "gpt-5.1-codex-max",
  "gpt-5",
  "gpt-4.1",
];

function hasRedundantDisplayName(model: string, displayName: string): boolean {
  return displayName.trim().toLowerCase() === model.trim().toLowerCase();
}

/**
 * Picks the newest model an account actually offers, so the default follows the account's own
 * catalog instead of a hardcoded slug. Returns the fallback when nothing matches.
 */
export function pickPreferredTestModel(
  models: string[],
  fallback: string,
): string {
  const catalog = models.map((model) => model.trim()).filter(Boolean);
  if (catalog.length === 0) {
    return fallback;
  }

  const lowered = catalog.map((model) => model.toLowerCase());
  for (const preferred of BUILT_IN_TEST_MODELS) {
    const index = lowered.indexOf(preferred.toLowerCase());
    if (index >= 0) {
      return catalog[index];
    }
  }
  for (const preferred of BUILT_IN_TEST_MODELS) {
    const index = lowered.findIndex((model) =>
      model.startsWith(preferred.toLowerCase()),
    );
    if (index >= 0) {
      return catalog[index];
    }
  }
  return fallback;
}

export type TestModelOption = {
  value: string;
  label: string;
};

/**
 * Builds the picker options: the account's upstream/cached catalog first, then the built-in
 * suggestions that catalog does not already cover. The field still accepts free text.
 */
export function buildTestModelOptions(
  catalog: AccountModelEntry[],
): TestModelOption[] {
  const options: TestModelOption[] = [];
  const seen = new Set<string>();
  const push = (model: string, displayName?: string) => {
    const value = model.trim();
    if (value === "") {
      return;
    }
    const key = value.toLowerCase();
    if (seen.has(key)) {
      return;
    }
    seen.add(key);
    const label = (displayName ?? "").trim();
    const showLabel = label !== "" && !hasRedundantDisplayName(value, label);
    options.push({ value, label: showLabel ? `${value} · ${label}` : value });
  };

  catalog.forEach((entry) => push(entry.id, entry.display_name));
  BUILT_IN_TEST_MODELS.forEach((model) => push(model));
  return options;
}
