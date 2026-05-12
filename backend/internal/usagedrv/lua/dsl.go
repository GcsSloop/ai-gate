package lua

func builtInManagedScript(key string) (string, bool) {
	switch key {
	case "ai.nodeseek.in", "nodeseek.in":
		return `simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining", "quota.remaining", "balance"),
  unit = pick("unit", "quota.unit", default("USD")),
  valid = pick("is_active", "isValid", default(true))
})
`, true
	case "ppchat.vip", "code.ppchat.vip":
		return `usage_adapter({
  get = "https://his.ppchat.vip/api/token-logs?page=1&page_size=1&token_key={{api_key}}",

  limits = {
    quota_remaining = pick("data.token_info.remain_quota_display")
  },

  meta = {
    today_used_quota = pick("data.token_info.today_used_quota"),
    today_added_quota = pick("data.token_info.today_added_quota"),
    unit = "quota"
  }
})
`, true
	default:
		return "", false
	}
}

const luaDSLPrelude = `
local function aigate_path_get(root, path)
  if root == nil or path == nil or path == "" then
    return nil
  end
  local current = root
  for part in string.gmatch(path, "[^%.]+") do
    if type(current) ~= "table" then
      return nil
    end
    current = current[part]
    if current == nil then
      return nil
    end
  end
  return current
end

function default(value)
  return { __aigate_default = true, value = value }
end

function pick(...)
  local selector = { __aigate_pick = true, paths = {} }
  for i = 1, select("#", ...) do
    local value = select(i, ...)
    if type(value) == "table" and value.__aigate_default == true then
      selector.default = value.value
      selector.has_default = true
    elseif value ~= nil then
      table.insert(selector.paths, tostring(value))
    end
  end
  return selector
end

local function aigate_resolve(spec, payload)
  if type(spec) == "table" and spec.__aigate_pick == true then
    for _, path in ipairs(spec.paths) do
      local value = aigate_path_get(payload, path)
      if value ~= nil then
        return value
      end
    end
    if spec.has_default then
      return spec.default
    end
    return nil
  end
  if type(spec) == "function" then
    return spec(payload)
  end
  return spec
end

local function aigate_render_template(raw, ctx)
  local value = tostring(raw or "")
  local replacements = {
    base_url = ctx.account.base_url or "",
    baseUrl = ctx.account.base_url or "",
    api_key = ctx.credential.api_key or ctx.credential.access_token or "",
    apiKey = ctx.credential.api_key or ctx.credential.access_token or "",
    access_token = ctx.credential.access_token or ctx.credential.api_key or "",
    accessToken = ctx.credential.access_token or ctx.credential.api_key or ""
  }
  return (string.gsub(value, "{{%s*([%w_]+)%s*}}", function(name)
    local replacement = replacements[name]
    if replacement == nil then
      return ""
    end
    return tostring(replacement)
  end))
end

local function aigate_join_url(base_url, path)
  if path == nil or path == "" then
    return base_url
  end
  if string.match(path, "^https?://") then
    return path
  end
  local base = string.gsub(base_url or "", "/+$", "")
  local suffix = string.gsub(path, "^/+", "")
  if string.match(base, "/v1$") and string.match(suffix, "^v1/") then
    suffix = string.gsub(suffix, "^v1/", "")
  end
  return base .. "/" .. suffix
end

local function aigate_resolve_map(spec, payload)
  local result = {}
  if type(spec) ~= "table" then
    return result
  end
  for key, value in pairs(spec) do
    result[key] = aigate_resolve(value, payload)
  end
  return result
end

local function aigate_fetch_json(adapter, ctx)
  local request = adapter.request or {}
  local raw_url = adapter.get or request.url
  local url = aigate_render_template(raw_url, ctx)
  url = aigate_join_url(ctx.account.base_url, url)
  local method = string.upper(tostring(request.method or "GET"))
  if method ~= "GET" then
    return {
      ok = false,
      error = {
        kind = "config_error",
        message = "Lua usage DSL currently supports GET requests"
      }
    }
  end

  local headers = {}
  if type(request.headers) == "table" then
    for key, value in pairs(request.headers) do
      headers[key] = aigate_render_template(value, ctx)
    end
  end
  if adapter.bearer ~= nil then
    headers.Authorization = "Bearer " .. aigate_render_template(adapter.bearer, ctx)
  elseif adapter.auth == "bearer" then
    headers.Authorization = "Bearer " .. aigate_render_template("{{api_key}}", ctx)
  end

  local response = ctx.host.http_get({
    url = url,
    headers = headers
  })
  if response.status < 200 or response.status >= 300 then
    return {
      ok = false,
      error = {
        kind = "upstream_http_error",
        message = "status " .. tostring(response.status)
      }
    }
  end
  return ctx.host.json_decode(response.body)
end

local function aigate_execute_simple_usage(adapter, ctx, payload)
  local remaining = aigate_resolve(adapter.remaining or adapter.value, payload)
  local unit = aigate_resolve(adapter.unit, payload)
  local is_valid = aigate_resolve(adapter.valid, payload)
  if is_valid == nil then
    is_valid = true
  end
  if unit == nil then
    unit = "USD"
  end
  return {
    ok = true,
    source = adapter.source or "remote",
    confidence = adapter.confidence or "high",
    limits = {
      balance = remaining
    },
    meta = {
      unit = unit,
      is_valid = is_valid
    },
    payload = payload
  }
end

local function aigate_execute_usage_adapter(adapter, ctx)
  local payload = aigate_fetch_json(adapter, ctx)
  if type(payload) == "table" and payload.ok == false and payload.error ~= nil then
    return payload
  end
  if adapter.__aigate_simple_usage == true then
    return aigate_execute_simple_usage(adapter, ctx, payload)
  end

  local extracted = aigate_resolve_map(adapter.extract, payload)
  if type(adapter.result) == "function" then
    return adapter.result(extracted, payload, ctx)
  end

  return {
    ok = true,
    source = adapter.source or "remote",
    confidence = adapter.confidence or "high",
    limits = aigate_resolve_map(adapter.limits, payload),
    meta = aigate_resolve_map(adapter.meta, payload),
    payload = payload
  }
end

function usage_adapter(adapter)
  fetch_usage = function(ctx)
    return aigate_execute_usage_adapter(adapter, ctx)
  end
end

function simple_usage(adapter)
  adapter.__aigate_simple_usage = true
  usage_adapter(adapter)
end
`
