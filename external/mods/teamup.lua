--[[
    Team Up Bot Integration for Ikemen-GO

    This Lua mod captures match results and sends them to a relay server.
    NO API TOKENS OR SECRETS - this runs on the player's machine (untrusted).

    Installation:
    1. Copy this file to your Ikemen-GO's external/mods/ directory
    2. Configure the RELAY_URL below to point to your relay server
    3. Link your Ikemen player name to your Discord account via the relay server's web interface

    Security Model:
    - Both players must report the same result for it to count
    - The relay server validates and reconciles reports
    - API authentication happens server-side only
]]

-- ============================================================================
-- CONFIGURATION (users should edit this)
-- ============================================================================

local config = {
    -- URL of your Team Up relay server (set up by tournament organizer)
    RELAY_URL = "https://ikemen-relay.your-domain.workers.dev",

    -- Your community/guild identifier (provided by tournament organizer)
    COMMUNITY_ID = "YOUR_COMMUNITY_ID",

    -- Leaderboard name to record matches to
    LEADERBOARD = "ikemen",

    -- Enable debug logging
    DEBUG = false,

    -- Report timeout in seconds
    TIMEOUT = 10,
}

-- ============================================================================
-- INTERNAL STATE (do not edit)
-- ============================================================================

local state = {
    match_in_progress = false,
    match_id = nil,
    p1_name = nil,
    p2_name = nil,
    p1_character = nil,
    p2_character = nil,
    round_results = {},
    match_start_time = nil,
}

-- ============================================================================
-- UTILITY FUNCTIONS
-- ============================================================================

local function log(msg)
    if config.DEBUG then
        print("[TeamUp] " .. msg)
    end
end

local function generate_match_id()
    -- Generate a unique match ID based on timestamp and random
    return string.format("%d_%d", os.time(), math.random(100000, 999999))
end

-- Simple JSON encoder for basic types
local function encode_json(tbl)
    local function encode_value(val)
        local t = type(val)
        if t == "string" then
            -- Escape special characters
            val = val:gsub('\\', '\\\\')
            val = val:gsub('"', '\\"')
            val = val:gsub('\n', '\\n')
            val = val:gsub('\r', '\\r')
            val = val:gsub('\t', '\\t')
            return '"' .. val .. '"'
        elseif t == "number" then
            return tostring(val)
        elseif t == "boolean" then
            return val and "true" or "false"
        elseif t == "nil" then
            return "null"
        elseif t == "table" then
            -- Check if array or object
            local is_array = true
            local max_index = 0
            for k, _ in pairs(val) do
                if type(k) ~= "number" or k < 1 or math.floor(k) ~= k then
                    is_array = false
                    break
                end
                if k > max_index then max_index = k end
            end
            is_array = is_array and max_index == #val

            if is_array then
                local parts = {}
                for i, v in ipairs(val) do
                    parts[i] = encode_value(v)
                end
                return "[" .. table.concat(parts, ",") .. "]"
            else
                local parts = {}
                for k, v in pairs(val) do
                    table.insert(parts, '"' .. tostring(k) .. '":' .. encode_value(v))
                end
                return "{" .. table.concat(parts, ",") .. "}"
            end
        end
        return "null"
    end
    return encode_value(tbl)
end

-- HTTP POST using the built-in http library
local function do_http_post(url, data)
    local json_data = encode_json(data)

    log("Sending request to: " .. url)
    log("Payload: " .. json_data)

    local body, status = http.post(url, json_data, {
        headers = { ["Content-Type"] = "application/json" },
        timeout = config.TIMEOUT,
    })

    log("Response status: " .. tostring(status))
    log("Response body: " .. (body or ""))

    return {
        status = status or 0,
        body = body or "",
    }
end

-- ============================================================================
-- MATCH TRACKING
-- ============================================================================

local function on_match_start()
    log("Match starting...")

    state.match_in_progress = true
    state.match_id = generate_match_id()
    state.match_start_time = os.time()
    state.round_results = {}

    -- Get player names (these should be retrieved from Ikemen's player system)
    -- In actual implementation, use player(1):name() and player(2):name()
    state.p1_name = player and player(1) and player(1).name and player(1):name() or "Player1"
    state.p2_name = player and player(2) and player(2).name and player(2):name() or "Player2"

    -- Get character names if available
    state.p1_character = player and player(1) and player(1).charname and player(1):charname() or nil
    state.p2_character = player and player(2) and player(2).charname and player(2):charname() or nil

    log(string.format("Match ID: %s | %s vs %s", state.match_id, state.p1_name, state.p2_name))
end

local function on_round_end(winner)
    if not state.match_in_progress then return end

    table.insert(state.round_results, {
        winner = winner,  -- 1 or 2
        timestamp = os.time()
    })

    log(string.format("Round %d ended - Winner: Player %d", #state.round_results, winner))
end

local function on_match_end(winner_team)
    if not state.match_in_progress then return end

    log(string.format("Match ended - Winner: Player %d", winner_team))

    local match_duration = os.time() - state.match_start_time

    -- Count rounds won by each player
    local p1_rounds = 0
    local p2_rounds = 0
    for _, round in ipairs(state.round_results) do
        if round.winner == 1 then
            p1_rounds = p1_rounds + 1
        else
            p2_rounds = p2_rounds + 1
        end
    end

    -- Build match report
    local report = {
        match_id = state.match_id,
        community_id = config.COMMUNITY_ID,
        leaderboard = config.LEADERBOARD,
        timestamp = os.time(),
        duration_seconds = match_duration,

        -- Players
        player1 = {
            name = state.p1_name,
            character = state.p1_character,
            rounds_won = p1_rounds,
            placement = winner_team == 1 and 1 or 2
        },
        player2 = {
            name = state.p2_name,
            character = state.p2_character,
            rounds_won = p2_rounds,
            placement = winner_team == 2 and 1 or 2
        },

        -- Match info
        total_rounds = #state.round_results,
        winner = winner_team,

        -- Round-by-round results
        rounds = state.round_results,

        -- Client info (for debugging)
        client_version = main and main.version or "unknown",
    }

    -- Send report to relay server
    local url = config.RELAY_URL .. "/report"
    local response = do_http_post(url, report)

    if response.status == 200 or response.status == 202 then
        log("Match report sent successfully")
    elseif response.status == 409 then
        log("Match report conflict - waiting for other player")
    else
        log("Failed to send match report: HTTP " .. tostring(response.status))
    end

    -- Reset state
    state.match_in_progress = false
    state.match_id = nil
    state.round_results = {}
end

-- ============================================================================
-- IKEMEN-GO HOOKS
-- ============================================================================

-- Hook into the game loop to detect match state changes
if hook and hook.add then
    -- Track previous values to detect changes
    local prev_roundno = 0
    local prev_winteam = 0
    local match_started = false

    hook.add("loop", "teamup_match_tracker", function()
        -- Check if we're in a match
        if not roundno then return end

        local current_round = roundno()
        local current_winteam = winteam and winteam() or 0

        -- Detect match start (round 1 begins)
        if current_round == 1 and not match_started then
            match_started = true
            on_match_start()
        end

        -- Detect round end (winteam changes from 0 to 1 or 2)
        if prev_winteam == 0 and current_winteam > 0 then
            on_round_end(current_winteam)
        end

        -- Update previous values
        prev_roundno = current_round
        prev_winteam = current_winteam
    end)

    -- Hook into match end
    hook.add("launchFight", "teamup_match_end", function()
        -- This fires when returning from a match
        if match_started then
            -- Determine overall winner based on rounds
            local p1_wins = 0
            local p2_wins = 0
            for _, round in ipairs(state.round_results) do
                if round.winner == 1 then p1_wins = p1_wins + 1
                else p2_wins = p2_wins + 1 end
            end

            local winner = p1_wins > p2_wins and 1 or 2
            on_match_end(winner)
            match_started = false
        end
    end)

    log("Team Up integration loaded successfully!")
    log("Relay URL: " .. config.RELAY_URL)
    log("Community: " .. config.COMMUNITY_ID)
else
    print("[TeamUp] WARNING: Hook system not available. Integration disabled.")
end

-- ============================================================================
-- ALTERNATIVE: Manual reporting via console commands
-- ============================================================================

-- For testing or manual reporting, you can call these functions directly
-- from Ikemen's debug console:
--
--   teamup_report_match(1)  -- Report Player 1 as winner
--   teamup_report_match(2)  -- Report Player 2 as winner

function teamup_report_match(winner)
    if winner ~= 1 and winner ~= 2 then
        print("[TeamUp] Invalid winner. Use 1 or 2.")
        return
    end

    on_match_start()
    -- Simulate a 2-0 result
    on_round_end(winner)
    on_round_end(winner)
    on_match_end(winner)
end

function teamup_set_config(key, value)
    if config[key] ~= nil then
        config[key] = value
        print("[TeamUp] Set " .. key .. " = " .. tostring(value))
    else
        print("[TeamUp] Unknown config key: " .. key)
    end
end

function teamup_status()
    print("[TeamUp] Configuration:")
    print("  RELAY_URL: " .. config.RELAY_URL)
    print("  COMMUNITY_ID: " .. config.COMMUNITY_ID)
    print("  LEADERBOARD: " .. config.LEADERBOARD)
    print("  DEBUG: " .. tostring(config.DEBUG))
    print("")
    print("[TeamUp] State:")
    print("  match_in_progress: " .. tostring(state.match_in_progress))
    print("  match_id: " .. tostring(state.match_id))
end

return {
    config = config,
    report_match = teamup_report_match,
    set_config = teamup_set_config,
    status = teamup_status,
}
