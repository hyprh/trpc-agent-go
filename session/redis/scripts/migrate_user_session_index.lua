-- Migrate one hashidx session meta key into its per-user session index.
--
-- Usage:
--   redis-cli --eval migrate_user_session_index.lua <hashidx-meta-key> , dry-run
--   redis-cli --eval migrate_user_session_index.lua <hashidx-meta-key> , apply
--
-- The script reads a HashIdx session meta JSON value and writes:
--   [prefix:]hashidx:sessidx:<appName>:{userID} <sessionID> {"createdAt":"..."}
--
-- Return values:
--   1: index entry was added, or would be added in dry-run mode
--   0: index entry already existed
--  -1: meta key does not exist
--  -2: meta JSON is invalid or missing appName/userID/id

local meta_key = KEYS[1]
local dry_run = ARGV[1] == "dry-run"

local function prefix_of(key)
    local marker = "hashidx:meta:"
    local start_pos = string.find(key, marker, 1, true)
    if not start_pos then
        return nil
    end
    return string.sub(key, 1, start_pos - 1)
end

local function hash_tag(value)
    return "{" .. value .. "}"
end

local meta_json = redis.call("GET", meta_key)
if not meta_json then
    return -1
end

local ok, meta = pcall(cjson.decode, meta_json)
if not ok or type(meta) ~= "table" then
    return -2
end
if not meta.appName or not meta.userID or not meta.id then
    return -2
end

local prefix = prefix_of(meta_key)
if not prefix then
    return -2
end

local index_key = prefix .. "hashidx:sessidx:" .. meta.appName .. ":" .. hash_tag(meta.userID)
local created_at = meta.createdAt
if not created_at or created_at == cjson.null then
    created_at = ""
end

if dry_run then
    local exists = redis.call("HEXISTS", index_key, meta.id)
    if exists == 1 then
        return 0
    end
    return 1
end

return redis.call("HSETNX", index_key, meta.id, cjson.encode({ createdAt = created_at }))
