import { useEffect, useState } from "react";

import { apiPath } from "../../lib/paths";

type Overview = {
  status_counts: Record<string, number>;
  totals: { balance: number; quota: number };
};

export function MonitoringPage() {
  const [overview, setOverview] = useState<Overview>({
    status_counts: {},
    totals: { balance: 0, quota: 0 },
  });

  useEffect(() => {
    void fetch(apiPath("/monitoring/overview"))
      .then((response) => response.json() as Promise<Overview>)
      .then(setOverview);
  }, []);

  return (
    <section className="panel">
      <h2>监控面板</h2>
      <div className="metric-grid">
        <article className="metric-card">
          <span>总余额</span>
          <strong>{overview.totals.balance}</strong>
        </article>
        <article className="metric-card">
          <span>总额度</span>
          <strong>{overview.totals.quota}</strong>
        </article>
        <article className="metric-card">
          <span>冷却中的账户</span>
          <strong>{overview.status_counts.cooldown ?? 0}</strong>
        </article>
      </div>
    </section>
  );
}
