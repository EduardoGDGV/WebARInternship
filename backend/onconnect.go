/*
   !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
   NOTICE: This file will be heavily changed for the new group assignment functionality.

   Right now: Assign players dinamically to groups on connect, balancing group sizes.
   Future: Assign players to fixed groups based on manual assignment by admins.
   !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
*/

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"sync"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

const (
	GroupNamePrefix = "Group"
	MaxGroups       = 80
	LockRetryDelay     = 100 * time.Millisecond
	LockRetryAttempts  = 5
	MatchJoinDelay     = 200 * time.Millisecond // small delay before server-side MatchJoin to avoid races
	BroadcastTickDelta = 2 * time.Second
	LockCollection  = "locks"
	JoinLockKey     = "join_lock"
	GroupSizeKey    = "max_group_size"
	NextGroupKey    = "next_group"
	StreamMode      = 2
	AdminID         = "5c6f4519-0ba6-4fd2-b26d-f3639c3bf1e3"
)

// Read/Write in storage

func readInt(nk runtime.NakamaModule, key string, defaultVal int) int {
	records, err := nk.StorageRead(context.Background(), []*runtime.StorageRead{{
		Collection: LockCollection,
		Key:        key,
		UserID:     "",
	}})
	if err != nil || len(records) == 0 {
		return defaultVal
	}

	var val map[string]int
	if err := json.Unmarshal([]byte(records[0].Value), &val); err != nil {
		return defaultVal
	}
	return val["value"]
}

func writeInt(nk runtime.NakamaModule, key string, value int) {
	val, _ := json.Marshal(map[string]int{"value": value})
	_, err := nk.StorageWrite(context.Background(), []*runtime.StorageWrite{{
		Collection:      LockCollection,
		Key:             key,
		Value:           string(val),
		UserID:          "",
		PermissionRead:  2,
		PermissionWrite: 2,
	}})
	if err != nil {
		fmt.Printf("Failed to write %s: %v\n", key, err)
	}
}

// Lock Helpers

func acquireLock(nk runtime.NakamaModule, key string) bool {
	for attempt := 0; attempt < LockRetryAttempts; attempt++ {
		// Try to read current lock state
		records, err := nk.StorageRead(context.Background(), []*runtime.StorageRead{
			{
				Collection: LockCollection,
				Key:        key,
				UserID:     "",
			},
		})
		if err != nil {
			return false
		}

		// If record exists and is locked, retry after delay
		if len(records) > 0 && string(records[0].Value) == `{"locked":true}` {
			time.Sleep(LockRetryDelay)
			continue
		}

		// Otherwise, write lock = true
		val, _ := json.Marshal(map[string]bool{"locked": true})
		_, err = nk.StorageWrite(context.Background(), []*runtime.StorageWrite{
			{
				Collection:      LockCollection,
				Key:             key,
				Value:           string(val),
				UserID:          "",
				PermissionRead:  2, // public
				PermissionWrite: 2, // public
			},
		})
		if err == nil {
			return true
		}
		time.Sleep(LockRetryDelay)
	}
	return false
}

func releaseLock(nk runtime.NakamaModule, key string) {
	val, _ := json.Marshal(map[string]bool{"locked": false})
	_, _ = nk.StorageWrite(context.Background(), []*runtime.StorageWrite{
		{
			Collection:      LockCollection,
			Key:             key,
			Value:           string(val),
			UserID:          "",
			PermissionRead:  2, // public
			PermissionWrite: 2, // public
		},
	})
}

// Player Join

func handlePlayerJoin(ctx context.Context, nk runtime.NakamaModule, userID string, sessionID string, logger runtime.Logger) {
	// Acquire lock so only one joiner mutates shared state at a time
	if !acquireLock(nk, JoinLockKey) {
		logger.Error("Could not acquire join lock for user %s", userID)
		return
	}
	defer releaseLock(nk, JoinLockKey)

	// List all available groups
	maxmembers := 100
	open := true
	groups, _, err := nk.GroupsList(ctx, "", "", &maxmembers, &open, 80, "")
	if err != nil {
		logger.Error("Error fetching groups: %v", err)
		return
	}

	// If no groups exist, create them
	if len(groups) == 0 {
		logger.Info("No groups found, creating %d groups...", MaxGroups)
		for i := 1; i <= MaxGroups; i++ {
			name := fmt.Sprintf("%s_%d", GroupNamePrefix, i)
			_, err := nk.GroupCreate(ctx, AdminID, name, "", "", "", "", true, map[string]interface{}{}, 100)
			if err != nil {
				logger.Error("Failed to create group %s: %v", name, err)
				// continue trying others
			}
		}
		// refresh list
		groups, _, err = nk.GroupsList(ctx, "", "", &maxmembers, &open, MaxGroups, "")
		if err != nil || len(groups) == 0 {
			logger.Error("Failed to list groups after creation: %v", err)
			return
		}
	}

	// Load state from storage
	maxGroupSize := readInt(nk, GroupSizeKey, 6)
	nextGroup := readInt(nk, NextGroupKey, 0)

	logger.Info("Loaded MaxGroupSize=%d, NextGroup=%d from storage", maxGroupSize, nextGroup)

	// Look at current group occupancy
	memberState := 2 // member
	members, _, _ := nk.GroupUsersList(ctx, groups[nextGroup].Id, 100, &memberState, "")

	if len(members)+1 > maxGroupSize {
		// Increase capacity proportionally
		maxGroupSize = maxGroupSize + (nextGroup+1)/MaxGroups
		writeInt(nk, GroupSizeKey, maxGroupSize)

		// Move to next group (round-robin)
		nextGroup = (nextGroup + 1) % MaxGroups
		writeInt(nk, NextGroupKey, nextGroup)
	}

	// Add player to chosen group
	if err := nk.GroupUsersAdd(ctx, "", groups[nextGroup].Id, []string{userID}); err != nil {
		logger.Error("Failed to add user %s to group %s: %v", userID, groups[nextGroup].Name, err)
		return
	}

	// Join stream for that group
	if _, err := nk.StreamUserJoin(StreamMode, "", "", groups[nextGroup].Name, userID, sessionID, false, false, ""); err != nil {
		logger.Error("Failed stream join for user %s: %v", userID, err)
	}

	groupdata := map[string]any{
		"group": map[string]any{
			"id":   groups[nextGroup].Id,
			"name": groups[nextGroup].Name,
		},
	}

	if err := nk.AccountUpdateId(ctx, userID, "", groupdata, "", "", "", "", ""); err != nil {
		logger.WithField("err", err).Error("Account update error.")
	}
}

// Global group matches for location updates distribution and management

type GlobalMatch struct{}

type Player struct {
	Presence  runtime.Presence
	SessionID string
	Position  Position
}

type MatchState struct {
	Debug     bool
	GroupName  string
	Players   map[string]*Player
	//Score     map[string]int //player scores
}

func (m *GlobalMatch) MatchInit(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, params map[string]interface{}) (interface{}, int, string) {
	groupName, _ := params["group_name"].(string)
	if groupName == "" {
		groupName = "unknown_group"
	}
	state := &MatchState{
		Debug:      true,
		GroupName:  groupName,
		Players:    make(map[string]*Player),
	}
	tickRate := 1
	label := fmt.Sprintf("match=%s", groupName)
	logger.Info("Initialized match for %s", groupName)
	return state, tickRate, label
}

func (m *GlobalMatch) MatchJoinAttempt(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presence runtime.Presence, metadata map[string]string,) (interface{}, bool, string) {
	// Allow all joins by default
	return state, true, ""
}

func (m *GlobalMatch) MatchJoin(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	for _, p := range presences {
		userID := p.GetUserId()
		sessionID := p.GetSessionId()
		s.Players[userID] = &Player{
			Presence:  p,
			SessionID: sessionID,
			Position:  Position{Lat: 0, Lon: 0},
		}
		logger.Info("%s joined %s", p.GetUsername(), s.GroupName)
	}
	return s
}

func (m *GlobalMatch) MatchLeave(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	for _, p := range presences {
		delete(s.Players, p.GetUserId())
		logger.Info("%s left %s", p.GetUsername(), s.GroupName)
	}
	return s
}

func (m *GlobalMatch) MatchLoop(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, messages []runtime.MatchData) interface{} {
	s, ok := state.(*MatchState)
    if !ok || s == nil {
        logger.Error("Invalid match state")
        return state
    }

    for _, msg := range messages {
        if msg.GetOpCode() != 1 {
            continue
        }

        userID := msg.GetUserId()
        player, ok := s.Players[userID]
        if !ok {
            logger.Warn("Message from unknown player: %s", userID)
            continue
        }

        var newPos Position
        if err := json.Unmarshal(msg.GetData(), &newPos); err != nil {
            logger.Warn("Invalid pos payload from %s: %v", userID, err)
            continue
        }

        // Sanity checks
        if newPos.Lat < -90 || newPos.Lat > 90 || newPos.Lon < -180 || newPos.Lon > 180 {
            logger.Warn("Out-of-bounds pos from %s: %+v", userID, newPos)
            continue
        }

        // Compare to previous
		if player.Position.Lat != 0 || player.Position.Lon != 0 {
        	prevPos := player.Position
			dLat := newPos.Lat - prevPos.Lat
			dLon := newPos.Lon - prevPos.Lon
			distance := dLat*dLat+dLon*dLon
			if distance > 0.0001 {
				logger.Warn("Invalid movement from %s: prev=%+v new=%+v", userID, prevPos, newPos)
				continue
			}
			if distance < 0.00001 {
				continue // negligible movement
			}
		}
        player.Position = newPos
        updatePlayerPosition(ctx, nk, userID, player.SessionID, newPos)
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

// Factory function for the match
func NewGlobalMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule) (runtime.Match, error) {
	return &GlobalMatch{}, nil
}

// Global in-memory map (only lasts while the server process is running)
var (
	groupMatches   = make(map[string]string)
	groupMatchesMu sync.RWMutex
)

// rpcGetMatch ensures a single match per group using runtime memory only.
func rpcGetMatch(ctx context.Context, nk runtime.NakamaModule, logger runtime.Logger) (string, error) {
	userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	groupName := getUserGroup(ctx, nk, userID)
	if groupName == "" {
		return "", fmt.Errorf("user %s has no group", userID)
	}

	groupMatchesMu.RLock()
	matchID, ok := groupMatches[groupName]
	groupMatchesMu.RUnlock()
	if ok {
		return matchID, nil
	}

	// not found, create new match
	matchParams := map[string]any{"group_name": groupName}
	newMatchID, err := nk.MatchCreate(ctx, "global_match", matchParams)
	if err != nil {
		return "", err
	}

	groupMatchesMu.Lock()
	groupMatches[groupName] = newMatchID
	groupMatchesMu.Unlock()

	logger.Info("Created new match for %s -> %s", groupName, newMatchID)
	return newMatchID, nil
}

// Init Module
func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {

	if err := initializer.RegisterMatch("global_match", NewGlobalMatch); err != nil {
		return err
	}

	if err := initializer.RegisterEventSessionStart(
		func(ctx context.Context, logger runtime.Logger, evt *api.Event) {
			userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
			sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
			groups, _, err := nk.UserGroupsList(ctx, userID, 1, nil, "")
			if err != nil {
				return
			}
			if len(groups) == 0 {
				handlePlayerJoin(ctx, nk, userID, sessionID, logger)
				groups, _, _ = nk.UserGroupsList(ctx, userID, 1, nil, "")
			}
			group := groups[0]
			userGroups[userID] = group.GetGroup().Name
			if _, err := nk.StreamUserJoin(StreamMode, "", "", group.GetGroup().Name, userID, sessionID, false, false, ""); err != nil {
				logger.Error("Failed stream join for user %s: %v", userID, err)
				return
			}
		},
	); err != nil {
		return err
	}

	if err := InitBuildings(ctx, logger, db, nk, initializer); err != nil {
		logger.Error("Failed to init buildings module: %v", err)
		return err
	}

	if err := initializer.RegisterRpc("update_position", func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
		return rpcUpdatePosition(ctx, nk, payload)
	}); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterRpc("get_match", func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
		return rpcGetMatch(ctx, nk, logger)
	}); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	logger.Info("On connection module loaded.")
	return nil
}
