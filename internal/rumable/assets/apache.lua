-- Managed, immutable per deployment. Keep six bytes across buckets so an anchor
-- split across ordinary chunked responses is matched without buffering the page.
local includes = RUMABLE_INCLUDES
local excludes = RUMABLE_EXCLUDES
local tag = RUMABLE_TAG
local environment = RUMABLE_ENV

local function starts(value, prefix)
    return value:sub(1, #prefix) == prefix
end

local function selected(uri)
    for _, prefix in ipairs(excludes) do
        if starts(uri, prefix) then return false end
    end
    for _, prefix in ipairs(includes) do
        if starts(uri, prefix) then return true end
    end
    return false
end

function inject(r)
    local head = r.method == "HEAD"
    local mime = (r.content_type or ""):lower()
    local encoding = r.headers_out["Content-Encoding"] or r.err_headers_out["Content-Encoding"] or ""
    local disposition = r.headers_out["Content-Disposition"] or r.err_headers_out["Content-Disposition"] or ""
    if not selected(r.uri or "") or (r.method ~= "GET" and not head) or r.status ~= 200 or
       not (mime == "text/html" or starts(mime, "text/html;")) or
       (not head and encoding ~= "") or disposition:lower():find("attachment", 1, true) then return end

    -- Older mod_lua rejects nil header assignments. mod_headers clears both
    -- response header tables after this resource filter has marked the request.
    r.subprocess_env[environment] = "1"
    local pending, done = "", false
    coroutine.yield()
    while bucket ~= nil do
        local output = bucket
        if head then
            -- Static HEAD handlers send a temporary body through filters.
            -- Discard it so Apache cannot recompute the original GET length
            -- after mod_headers removes it; the HTTP layer emits no HEAD body.
            output = ""
        elseif not done then
            local data = pending .. bucket
            local first = data:lower():find("</head>", 1, true)
            if first then
                output = data:sub(1, first - 1) .. tag .. data:sub(first)
                pending, done = "", true
            else
                local size = math.max(0, #data - 6)
                output, pending = data:sub(1, size), data:sub(size + 1)
            end
        end
        coroutine.yield(output)
    end
    coroutine.yield(pending)
end
