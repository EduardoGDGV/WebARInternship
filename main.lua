local nk = require("nakama")

-- Mode 2 is safe for custom chat/game streams
local STREAM_MODE = 2

-- Join a cell stream
local function join_cell(context, payload)
    nk.logger_info("join_cell called with payload: " .. tostring(payload))

    local success, data = pcall(nk.json_decode, payload)
    if not success then
        nk.logger_error("Failed to decode payload in join_cell: " .. tostring(data))
        error("Invalid JSON payload")
    end

    if not data.lat or not data.lon then
        nk.logger_error("Missing lat or lon in join_cell payload")
        error("Missing lat or lon in payload")
    end

    local stream_id = { mode = STREAM_MODE, label = string.format("cell_%f_%f", data.lat, data.lon) }
    nk.stream_user_join(context.user_id, context.session_id, stream_id, false, false)

    return nk.json_encode({ ok = true })
end
nk.register_rpc(join_cell, "rpcJoinCell")

-- Leave a stream
local function leave_cell(context, payload)
    nk.logger_info("leave_cell called with payload: " .. tostring(payload))
    
    local success, json = pcall(nk.json_decode, payload)
    if not success then
        nk.logger_error("Failed to decode payload in leave_cell: " .. tostring(json))
        error("Invalid JSON payload")
    end

    local lat, lon = json.lat, json.lon
    if not lat or not lon then
        nk.logger_error("Missing lat or lon in leave_cell payload")
        error("Missing lat or lon in payload")
    end

    local stream_id = {
        mode = STREAM_MODE,
        label = string.format("cell_%f_%f", lat, lon)
    }

    nk.logger_info("Leaving stream: " .. stream_id.label .. " for user " .. context.user_id)
    nk.stream_user_leave(context.user_id, context.session_id, stream_id)

    return nk.json_encode({ status = "ok", stream = stream_id.label })
end
nk.register_rpc(leave_cell, "rpcLeaveCell")

-- Send data to a stream
local function send_cell_data(context, payload)
    nk.logger_info("send_cell_data called with payload: " .. tostring(payload))

    local success, json = pcall(nk.json_decode, payload)
    if not success then
        nk.logger_error("Failed to decode payload in send_cell_data: " .. tostring(json))
        error("Invalid JSON payload")
    end

    local lat, lon, data = json.lat, json.lon, json.data
    if not lat or not lon or data == nil then
        nk.logger_error("Missing fields in send_cell_data payload")
        error("Missing lat, lon, or data in payload")
    end

    local stream_id = {
        mode = STREAM_MODE,
        label = string.format("cell_%f_%f", lat, lon)
    }

    nk.logger_info("Sending data from " .. context.user_id .. " to stream " .. stream_id.label)
    nk.stream_send(stream_id, nk.json_encode({ user_id = context.user_id, data = data }))

    return nk.json_encode({ status = "ok" })
end
nk.register_rpc(send_cell_data, "rpcSendCellData")

-- Check if user is in a stream
local function check_cell_presence(context, payload)
    nk.logger_info("check_cell_presence called with payload: " .. tostring(payload))

    local success, json = pcall(nk.json_decode, payload)
    if not success then
        nk.logger_error("Failed to decode payload in check_cell_presence: " .. tostring(json))
        error("Invalid JSON payload")
    end

    local lat, lon = json.lat, json.lon
    if not lat or not lon then
        nk.logger_error("Missing lat or lon in check_cell_presence payload")
        error("Missing lat or lon in payload")
    end

    local stream_id = {
        mode = STREAM_MODE,
        label = string.format("cell_%f_%f", lat, lon)
    }

    local meta = nk.stream_user_get(context.user_id, context.session_id, stream_id)
    nk.logger_info("User " .. context.user_id .. " presence in stream " .. stream_id.label .. ": " .. tostring(meta ~= nil))

    return nk.json_encode({ present = meta ~= nil })
end
nk.register_rpc(check_cell_presence, "rpcCheckCell")
