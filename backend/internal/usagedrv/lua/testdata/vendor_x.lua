local function bearer_token(ctx)
  if ctx.credential == nil then
    return ""
  end
  if ctx.credential.access_token ~= nil and ctx.credential.access_token ~= "" then
    return ctx.credential.access_token
  end
  if ctx.credential.api_key ~= nil and ctx.credential.api_key ~= "" then
    return ctx.credential.api_key
  end
  return ""
end

function fetch_usage(ctx)
  local endpoint = ctx.config.endpoint
  if endpoint == nil or endpoint == "" then
    return {
      ok = false,
      error = {
        kind = "config_error",
        message = "missing endpoint"
      }
    }
  end

  local token = bearer_token(ctx)
  local response = ctx.host.http_get({
    url = endpoint,
    headers = {
      Authorization = "Bearer " .. token
    }
  })

  if response.status ~= 200 then
    return {
      ok = false,
      error = {
        kind = "upstream_http_error",
        message = "status " .. tostring(response.status)
      }
    }
  end

  local payload = ctx.host.json_decode(response.body)

  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {
      quota_remaining = payload.quota_remaining,
      rpm_remaining = payload.rpm_remaining,
      tpm_remaining = payload.tpm_remaining
    },
    meta = {
      account_id = ctx.account.id,
      provider_type = ctx.account.provider_type
    },
    payload = payload
  }
end
