package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/heroiclabs/nakama-common/runtime"
)

// Compact binary position
type Position struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Each stream cell covers ~40 meters
const CELL_SIZE float64 = 0.0004

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
func cellKeyFromIndices(i, j int) string { return fmt.Sprintf("cell_%d_%d", i, j) }
func cellKey(lat, lon float64) string {
	i, j := cellIndices(lat, lon)
	return cellKeyFromIndices(i, j)
}

// Given a cell key "cell_i_j" return i,j
func parseCellKey(key string) (int, int, error) {
	parts := strings.Split(key, "_")
	if len(parts) != 3 {
		return 0, 0, fmt.Errorf("bad key")
	}
	ii, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	jj, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, err
	}
	return ii, jj, nil
}

// Return neighbor cell keys (3x3 neighborhood)
func getNeighborsFromCellKey(key string) []string {
	i, j, err := parseCellKey(key)
	if err != nil {
		return nil
	}
	var keys []string
	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			keys = append(keys, cellKeyFromIndices(i+di, j+dj))
		}
	}
	return keys
}

// Integer cell indices for future improvements
func getNeighbors(lat, lon float64) []string {
	i, j := cellIndices(lat, lon)
	var keys []string
	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			keys = append(keys, cellKeyFromIndices(i+di, j+dj))
		}
	}
	return keys
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
	ID       string
	GroupID  string
	Presence runtime.Presence
	Position Position
	// other fields (lastBroadcastTime, etc) to be added
}

type MatchState struct {
	Debug       bool
	Players     map[string]*Player            // userID -> player
	cellPlayers map[string]map[string]*Player // cellKey -> userID -> *Player
	playersCell map[string]string             // userID -> cellKey
	// temporary buffers per tick, created locally in MatchLoop
}

func distanceMeters(a, b Position) float64 {
	dLat := a.Lat - b.Lat
	dLon := a.Lon - b.Lon
	return math.Sqrt(dLat*dLat+dLon*dLon) * 111000 // meters approx
}

// Match lifecycle
func (m *GlobalMatch) MatchInit(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, params map[string]interface{}) (interface{}, int, string) {
	state := &MatchState{
		Debug:       true,
		Players:     make(map[string]*Player),
		cellPlayers: make(map[string]map[string]*Player),
		playersCell: make(map[string]string),
	}
	tickRate := 1
	label := "global_match"
	logger.Info("Initialized global match.")
	return state, tickRate, label
}

func (m *GlobalMatch) MatchJoinAttempt(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presence runtime.Presence, metadata map[string]string) (interface{}, bool, string) {
	// Allow all joins by default
	return state, true, ""
}

func (m *GlobalMatch) MatchJoin(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	for _, p := range presences {
		userID := p.GetUserId()
		groupName, ok := groupManager.GetUserGroupName(nk, ctx, userID)
		if !ok {
			logger.Warn("User %s has no group assigned", userID)
			continue
		}
		s.Players[userID] = &Player{
			ID:       userID,
			GroupID:  groupName,
			Presence: p,
			Position: Position{Lat: 0, Lon: 0}, // not known yet
		}
		// no cell assigned until first valid position update
		logger.Info(fmt.Sprintf("%s joined.", p.GetUsername()))
	}
	return s
}

func (m *GlobalMatch) MatchLeave(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	for _, p := range presences {
		uid := p.GetUserId()
		// remove from cell index if present
		if ck, ok := s.playersCell[uid]; ok {
			if mp, ok2 := s.cellPlayers[ck]; ok2 {
				delete(mp, uid)
				// if cell empty, we can delete it
				if len(mp) == 0 {
					delete(s.cellPlayers, ck)
				}
			}
			delete(s.playersCell, uid)
		}
		delete(s.Players, uid)
		logger.Info(fmt.Sprintf("%s left.", p.GetUsername()))
	}
	return s
}

// Process incoming position messages, validate and update internal state, then batch and send updates once per tick
func (m *GlobalMatch) MatchLoop(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, messages []runtime.MatchData) interface{} {
	s, ok := state.(*MatchState)
	if !ok || s == nil {
		logger.Error("Invalid match state")
		return state
	}

	// Buffers to collect updates produced this tick
	type update struct {
		UserID  string  `json:"user_id"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		GroupID string  `json:"group_id"`
	}
	// per-group aggregated updates
	groupUpdates := make(map[string][]update)
	// per-cell aggregated updates (proximity sets)
	cellUpdates := make(map[string][]update)

	// Process incoming messages and populate buffers and spatial indices
	for _, msg := range messages {
		if msg.GetOpCode() != 1 {
			continue
		}
		userID := msg.GetUserId()
		player, okp := s.Players[userID]
		if !okp {
			logger.Warn(fmt.Sprintf("Message from unknown player: %s", userID))
			continue
		}
		var newPos Position
		if err := json.Unmarshal(msg.GetData(), &newPos); err != nil {
			logger.Warn(fmt.Sprintf("Invalid pos payload from %s: %v", userID, err))
			continue
		}
		// Sanity check for valid lat/lon
		if newPos.Lat < -90 || newPos.Lat > 90 || newPos.Lon < -180 || newPos.Lon > 180 {
			logger.Warn(fmt.Sprintf("Out-of-bounds pos from %s: %+v", userID, newPos))
			continue
		}
		// Must be inside campus polygon
		if !pointInPolygon(newPos.Lat, newPos.Lon, mapBounds) {
			logger.Warn(fmt.Sprintf("Pos outside campus from %s: %+v", userID, newPos))
			continue
		}

		// Compare with previous if available
		if player.Position.Lat != 0 || player.Position.Lon != 0 {
			prev := player.Position
			dist := distanceMeters(prev, newPos)
			if dist > 30 { // suspiciously large jump
				logger.Warn(fmt.Sprintf("Invalid movement from %s: prev=%+v new=%+v dist=%.1fm", userID, prev, newPos, dist))
				continue
			}
			if dist < 1 {
				// ignore negligible movement
				continue
			}
		}

		// update player position in memory
		player.Position = newPos

		// Update spatial index -> compute cell key and move player between cells if needed
		newCell := cellKey(newPos.Lat, newPos.Lon)
		oldCell, hadOld := s.playersCell[userID]
		if !hadOld || oldCell != newCell {
			// remove from old cell
			if hadOld {
				if mp, okc := s.cellPlayers[oldCell]; okc {
					delete(mp, userID)
					if len(mp) == 0 {
						delete(s.cellPlayers, oldCell)
					}
				}
			}
			// add to new cell
			mp, okc := s.cellPlayers[newCell]
			if !okc {
				mp = make(map[string]*Player)
				s.cellPlayers[newCell] = mp
			}
			mp[userID] = player
			s.playersCell[userID] = newCell
		} else {
			// already in cell, ensure map has pointer
			if mp, okc := s.cellPlayers[newCell]; okc {
				mp[userID] = player
			} else {
				mp := make(map[string]*Player)
				mp[userID] = player
				s.cellPlayers[newCell] = mp
			}
		}

		// Add update to group and cell buffers
		up := update{UserID: userID, Lat: newPos.Lat, Lon: newPos.Lon, GroupID: player.GroupID}
		groupUpdates[player.GroupID] = append(groupUpdates[player.GroupID], up)
		cellUpdates[newCell] = append(cellUpdates[newCell], up)
	}

	// For each group, send the array of updates to group's members in one message
	for groupID, updates := range groupUpdates {
		if len(updates) == 0 {
			continue
		}
		// Build recipient presences of group members currently in the match
		var presences []runtime.Presence
		for _, p := range s.Players {
			if p.GroupID == groupID {
				presences = append(presences, p.Presence)
			}
		}
		if len(presences) == 0 {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"scope":   "group",
			"updates": updates,
		})
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to marshal group updates for %s: %v", groupID, err))
			continue
		}
		dispatcher.BroadcastMessage(1, payload, presences, nil, false)
	}

	// For each recipient player, gather nearby updates from their neighboring cells
	for userID, recipient := range s.Players {
		// Determine recipient's current cell
		ck, ok := s.playersCell[userID]
		if !ok {
			// recipient has no known cell (no valid position sent yet)
			continue
		}
		neighborKeys := getNeighborsFromCellKey(ck)

		// Gather updates from neighbor cells, excluding those from same group
		var proxUpdates []update
		for _, nkc := range neighborKeys {
			if ups, ok := cellUpdates[nkc]; ok {
				for _, u := range ups {
					if u.GroupID == recipient.GroupID {
						// skip group updates, already delivered in group broadcast
						continue
					}
					// avoid sending recipient's own update
					if u.UserID == userID {
						continue
					}
					proxUpdates = append(proxUpdates, u)
				}
			}
		}
		if len(proxUpdates) == 0 {
			continue
		}

		payload, err := json.Marshal(map[string]any{
			"scope":   "nearby",
			"updates": proxUpdates,
		})
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to marshal prox updates for %s: %v", userID, err))
			continue
		}
		// send only to this player's presence (single recipient)
		dispatcher.BroadcastMessage(1, payload, []runtime.Presence{recipient.Presence}, nil, false)
	}

	return s
}

func (m *GlobalMatch) MatchTerminate(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, graceSeconds int) interface{} {
	logger.Info("Terminating match.")
	return state
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
	const matchLabel = "global_match"

	// List active global match
	matches, err := nk.MatchList(ctx, 1, true, matchLabel, nil, nil, "")
	if err != nil {
		logger.Error("Failed to list matches: ", err)
		return "", err
	}

	var matchID string
	if len(matches) > 0 {
		matchID = matches[0].MatchId
		logger.Info("Existing global match found: ", matchID)
	} else {
		// Create the global match if not found. We pass label in params so it can be seen on MatchList.
		newMatchID, err := nk.MatchCreate(ctx, "global_match", map[string]any{
			"label": matchLabel,
		})
		if err != nil {
			logger.Error("Failed to create global match: ", err)
			return "", err
		}
		matchID = newMatchID
		logger.Info("Created new global match: ", matchID)
	}

	resp, _ := json.Marshal(map[string]string{"match_id": matchID})
	return string(resp), nil
}
