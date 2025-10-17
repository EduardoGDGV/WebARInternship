package main

import (
	//"bytes"
	"context"
	//"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/heroiclabs/nakama-common/runtime"
)

// Compact binary position
type Position struct {
	Lat float32 `json:"lat"`
	Lon float32 `json:"lon"`
}

// Each stream cell covers ~100 meters
const CELL_SIZE float32 = 0.001

// Caches
var (
	cellLock    sync.RWMutex
	playerCells = make(map[string][]string) // playerID -> cells
	userGroups  = make(map[string]string)   // playerID -> group
)

// Helpers
func cellKey(lat, lon float32) string {
	return fmt.Sprintf("%.5f,%.5f", lat, lon)
}

func getCell(lat, lon float32) (float32, float32) {
	return float32(math.Floor(float64(lat/CELL_SIZE)) * float64(CELL_SIZE)),
		float32(math.Floor(float64(lon/CELL_SIZE)) * float64(CELL_SIZE))
}

func determineCells(lat, lon float32) []string {
	baseLat, baseLon := getCell(lat, lon)
	offsetLat := lat - baseLat
	offsetLon := lon - baseLon

	// First cell is always the base cell
	keys := []string{cellKey(baseLat, baseLon)}

	if offsetLat > 0 {
		keys = append(keys, cellKey(baseLat+CELL_SIZE, baseLon))
		baseLat += CELL_SIZE
	} else if offsetLat < 0 {
		keys = append(keys, cellKey(baseLat-CELL_SIZE, baseLon))
		baseLat -= CELL_SIZE
	}
	if offsetLon > 0 {
		keys = append(keys, cellKey(baseLat, baseLon+CELL_SIZE))
		baseLon += CELL_SIZE
	} else if offsetLon < 0 {
		keys = append(keys, cellKey(baseLat, baseLon-CELL_SIZE))
		baseLon -= CELL_SIZE
	}
	if offsetLat != 0 && offsetLon != 0 {
		keys = append(keys, cellKey(baseLat, baseLon))
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

	return ""
}

// EncodePosition packs a Position struct into 8 bytes
/*func EncodePosition(pos Position) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, pos.Lat); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pos.Lon); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}*/

// Core Update Function
func updatePlayerPosition(ctx context.Context, nk runtime.NakamaModule, pos Position) {
	userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

	newCells := determineCells(pos.Lat, pos.Lon)
	group := getUserGroup(ctx, nk, userID)

	cellLock.Lock()
	defer cellLock.Unlock()

	// Leave old cells
	oldCells := playerCells[userID]
	newCellsMap := make(map[string]struct{}, len(newCells))
	for _, c := range newCells {
		newCellsMap[c] = struct{}{}
	}
	for _, cell := range oldCells {
		if _, stillInNew := newCellsMap[cell]; !stillInNew {
			if err := nk.StreamUserLeave(StreamMode, "", "", cell, userID, sessionID); err != nil {
				fmt.Printf("Failed stream leave for user %s: %v\n", userID, err)
			}
		}
	}

	// Join new cells
	oldCellsMap := make(map[string]struct{}, len(oldCells))
	for _, c := range oldCells {
		oldCellsMap[c] = struct{}{}
	}

	for _, cell := range newCells {
		if _, alreadyJoined := oldCellsMap[cell]; !alreadyJoined {
			if _, err := nk.StreamUserJoin(StreamMode, "", "", cell, userID, sessionID, false, false, ""); err != nil {
				fmt.Printf("Failed stream join for user %s: %v\n", userID, err)
			}
		}
	}

	playerCells[userID] = newCells

	payload := struct {
		UserID string   `json:"UserID"`
		Pos    Position `json:"Pos"`
	}{
		UserID: userID,
		Pos:    pos,
	}

	// Broadcast JSON position
	posJSON, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Failed to marshal position for user %s: %v\n", userID, err)
		return
	}

	realCell := newCells[0] // Real cell
	if err := nk.StreamSend(StreamMode, "", "", realCell, string(posJSON), nil, false); err != nil {
		fmt.Printf("Failed stream send for user %s: %v\n", userID, err)
	}

	// Broadcast to group if applicable
	if group != "" {
		if err := nk.StreamSend(StreamMode, "", "", group, string(posJSON), nil, false); err != nil {
			fmt.Printf("Failed stream send for user %s: %v\n", userID, err)
		}
	}
}

// RPC Handler
func rpcUpdatePosition(ctx context.Context, nk runtime.NakamaModule, payload string) (string, error) {
	var pos Position
	if err := json.Unmarshal([]byte(payload), &pos); err != nil {
		return "", runtime.NewError("invalid payload", 3)
	}

	if pos.Lat < -90 || pos.Lat > 90 || pos.Lon < -180 || pos.Lon > 180 {
		return "", runtime.NewError("invalid coordinates", 3)
	}

	updatePlayerPosition(ctx, nk, pos)
	return "ok", nil
}