function fetch_usage(ctx)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {},
    unexpected = "this key should be rejected"
  }
end
