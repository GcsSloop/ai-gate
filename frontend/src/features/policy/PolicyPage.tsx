import { FormEvent, useEffect, useState } from "react";

type PolicyDefinition = {
  name: string;
  candidate_order: string[];
  minimum_balance_threshold: number;
  minimum_quota_threshold: number;
  token_budget_factor: number;
  model_pool_rules: Record<string, string[]>;
};

export function PolicyPage() {
  const [policy, setPolicy] = useState<PolicyDefinition>({
    name: "default",
    candidate_order: [],
    minimum_balance_threshold: 0,
    minimum_quota_threshold: 0,
    token_budget_factor: 1.3,
    model_pool_rules: {},
  });

  useEffect(() => {
    void fetch("/policy/default")
      .then((response) => response.json() as Promise<PolicyDefinition>)
      .then(setPolicy);
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await fetch("/policy/default", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(policy),
    });
  }

  return (
    <section className="panel">
      <h2>Routing policy</h2>
      <form className="stack" onSubmit={(event) => void handleSubmit(event)}>
        <label>
          Candidate order
          <input
            value={policy.candidate_order.join(",")}
            onChange={(event) =>
              setPolicy((current) => ({
                ...current,
                candidate_order: event.target.value.split(",").filter(Boolean),
              }))
            }
          />
        </label>
        <label>
          Token budget factor
          <input
            aria-label="Token budget factor"
            value={policy.token_budget_factor}
            onChange={(event) =>
              setPolicy((current) => ({
                ...current,
                token_budget_factor: Number(event.target.value),
              }))
            }
          />
        </label>
        <button type="submit">Save policy</button>
      </form>
    </section>
  );
}
