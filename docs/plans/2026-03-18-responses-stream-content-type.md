# Responses Stream Content-Type Regression

## Context

On March 18, 2026, `/v1/responses` token statistics were missing in the dashboard even though request counts continued to grow.

The live OpenAI official upstream returned a valid SSE body with `response.completed.response.usage`, but the HTTP response did not include a reliable `Content-Type: text/event-stream` header. The router treated that response as a normal JSON body, skipped the SSE observer path, and persisted `0/0/0` tokens.

## Root Cause

The thin `/responses` path used `isEventStreamResponse(resp.Header)` as the only signal for stream parsing.

That assumption is unsafe for this upstream:

- request was `stream=true`
- body was SSE
- `Content-Type` was empty or fell back to plain text
- usage parsing never ran

## Permanent Rule

For `/responses`, when the client request is `stream=true`, prefer the SSE parsing path on successful upstream responses even if the upstream `Content-Type` is missing or incorrect.

Do not rely on `Content-Type` alone to decide whether stream telemetry should be observed.

## Guardrail

Keep a regression test that covers:

- `stream=true`
- SSE body with `response.completed.response.usage`
- missing or incorrect upstream `Content-Type`

Expected result:

- recent usage event stores non-zero `input_tokens`, `output_tokens`, and `total_tokens`

