package edgeworker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/secrets"
)

func (s *Service) LuaDir() string {
	return filepath.Join(s.confDir, "lua")
}

func (s *Service) RuntimeLuaPath() string {
	return filepath.Join(s.LuaDir(), "edge_runtime.lua")
}

func (s *Service) InternalSecret() string {
	return secrets.LoadOrCreateEdgeWorkerSecret(s.dataDir)
}

func (s *Service) WriteRuntimeLua(panelPort int, apiPrefix string) error {
	_ = os.MkdirAll(s.LuaDir(), 0755)
	secret := s.InternalSecret()
	prefix := strings.Trim(apiPrefix, "/")
	if prefix != "" {
		prefix = "/" + prefix
	}
	body := generateEdgeRuntimeLua(panelPort, prefix, secret)
	return os.WriteFile(s.RuntimeLuaPath(), []byte(body), 0644)
}

func sharedDictName(nsID uint) string {
	return fmt.Sprintf("edge_kv_ns_%d", nsID)
}

func generateEdgeRuntimeLua(port int, apiPrefix, secret string) string {
	return fmt.Sprintf(`-- OWPanel edge_runtime — auto-generated, do not edit
local http = require "resty.http"
local cjson = require "cjson.safe"

local PANEL_PORT = %d
local API_PREFIX = %q
local WORKER_SECRET = %q

local _M = {}

local function internal_url(path)
    return "http://127.0.0.1:" .. PANEL_PORT .. API_PREFIX .. "/api/v1/edge-internal" .. path
end

local function http_call(method, path, body)
    local httpc = http.new()
    httpc:set_timeout(3000)
    local res, err = httpc:request_uri(internal_url(path), {
        method = method,
        body = body,
        headers = {
            ["Content-Type"] = "application/json",
            ["X-Edge-Worker-Secret"] = WORKER_SECRET,
        },
    })
    if not res then
        return nil, err
    end
    if res.status >= 400 then
        return nil, "edge internal api status " .. tostring(res.status)
    end
    return res.body, nil
end

function _M.kv(namespace_id)
    local dict_name = "edge_kv_ns_" .. tostring(namespace_id)
    local shdict = ngx.shared[dict_name]
    local ns = tostring(namespace_id)
    local api_base = "/kv/" .. ns .. "/"

    local kv = {}

    function kv:get(key)
        if shdict then
            local val = shdict:get(key)
            if val ~= nil then
                return val
            end
        end
        local body, err = http_call("GET", api_base .. ngx.escape_uri(key))
        if not body then
            return nil, err
        end
        local data = cjson.decode(body)
        if data and data.value ~= nil then
            if shdict then shdict:set(key, data.value, 300) end
            return data.value
        end
        return nil
    end

    function kv:put(key, val, ttl)
        ttl = ttl or 0
        local payload = cjson.encode({ value = val, ttl = ttl })
        local _, err = http_call("PUT", api_base .. ngx.escape_uri(key), payload)
        if err then return false, err end
        if shdict then
            if ttl and ttl > 0 then
                shdict:set(key, val, ttl)
            else
                shdict:set(key, val)
            end
        end
        return true
    end

    function kv:delete(key)
        local _, err = http_call("DELETE", api_base .. ngx.escape_uri(key))
        if err then return false, err end
        if shdict then shdict:delete(key) end
        return true
    end

    return kv
end

function _M.d1(db_id)
    local id = tostring(db_id)
    local db = {}

    function db:query(sql)
        local payload = cjson.encode({ sql = sql })
        local body, err = http_call("POST", "/d1/" .. id .. "/query", payload)
        if not body then return nil, err end
        return cjson.decode(body)
    end

    return db
end

function _M.redis(config_key)
    local ok, redis = pcall(require, "resty.redis")
    if not ok then
        return { get = function() return nil, "resty.redis not available" end }
    end
    local host, port, dbnum = "127.0.0.1", 6379, 0
    if config_key and config_key ~= "" then
        local parts = {}
        for p in string.gmatch(config_key, "[^:]+") do table.insert(parts, p) end
        if parts[1] then host = parts[1] end
        if parts[2] then port = tonumber(parts[2]) or 6379 end
        if parts[3] then dbnum = tonumber(parts[3]) or 0 end
    end
    local r = {}

    function r:get(key)
        local red = redis:new()
        red:set_timeout(1000)
        local ok_conn, err = red:connect(host, port)
        if not ok_conn then return nil, err end
        if dbnum > 0 then red:select(dbnum) end
        return red:get(key)
    end

    function r:set(key, val)
        local red = redis:new()
        red:set_timeout(1000)
        local ok_conn, err = red:connect(host, port)
        if not ok_conn then return false, err end
        if dbnum > 0 then red:select(dbnum) end
        return red:set(key, val)
    end

    return r
end

function _M.match_host(host, domains)
    if not domains or #domains == 0 then
        return true
    end
    host = string.lower(host or "")
    for _, d in ipairs(domains) do
        d = string.lower(d)
        if d == "*" or d == host then
            return true
        end
        if string.sub(d, 1, 2) == "*." then
            local suffix = string.sub(d, 2)
            if string.sub(host, -(#suffix)) == suffix then
                return true
            end
        end
    end
    return false
end

return _M
`, port, apiPrefix, secret)
}
