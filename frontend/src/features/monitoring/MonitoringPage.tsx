import { useEffect, useState } from "react";

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
    void fetch("/monitoring/overview")
      .then((response) => response.json() as Promise<Overview>)
      .then(setOverview);
  }, []);

  return (
    <section className="panel">
      <h2>Monitoring</h2>
      <div className="metric-grid">
        <article className="metric-card">
          <span>Total balance</span>
          <strong>{overview.totals.balance}</strong>
        </article>
        <article className="metric-card">
          <span>Total quota</span>
          <strong>{overview.totals.quota}</strong>
        </article>
        <article className="metric-card">
          <span>Cooldown accounts</span>
          <strong>{overview.status_counts.cooldown ?? 0}</strong>
        </article>
      </div>
    </section>
  );
}
