package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/heroiclabs/nakama-common/runtime"
)

func rpcJoinCell(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Decode payload JSON
    var data struct {
        Lat float64 `json:"lat"`
        Lon float64 `json:"lon"`
    }
    if err := json.Unmarshal([]byte(payload), &data); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }

    // Join the cell stream
    label := fmt.Sprintf("cell_%f_%f", data.Lat, data.Lon)
    if _, err := nk.StreamUserJoin(StreamMode, "", "", label, userID, sessionID, false, false, ""); err != nil {
        return "", fmt.Errorf("failed to join cell: %w", err)
    }

    return `{"ok":true}`, nil
}

func rpcLeaveCell(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Optional: decode payload if you want to specify a group/cell
    var data struct {
        Lat float64 `json:"lat,omitempty"`
        Lon float64 `json:"lon,omitempty"`
    }
    _ = json.Unmarshal([]byte(payload), &data) // ignore error if optional

    // Leave all cell streams if lat/lon provided
    if data.Lat != 0 || data.Lon != 0 {
        label := fmt.Sprintf("cell_%f_%f", data.Lat, data.Lon)
        _ = nk.StreamUserLeave(StreamMode, "", "", label, userID, sessionID)
    }
	return `{"ok":true}`, nil
}

func sendCellData(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)

    // Decode payload only to extract lat/lon for the cell label
    var payloadMap map[string]interface{}
    if err := json.Unmarshal([]byte(payload), &payloadMap); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }

    lat, okLat := payloadMap["lat"].(float64)
    lon, okLon := payloadMap["lon"].(float64)
    data, okData := payloadMap["data"]
    if !okLat || !okLon || !okData {
        return "", runtime.NewError("missing lat, lon, or data fields", 3)
    }

    // Construct the JSON object to send
    msgBytes, err := json.Marshal(map[string]interface{}{
        "user_id": userID,
        "data":    data,
		"group":   nil,
    })
    if err != nil {
        return "", fmt.Errorf("failed to marshal message: %w", err)
    }

    // Send to the cell stream
    cellLabel := fmt.Sprintf("cell_%f_%f", lat, lon)
    if err := nk.StreamSend(StreamMode, "", "", cellLabel, string(msgBytes), nil, true); err != nil {
        return "", fmt.Errorf("failed to send to cell stream: %w", err)
    }

    return `{"ok":true}`, nil
}

func sendGroupData(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)

    var payloadMap map[string]interface{}
    if err := json.Unmarshal([]byte(payload), &payloadMap); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }
    data, ok := payloadMap["data"]
    if !ok {
        return "", runtime.NewError("missing data field", 3)
    }
    groupVal, ok := payloadMap["group"]
    if !ok {
        return "", runtime.NewError("missing data field", 3)
    }

	group, ok := groupVal.(string)
    if !ok || group == "" {
        return "", runtime.NewError("group must be a string", 3)
    }

    msgBytes, _ := json.Marshal(map[string]interface{}{
        "user_id":    userID,
        "data":       data,
        "group":      group,
    })

    if err := nk.StreamSend(StreamMode, "", "", group, string(msgBytes), nil, true); err != nil {
        logger.WithField("group", group).WithField("err", err).Error("Failed to send to group stream")
        return "", err
    }

    return `{"ok":true}`, nil
}

func rpcSendLocation(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    if _, err := sendCellData(ctx, logger, db, nk, payload); err != nil {
		logger.WithField("err", err).Error("failed to send to cell stream")
		return "", err
	}
    if _, err := sendGroupData(ctx, logger, db, nk, payload); err != nil {
        logger.WithField("err", err).Error("failed to send to group stream")
		return "", err
    }

    return `{"ok":true}`, nil
}