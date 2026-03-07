import { FormEvent, useEffect, useState } from "react";

import { apiPath } from "../../lib/paths";

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
    void fetch(apiPath("/policy/default"))
      .then((response) => response.json() as Promise<PolicyDefinition>)
      .then(setPolicy);
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await fetch(apiPath("/policy/default"), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(policy),
    });
  }

  return (
    <section className="panel">
      <h2>路由策略</h2>
      <form className="stack" onSubmit={(event) => void handleSubmit(event)}>
        <label>
          候选账户顺序
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
          Token 预算系数
          <input
            aria-label="Token 预算系数"
            value={policy.token_budget_factor}
            onChange={(event) =>
              setPolicy((current) => ({
                ...current,
                token_budget_factor: Number(event.target.value),
              }))
            }
          />
        </label>
        <button type="submit">保存策略</button>
      </form>
    </section>
  );
}
