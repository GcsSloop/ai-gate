function fetch_usage(ctx)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {
      quota_remaining = 120000
    },
    meta = {
      provider = ctx.account.provider_type
    }
  }
end
