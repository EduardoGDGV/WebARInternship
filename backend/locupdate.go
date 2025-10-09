package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/heroiclabs/nakama-common/runtime"
)

const (
	CELL_SIZE  = 0.001 // ~100m per cell
)

type Position struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Group string  `json:"group,omitempty"`
}

type Player struct {
	UserID string   `json:"UserID"`
	Username string   `json:"Username"`
	Pos    Position `json:"Pos"`
}

// Caches
var (
	cellLock    sync.RWMutex
	playerCells = make(map[string][]string) // playerID -> cells
	userGroups  = make(map[string]string)   // playerID -> group
)

// Helpers
func cellKey(lat, lon float64) string {
	return fmt.Sprintf("%.5f,%.5f", lat, lon)
}

func getCell(lat, lon float64) (float64, float64) {
	return math.Floor(lat/CELL_SIZE) * CELL_SIZE,
		math.Floor(lon/CELL_SIZE) * CELL_SIZE
}

func determineCells(lat, lon float64) []string {
	baseLat, baseLon := getCell(lat, lon)
	offsetLat := lat - baseLat
	offsetLon := lon - baseLon

	keys := []string{cellKey(baseLat, baseLon)}

	if offsetLat > 0 {
		keys = append(keys, cellKey(baseLat+CELL_SIZE, baseLon))
	}
	if offsetLat < 0 {
		keys = append(keys, cellKey(baseLat-CELL_SIZE, baseLon))
	}
	if offsetLon > 0 {
		keys = append(keys, cellKey(baseLat, baseLon+CELL_SIZE))
	}
	if offsetLon < 0 {
		keys = append(keys, cellKey(baseLat, baseLon-CELL_SIZE))
	}
	if offsetLat != 0 && offsetLon != 0 {
		keys = append(keys, cellKey(baseLat+math.Copysign(CELL_SIZE, offsetLat),
			baseLon+math.Copysign(CELL_SIZE, offsetLon)))
	}
	return keys
}

func getUserGroup(ctx context.Context, nk runtime.NakamaModule, userID string) string {
	if g, ok := userGroups[userID]; ok {
		return g
	}

	// Fetch first group user belongs to
	groups, _, err := nk.UserGroupsList(ctx, userID, 1, nil, "")
	if err == nil && len(groups) > 0 {
		group := groups[0].Group.Name
		userGroups[userID] = group
		return group
	}

	// No group
	return ""
}

// Update player
func updatePlayerPosition(ctx context.Context, nk runtime.NakamaModule, pos Position) {
	userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	username, _ := ctx.Value(runtime.RUNTIME_CTX_USERNAME).(string)
	sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
	newCells := determineCells(pos.Lat, pos.Lon)

	group := getUserGroup(ctx, nk, userID)
	pos.Group = group

	cellLock.Lock()
	defer cellLock.Unlock()

	// leave old streams not in newCells
	oldCells := playerCells[userID]
	newCellsMap := make(map[string]struct{})
	for _, c := range newCells {
		newCellsMap[c] = struct{}{}
	}
	for _, cell := range oldCells {
		if _, stillInNew := newCellsMap[cell]; !stillInNew {
			if err := nk.StreamUserLeave(StreamMode, "", "", cell, userID, sessionID); err != nil {
				fmt.Printf("Failed stream leave for user %s: %v\n", userID, err)
			} else {
				leaveMap := map[string]any{"leave": userID}
				leaveUpdate, _ := json.Marshal(leaveMap)
				_ = nk.StreamSend(StreamMode, "", "", cell, string(leaveUpdate), nil, false)
			}
		}
	}

	// join new streams not already joined
	oldCellsMap := make(map[string]struct{})
	for _, c := range oldCells {
		oldCellsMap[c] = struct{}{}
	}
	for _, cell := range newCells {
		if _, alreadyJoined := oldCellsMap[cell]; !alreadyJoined {
			_, _ = nk.StreamUserJoin(StreamMode, "", "", cell, userID, sessionID, false, false, "")
			joinMap := map[string]any{"join": userID}
			joinUpdate, _ := json.Marshal(joinMap)
			_ = nk.StreamSend(StreamMode, "", "", cell, string(joinUpdate), nil, false)
		}
	}

	playerCells[userID] = newCells

	// broadcast update to cells
	playerUpdate, _ := json.Marshal(&Player{UserID: userID, Username: username, Pos: pos})
	for _, cell := range newCells {
		_ = nk.StreamSend(StreamMode, "", "", cell, string(playerUpdate), nil, false)
	}

	// broadcast to group stream
	if group != "" {
		_ = nk.StreamSend(StreamMode, "", "", group, string(playerUpdate), nil, false)
	}
}

// RPC handler
func rpcUpdatePosition(ctx context.Context, nk runtime.NakamaModule, payload string) (string, error) {
	var pos Position
	if err := json.Unmarshal([]byte(payload), &pos); err != nil {
		return "", runtime.NewError("invalid payload", 3)
	}

	updatePlayerPosition(ctx, nk, pos)

	return "ok", nil
}
