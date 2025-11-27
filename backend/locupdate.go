package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/heroiclabs/nakama-common/runtime"
)

// Compact binary position
type Position struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Each stream cell covers ~20 meters
const (
	CELL_SIZE             float64 = 0.0002
	TICK_RATE             int     = 20
	UPDATES_PER_SECOND    int     = 1
	BROADCASTS_PER_SECOND int     = 2
)

// Campus polygon bounds
var mapBounds = [][2]float64{
	{-23.557045162755653, -46.73422584856919},
	{-23.55147505044313, -46.73130018212596},
	{-23.554510643537427, -46.72538454895334},
	{-23.55966804391334, -46.72839059081891},
}

// Integer cell key
func cellIndices(lat, lon float64) (int, int) {
	return int(math.Floor(lat / CELL_SIZE)), int(math.Floor(lon / CELL_SIZE))
}

// Cell neighbors (including self)
func getNeighborCells(latIndex, lonIndex int) [][2]int {
	neighbors := make([][2]int, 0, 9)
	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			neighbors = append(neighbors, [2]int{latIndex + di, lonIndex + dj})
		}
	}
	return neighbors
}

// Ray casting to verify campus bounds
func pointInPolygon(lat, lon float64, poly [][2]float64) bool {
	inside := false
	n := len(poly)
	if n < 3 {
		return false
	}
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		intersect := ((yi > lon) != (yj > lon)) &&
			(lat < (xj-xi)*(lon-yi)/(yj-yi)+xi)
		if intersect {
			inside = !inside
		}
	}
	return inside
}

// Match structures
type GlobalMatch struct{}

type Player struct {
	ID         string
	Username   string
	GroupID    int
	GroupName  string
	Position   Position
	Presence   runtime.Presence
	lastUpdate int64 // unix ms
	// other fields (lastBroadcastTime, etc) to be added
}

// Data map sent to clients about joining/connected players
type UserData struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	GroupID   int    `json:"group_id"`
	GroupName string `json:"group_name"`
}

// update structure for location updates
type update struct {
	UserID string  `json:"user_id"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

type MatchState struct {
	Debug       bool
	Players     map[string]*Player          		// userID -> player
	playersCell map[string][2]int           		// userID -> {latIndex, lonIndex}
	cellPlayers map[int]map[int]map[string]struct{} // latIndex -> lonIndex -> userID
	// batched updates for broadcasting every second
	cellUpdates   map[int]map[int][]update
	groupUpdates  map[int][]update
	lastBroadcast int64 // unix ms
}

func distanceMeters(a, b Position) float64 {
	dLat := a.Lat - b.Lat
	dLon := a.Lon - b.Lon
	return math.Sqrt(dLat*dLat+dLon*dLon) * 111000 // meters approx
}

// Match lifecycle
func (m *GlobalMatch) MatchInit(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, params map[string]interface{}) (interface{}, int, string) {
	state := &MatchState{
		Debug:         true,
		Players:       make(map[string]*Player),
		playersCell:   make(map[string][2]int),
		cellPlayers:   make(map[int]map[int]map[string]struct{}),
		cellUpdates:   make(map[int]map[int][]update),
		groupUpdates:  make(map[int][]update),
		lastBroadcast: 0,
	}
	label := "global_match"
	logger.Info("Initialized match global_match.")
	//expected ~40 updates per tick = ~800 updates per second
	return state, TICK_RATE, label
}

func (m *GlobalMatch) MatchJoinAttempt(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presence runtime.Presence, metadata map[string]string) (interface{}, bool, string) {
	// Allow all joins by default
	return state, true, ""
}

func (m *GlobalMatch) MatchJoin(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)

	existing := make([]UserData, 0, len(s.Players))
	for _, pl := range s.Players {
		// Build list of existing players
		existing = append(existing, UserData{
			UserID:    pl.ID,
			Username:  pl.Username,
			GroupID:   pl.GroupID,
			GroupName: pl.GroupName,
		})
	}

	joins := make([]UserData, 0, len(presences))
	for _, p := range presences {
		userID := p.GetUserId()

		// prevent accidental duplicates
		if _, exists := s.Players[userID]; exists {
			logger.Warn("Ignoring duplicate join for user: " + userID)
			continue
		}

		username := p.GetUsername()
		groupID := groupManager.userToGroup[userID]
		groupName := groupManager.groups[groupID].Name
		s.Players[userID] = &Player{
			ID:         userID,
			Username:   username,
			GroupID:    groupID,
			GroupName:  groupName,
			Position:   Position{Lat: 0, Lon: 0}, // not known yet
			Presence:   p,
			lastUpdate: 0,
		}
		joins = append(joins, UserData{
			UserID:    userID,
			Username:  username,
			GroupID:   groupID,
			GroupName: groupName,
		})
		// no cell assigned until first valid position update
		logger.Info(fmt.Sprintf("%s joined.", p.GetUsername()))
	}

	// notify joining players of existing players
	if len(existing) > 0 {
		payload, _ := json.Marshal(existing)
		dispatcher.BroadcastMessage(10, payload, presences, nil, false)
	}

	// notify existing players of joining players
	if len(joins) > 0 {
		payload, _ := json.Marshal(joins)
		dispatcher.BroadcastMessage(10, payload, nil, nil, false)
	}
	return s
}

func (m *GlobalMatch) MatchLeave(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	for _, p := range presences {
		uid := p.GetUserId()
		// remove from nested cell map if present
		if idx, ok := s.playersCell[uid]; ok {
			li, lj := idx[0], idx[1]
			if row, ok1 := s.cellPlayers[li]; ok1 {
				if col, ok2 := row[lj]; ok2 {
					delete(col, uid)
				}
			}
			delete(s.playersCell, uid)
		}
		delete(s.Players, uid)
		logger.Info(fmt.Sprintf("%s left.", p.GetUsername()))
	}
	return s
}

// movePlayer ensures the player is located in the correct nested cell maps
func (s *MatchState) movePlayer(userID string, lat, lon float64) {
	latIdx, lonIdx := cellIndices(lat, lon)

	old, hadOld := s.playersCell[userID]

	// If moved or new
	if !hadOld || old[0] != latIdx || old[1] != lonIdx {
		// Remove from old cell
		if hadOld {
			oi, oj := old[0], old[1]
			if row, ok := s.cellPlayers[oi]; ok {
				if col, ok2 := row[oj]; ok2 {
					delete(col, userID)
				}
			}
		}

		// Add to new cell, creating nested maps as necessary
		row, ok := s.cellPlayers[latIdx]
		if !ok {
			row = make(map[int]map[string]struct{})
			s.cellPlayers[latIdx] = row
		}
		col, ok := row[lonIdx]
		if !ok {
			col = make(map[string]struct{})
			row[lonIdx] = col
		}
		col[userID] = struct{}{}
		s.playersCell[userID] = [2]int{latIdx, lonIdx}
		return
	}

	// No movement across cells, ensure pointer is present
	if row, ok := s.cellPlayers[latIdx]; ok {
		if col, ok2 := row[lonIdx]; ok2 {
			col[userID] = struct{}{}
		} else {
			ncol := make(map[string]struct{})
			ncol[userID] = struct{}{}
			row[lonIdx] = ncol
		}
	} else {
		row := make(map[int]map[string]struct{})
		col := make(map[string]struct{})
		col[userID] = struct{}{}
		row[lonIdx] = col
		s.cellPlayers[latIdx] = row
	}
}

// Process incoming position messages, validate and update internal state, then batch and send updates once per tick
func (m *GlobalMatch) MatchLoop(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, messages []runtime.MatchData) interface{} {
	s, ok := state.(*MatchState)
	if !ok || s == nil {
		logger.Error("Invalid match state")
		return state
	}

	// Aggregated updates per tick
	// Process incoming messages and populate buffers and spatial index
	for _, msg := range messages {
		if msg.GetOpCode() != 1 {
			continue
		}
		userID := msg.GetUserId()
		player, exists := s.Players[userID]
		if !exists {
			logger.Warn(fmt.Sprintf("Message from unknown player: %s", userID))
			continue
		}

		var newPos Position
		if err := json.Unmarshal(msg.GetData(), &newPos); err != nil {
			logger.Warn(fmt.Sprintf("Invalid pos payload from %s: %v", userID, err))
			continue
		}

		// Sanity checks
		if math.IsNaN(newPos.Lat) || math.IsNaN(newPos.Lon) || newPos.Lat < -90 || newPos.Lat > 90 || newPos.Lon < -180 || newPos.Lon > 180 {
			logger.Warn(fmt.Sprintf("Invalid pos values from %s: %+v", userID, newPos))
			continue
		}

		// Validation checks
		if player.Position.Lat != 0 || player.Position.Lon != 0 {
			prev := player.Position
			if tick-player.lastUpdate < int64(TICK_RATE/UPDATES_PER_SECOND) {
				continue // skip if last update is too recent
			}
			dist := distanceMeters(prev, newPos)
			if dist > 30 {
				logger.Info(fmt.Sprintf("Invalid movement from %s: prev=%+v new=%+v dist=%.1fm", userID, prev, newPos, dist))
				continue
			}
			if dist < 0.2 {
				// skip negligible movement
				continue
			}
		}
		// Campus bounds check
		if !pointInPolygon(newPos.Lat, newPos.Lon, mapBounds) {
			logger.Info(fmt.Sprintf("Pos outside campus from %s: %+v", userID, newPos))
			continue
		}

		// Accept update
		player.Position = newPos
		player.lastUpdate = tick
		// Move player between nested cell maps
		s.movePlayer(userID, newPos.Lat, newPos.Lon)
		latIdx, lonIdx := cellIndices(newPos.Lat, newPos.Lon)
		// append to cell updates
		up := update{UserID: userID, Lat: newPos.Lat, Lon: newPos.Lon}
		if _, ok := s.cellUpdates[latIdx]; !ok {
			s.cellUpdates[latIdx] = make(map[int][]update)
		}
		groupId := player.GroupID
		if _, ok := s.groupUpdates[groupId]; !ok {
			s.groupUpdates[groupId] = make([]update, 0)
		}
		s.cellUpdates[latIdx][lonIdx] = append(s.cellUpdates[latIdx][lonIdx], up)
		s.groupUpdates[groupId] = append(s.groupUpdates[groupId], up)
	}

	if tick-s.lastBroadcast < int64(TICK_RATE/BROADCASTS_PER_SECOND) {
		return s // broadcast at the rate of BROADCASTS_PER_SECOND
	}

	// Broadcast proximity updates cell-by-cell
	for latIdx, row := range s.cellUpdates {
		for lonIdx, updates := range row {
			// Serialize updates of THIS cell once
			payload, err := json.Marshal(updates)
			if err != nil {
				logger.Warn("Marshal cell update failed: " + err.Error())
				continue
			}

			// Iterate through 9 neighbor cells
			presences := make([]runtime.Presence, 0, 100)
			neighbors := getNeighborCells(latIdx, lonIdx)
			for _, c := range neighbors {
				ni, nj := c[0], c[1]
				// Get presences from the neighbor cell
				presMap, ok := s.cellPlayers[ni][nj]
				if !ok || len(presMap) == 0 {
					continue
				}
				// Append presences
				for userID := range presMap {
					presences = append(presences, s.Players[userID].Presence)
				}
			}
			// Broadcast to players of that neighbor cell
			dispatcher.BroadcastMessage(1, payload, presences, nil, false)
			s.cellUpdates[latIdx][lonIdx] = updates[:0] // clear updates
		}
	}

	// Broadcast group updates
	for groupID, updates := range s.groupUpdates {
		payload, err := json.Marshal(updates)
		if err != nil {
			logger.Warn("Marshal cell update failed: " + err.Error())
			continue
		}
		groupMembers := groupManager.groups[groupID].Members

		// Build slice of presences
		presences := make([]runtime.Presence, 0, len(groupMembers))
		for memberID := range groupMembers {
			if pl, ok := s.Players[memberID]; ok {
				presences = append(presences, pl.Presence)
			}
		}
		if len(presences) > 0 {
			dispatcher.BroadcastMessage(2, payload, presences, nil, false)
		}
		s.groupUpdates[groupID] = updates[:0] // clear updates
	}
	// Update last broadcast tick
	s.lastBroadcast = tick
	return s
}

func (m *GlobalMatch) MatchTerminate(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, graceSeconds int) interface{} {
	// Force this match to never terminate
	logger.Info("Global match attempted to terminate, ignoring.")
	return nil
}

func (m *GlobalMatch) MatchSignal(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, data string) (interface{}, string) {
	logger.Info("Signal received: %s", data)
	return state, ""
}

// Factory
func NewGlobalMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule) (runtime.Match, error) {
	return &GlobalMatch{}, nil
}

func rpcJoinGlobalMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	return globalMatchID, nil
}
