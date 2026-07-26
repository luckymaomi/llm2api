package coordination

import "github.com/redis/go-redis/v9"

var acquireRateScript = redis.NewScript(`
local clock = redis.call('TIME')
local now_ms = (tonumber(clock[1]) * 1000) + math.floor(tonumber(clock[2]) / 1000)
local states = {}
local retry_after_ms = 0

for i = 1, #KEYS do
  local offset = ((i - 1) * 7)
  local capacity = tonumber(ARGV[offset + 1])
  local refill = tonumber(ARGV[offset + 2])
  local interval_ms = tonumber(ARGV[offset + 3])
  local requested = tonumber(ARGV[offset + 4])
  local idle_ttl_ms = tonumber(ARGV[offset + 5])
  local window_ms = tonumber(ARGV[offset + 6])
  local window_offset_ms = tonumber(ARGV[offset + 7])

  if window_ms > 0 then
    local raw = redis.call('HMGET', KEYS[i], 'tokens', 'window_index', 'capacity_tokens', 'window_ms', 'window_offset_ms')
    local has_state = raw[1] ~= false
    for j = 2, 5 do
      if (raw[j] ~= false) ~= has_state then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
    end

    local window_index = math.floor((now_ms - window_offset_ms) / window_ms)
    local until_reset_ms = math.max(1, ((window_index + 1) * window_ms) + window_offset_ms - now_ms)
    local tokens = capacity
    if has_state then
      local stored_tokens = tonumber(raw[1])
      local stored_window = tonumber(raw[2])
      local stored_capacity = tonumber(raw[3])
      local stored_window_ms = tonumber(raw[4])
      local stored_offset_ms = tonumber(raw[5])
      if not stored_tokens or not stored_window or not stored_capacity or not stored_window_ms or not stored_offset_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if stored_window > window_index then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if stored_window == window_index then
        if stored_capacity == capacity and stored_window_ms == window_ms and stored_offset_ms == window_offset_ms then
          tokens = math.min(capacity, math.max(0, stored_tokens))
        else
          tokens = 0
        end
      end
    end
    if tokens < requested then
      retry_after_ms = math.max(retry_after_ms, until_reset_ms)
    end
    states[i] = {tokens, window_index, requested, until_reset_ms, capacity, 1, window_ms, window_offset_ms}
  else
    local raw = redis.call('HMGET', KEYS[i], 'tokens', 'updated_ms')
    local has_tokens = raw[1] ~= false
    local has_updated = raw[2] ~= false
    if has_tokens ~= has_updated then
      return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
    end

    local tokens = capacity
    local updated_ms = now_ms
    local clock_delay_ms = 0
    if has_tokens then
      tokens = tonumber(raw[1])
      updated_ms = tonumber(raw[2])
      if not tokens or not updated_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if updated_ms < 0 or updated_ms > now_ms + idle_ttl_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      tokens = math.min(capacity, math.max(0, tokens))
      if now_ms >= updated_ms then
        tokens = math.min(capacity, tokens + ((now_ms - updated_ms) * refill / interval_ms))
        updated_ms = now_ms
      else
        clock_delay_ms = updated_ms - now_ms
      end
    end

    if tokens < requested then
      local wait_ms = clock_delay_ms + math.ceil((requested - tokens) * interval_ms / refill)
      retry_after_ms = math.max(retry_after_ms, math.max(1, wait_ms))
    end
    states[i] = {tokens, updated_ms, requested, idle_ttl_ms + clock_delay_ms, capacity, 0}
  end
end

local granted = retry_after_ms == 0
local result = {granted and 1 or 0, now_ms, retry_after_ms}
for i = 1, #KEYS do
  local state = states[i]
  local remaining = state[1]
  if granted then
    remaining = remaining - state[3]
  end
  if state[6] == 1 then
    redis.call('HSET', KEYS[i], 'tokens', tostring(remaining), 'window_index', tostring(state[2]), 'capacity_tokens', tostring(state[5]), 'window_ms', tostring(state[7]), 'window_offset_ms', tostring(state[8]))
  else
    redis.call('HSET', KEYS[i], 'tokens', tostring(remaining), 'updated_ms', tostring(state[2]))
  end
  redis.call('PEXPIRE', KEYS[i], state[4])
  table.insert(result, math.floor(math.max(0, remaining)))
end
return result
`)

var inspectRateScript = redis.NewScript(`
local clock = redis.call('TIME')
local now_ms = (tonumber(clock[1]) * 1000) + math.floor(tonumber(clock[2]) / 1000)
local result = {now_ms}

for i = 1, #KEYS do
  local offset = ((i - 1) * 7)
  local capacity = tonumber(ARGV[offset + 1])
  local refill = tonumber(ARGV[offset + 2])
  local interval_ms = tonumber(ARGV[offset + 3])
  local idle_ttl_ms = tonumber(ARGV[offset + 5])
  local window_ms = tonumber(ARGV[offset + 6])
  local window_offset_ms = tonumber(ARGV[offset + 7])

  if window_ms > 0 then
    local raw = redis.call('HMGET', KEYS[i], 'tokens', 'window_index', 'capacity_tokens', 'window_ms', 'window_offset_ms')
    local has_state = raw[1] ~= false
    for j = 2, 5 do
      if (raw[j] ~= false) ~= has_state then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
    end
    local window_index = math.floor((now_ms - window_offset_ms) / window_ms)
    local until_reset_ms = math.max(1, ((window_index + 1) * window_ms) + window_offset_ms - now_ms)
    local tokens = capacity
    if has_state then
      local stored_tokens = tonumber(raw[1])
      local stored_window = tonumber(raw[2])
      local stored_capacity = tonumber(raw[3])
      local stored_window_ms = tonumber(raw[4])
      local stored_offset_ms = tonumber(raw[5])
      if not stored_tokens or not stored_window or not stored_capacity or not stored_window_ms or not stored_offset_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if stored_window > window_index then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if stored_window == window_index then
        if stored_capacity == capacity and stored_window_ms == window_ms and stored_offset_ms == window_offset_ms then
          tokens = math.min(capacity, math.max(0, stored_tokens))
        else
          tokens = 0
        end
      end
    end
    redis.call('HSET', KEYS[i], 'tokens', tostring(tokens), 'window_index', tostring(window_index), 'capacity_tokens', tostring(capacity), 'window_ms', tostring(window_ms), 'window_offset_ms', tostring(window_offset_ms))
    redis.call('PEXPIRE', KEYS[i], until_reset_ms)
    table.insert(result, math.floor(math.max(0, tokens)))
  else
    local raw = redis.call('HMGET', KEYS[i], 'tokens', 'updated_ms')
    local has_tokens = raw[1] ~= false
    local has_updated = raw[2] ~= false
    if has_tokens ~= has_updated then
      return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
    end

    local tokens = capacity
    local updated_ms = now_ms
    local clock_delay_ms = 0
    if has_tokens then
      tokens = tonumber(raw[1])
      updated_ms = tonumber(raw[2])
      if not tokens or not updated_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if updated_ms < 0 or updated_ms > now_ms + idle_ttl_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      tokens = math.min(capacity, math.max(0, tokens))
      if now_ms >= updated_ms then
        tokens = math.min(capacity, tokens + ((now_ms - updated_ms) * refill / interval_ms))
        updated_ms = now_ms
      else
        clock_delay_ms = updated_ms - now_ms
      end
    end
    redis.call('HSET', KEYS[i], 'tokens', tostring(tokens), 'updated_ms', tostring(updated_ms))
    redis.call('PEXPIRE', KEYS[i], idle_ttl_ms + clock_delay_ms)
    table.insert(result, math.floor(math.max(0, tokens)))
  end
end
return result
`)

var refundRateScript = redis.NewScript(`
local clock = redis.call('TIME')
local now_ms = (tonumber(clock[1]) * 1000) + math.floor(tonumber(clock[2]) / 1000)
local marker_ttl_ms = tonumber(ARGV[1])
local already_applied = redis.call('EXISTS', KEYS[1]) == 1
local states = {}

for i = 2, #KEYS do
  local offset = 1 + ((i - 2) * 7)
  local capacity = tonumber(ARGV[offset + 1])
  local refill = tonumber(ARGV[offset + 2])
  local interval_ms = tonumber(ARGV[offset + 3])
  local refund = tonumber(ARGV[offset + 4])
  local idle_ttl_ms = tonumber(ARGV[offset + 5])
  local window_ms = tonumber(ARGV[offset + 6])
  local window_offset_ms = tonumber(ARGV[offset + 7])

  if window_ms > 0 then
    local raw = redis.call('HMGET', KEYS[i], 'tokens', 'window_index', 'capacity_tokens', 'window_ms', 'window_offset_ms')
    local has_state = raw[1] ~= false
    for j = 2, 5 do
      if (raw[j] ~= false) ~= has_state then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
    end
    local window_index = math.floor((now_ms - window_offset_ms) / window_ms)
    local until_reset_ms = math.max(1, ((window_index + 1) * window_ms) + window_offset_ms - now_ms)
    local tokens = capacity
    local can_refund = false
    if has_state then
      local stored_tokens = tonumber(raw[1])
      local stored_window = tonumber(raw[2])
      local stored_capacity = tonumber(raw[3])
      local stored_window_ms = tonumber(raw[4])
      local stored_offset_ms = tonumber(raw[5])
      if not stored_tokens or not stored_window or not stored_capacity or not stored_window_ms or not stored_offset_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if stored_window > window_index then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if stored_window == window_index and stored_capacity == capacity and stored_window_ms == window_ms and stored_offset_ms == window_offset_ms then
        tokens = math.min(capacity, math.max(0, stored_tokens))
        can_refund = true
      elseif stored_window == window_index then
        tokens = 0
      end
    end
    states[i - 1] = {tokens, window_index, refund, until_reset_ms, capacity, 1, window_ms, window_offset_ms, can_refund}
  else
    local raw = redis.call('HMGET', KEYS[i], 'tokens', 'updated_ms')
    local has_tokens = raw[1] ~= false
    local has_updated = raw[2] ~= false
    if has_tokens ~= has_updated then
      return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
    end

    local tokens = capacity
    local updated_ms = now_ms
    local clock_delay_ms = 0
    if has_tokens then
      tokens = tonumber(raw[1])
      updated_ms = tonumber(raw[2])
      if not tokens or not updated_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      if updated_ms < 0 or updated_ms > now_ms + idle_ttl_ms then
        return redis.error_reply('CORRUPT_COORDINATION_BUCKET')
      end
      tokens = math.min(capacity, math.max(0, tokens))
      if now_ms >= updated_ms then
        tokens = math.min(capacity, tokens + ((now_ms - updated_ms) * refill / interval_ms))
        updated_ms = now_ms
      else
        clock_delay_ms = updated_ms - now_ms
      end
    end
    states[i - 1] = {tokens, updated_ms, refund, idle_ttl_ms + clock_delay_ms, capacity, 0}
  end
end

local applied = 0
if not already_applied then
  local marked = redis.call('SET', KEYS[1], '1', 'NX', 'PX', marker_ttl_ms)
  if marked then
    applied = 1
  end
end

local result = {applied, now_ms}
for i = 2, #KEYS do
  local state = states[i - 1]
  local remaining = state[1]
  if applied == 1 then
    if state[6] == 1 then
      if state[9] then
        remaining = math.min(state[5], remaining + state[3])
      end
      redis.call('HSET', KEYS[i], 'tokens', tostring(remaining), 'window_index', tostring(state[2]), 'capacity_tokens', tostring(state[5]), 'window_ms', tostring(state[7]), 'window_offset_ms', tostring(state[8]))
    else
      remaining = math.min(state[5], remaining + state[3])
      redis.call('HSET', KEYS[i], 'tokens', tostring(remaining), 'updated_ms', tostring(state[2]))
    end
    redis.call('PEXPIRE', KEYS[i], state[4])
  end
  table.insert(result, math.floor(math.max(0, remaining)))
end
return result
`)
