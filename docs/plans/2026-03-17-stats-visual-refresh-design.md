# Stats Visual Refresh Design

**Context**
- The current statistics page still uses a compact list-based trend view and a plain status list.
- The user wants the top summary to separate input and output token counts, remove quota change, replace the trend list with a dual-line chart, and replace the right-side status list with a model-share donut chart.
- The donut chart must reflect the full current filter window, not only the most recent 20 records already loaded for the event list.
- The frontend currently has no ECharts dependency, and the backend currently exposes summary, trends, and recent-events endpoints only.

**Decision**
- Keep the overall stats page layout intact, but refresh the summary row and both chart panels.
- Summary cards become: request count, input tokens, output tokens, estimated cost, and balance delta.
- Replace the list-based `Token 趋势` panel with an ECharts two-line chart for input and output tokens over time.
- Replace `状态分布` with `模型分布`, rendered as an ECharts donut chart using request-count share by model.
- Add a new backend endpoint that returns model distribution aggregated across the full filtered time window.

**Backend Scope**
- Extend the usage repository with a model-distribution aggregation method using the same `hours`, `account_id`, and `model` filter semantics as the other dashboard endpoints.
- Expose a new dashboard endpoint dedicated to model distribution so the frontend can render the donut chart from complete filtered data.

**Frontend Scope**
- Add ECharts as a frontend dependency.
- Introduce small chart wrappers rather than embedding chart setup directly inside `StatsPage`.
- Keep the page style restrained and modern: soft panel chrome, muted axes, two clearly distinct line colors, and a low-noise donut chart.
- Preserve the recent-events list below the charts.

**Non-Goals**
- Do not redesign the entire stats page layout.
- Do not add extra dashboard filters or drill-down behavior.
- Do not change the meaning of the existing filter controls.

**Testing**
- Add backend tests for the new model-distribution endpoint and repository aggregation.
- Update the stats page test to assert the new summary cards and chart sections.
- Keep the chart tests focused on rendered labels and chart containers rather than pixel output.
