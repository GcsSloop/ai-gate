# Active Account First Routing Design

**Context**
- Current thin `/v1/responses` routing iterates candidates by persisted `priority DESC`.
- When auto failover is enabled and the current account is still healthy, requests can still start from the manually sorted first account instead of the current account.
- The same routing expectation should apply to all request entry points, including `/v1/responses` and `/v1/chat/completions`.
- The user does not want backend routing changes to mutate the homepage drag order or the stored `priority` values.

**Decision**
- Keep the persisted account order unchanged. `priority` continues to represent the user-managed manual order.
- During request routing only, if there is an active account, place that account at the front of the candidate list.
- Keep the remaining candidates in their existing manual order: `priority DESC`, then `id ASC`.
- If auto failover is disabled, continue to route only through the active account.
- If auto failover is enabled, try the active account first. Only fail over when that account is not usable for the request.
- After a successful failover, persist the newly used account as active so later requests keep preferring the account that actually works.

**Non-Goals**
- Do not rewrite stored account priorities.
- Do not change frontend account ordering or drag behavior.
- Do not change the meaning of the failover switch.

**Backend Impact**
- Introduce one shared helper that returns routing order as: active account first, then the rest by priority.
- Use the helper in both thin `/responses` handling and `/chat/completions` handling so the rule stays consistent across APIs.
- Continue to reject an active account that lacks required capabilities, such as `/responses` support in thin mode.

**Error Handling**
- A healthy active account remains first even if another account has higher persisted priority.
- Failover continues to happen only for the same error classes that already trigger it today.
- If there is no active account, routing falls back to the persisted manual order.

**Testing**
- Add a thin responses test proving an active but lower-priority account is attempted before a higher-priority account.
- Add a gateway test proving the same active-account-first rule for `/chat/completions`.
- Keep coverage proving successful failover still updates the active account.
