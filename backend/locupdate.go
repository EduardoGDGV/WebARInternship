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
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Each stream cell covers ~40 meters
const CELL_SIZE float64 = 0.0004

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

func getCell(lat, lon float64) (float64 , float64) {
	return float64(math.Floor(float64(lat/CELL_SIZE)) * float64(CELL_SIZE)),
		float64(math.Floor(float64(lon/CELL_SIZE)) * float64(CELL_SIZE))
}

func determineCells(lat, lon float64) []string {
	baseLat, baseLon := getCell(lat, lon)
	centerLat := baseLat + CELL_SIZE/2
	centerLon := baseLon + CELL_SIZE/2
	offsetLat := lat - centerLat
	offsetLon := lon - centerLon

	// First cell is always the base cell
	keys := []string{cellKey(baseLat, baseLon)}

	if offsetLat > 0 {
		keys = append(keys, cellKey(baseLat+CELL_SIZE, baseLon))
	} else if offsetLat < 0 {
		keys = append(keys, cellKey(baseLat-CELL_SIZE, baseLon))
	}
	if offsetLon > 0 {
		keys = append(keys, cellKey(baseLat, baseLon+CELL_SIZE))
	} else if offsetLon < 0 {
		keys = append(keys, cellKey(baseLat, baseLon-CELL_SIZE))
	}
	if offsetLat != 0 && offsetLon != 0 {
		keys = append(keys, cellKey(baseLat+(math.Copysign(CELL_SIZE, offsetLat)), baseLon+(math.Copysign(CELL_SIZE, offsetLon))))
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
	}else{
		fmt.Println("Error fetching user groups for user ", userID, ": ", err)
	}

	return ""
}

// Core Update Function
func updatePlayerPosition(nk runtime.NakamaModule, userID, sessionID string, pos Position) {
	newCells := determineCells(pos.Lat, pos.Lon)

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
}