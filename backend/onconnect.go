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
	"math"
	"sync"
	"time"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

const (
	GroupNamePrefix    = "Group"
	MaxGroups          = 80
	LockRetryDelay     = 100 * time.Millisecond
	LockRetryAttempts  = 5
	MatchJoinDelay     = 200 * time.Millisecond // small delay before server-side MatchJoin to avoid races
	BroadcastTickDelta = 2 * time.Second
	LockCollection     = "locks"
	JoinLockKey        = "join_lock"
	GroupSizeKey       = "max_group_size"
	NextGroupKey       = "next_group"
	StreamMode         = 2
	AdminID            = "5c6f4519-0ba6-4fd2-b26d-f3639c3bf1e3"
)

// GroupManager implementation
type GroupInfo struct {
	ID      string
	Name    string
	Members map[string]struct{} // set of userIDs
}

type GroupManager struct {
	nk           runtime.NakamaModule
	logger       runtime.Logger
	groups       []*GroupInfo
	userToGroup  map[string]int // userID -> group index in groups slice
	nextTieIndex int            // tie-breaker for equal-size groups
	mu           sync.RWMutex
	maxGroups    int
	initialCap   int
}

func NewGroupManager(nk runtime.NakamaModule, logger runtime.Logger, maxGroups int, initialCap int) *GroupManager {
	return &GroupManager{
		nk:           nk,
		logger:       logger,
		groups:       make([]*GroupInfo, 0, maxGroups),
		userToGroup:  make(map[string]int),
		nextTieIndex: 0,
		maxGroups:    maxGroups,
		initialCap:   initialCap,
	}
}

// Initialize: list/create groups in Nakama and populate the in-memory groups array.
// Should be called at startup (InitModule).
func (gm *GroupManager) Init(ctx context.Context) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// load groups from Nakama
	maxmembers := 200
	open := true
	groups, _, err := gm.nk.GroupsList(ctx, "", "", &maxmembers, &open, gm.maxGroups, "")
	if err != nil {
		// report but try to continue if possible
		gm.logger.WithField("err", err).Error("GroupsList error")
		return err
	}

	// If not enough groups, create missing ones
	if len(groups) < gm.maxGroups {
		gm.logger.Info("Found %d groups, creating up to %d", len(groups), gm.maxGroups)
		existing := map[string]struct{}{}
		for _, g := range groups {
			existing[g.Name] = struct{}{}
		}
		for i := 1; i <= gm.maxGroups; i++ {
			name := fmt.Sprintf("%s_%d", GroupNamePrefix, i)
			if _, ok := existing[name]; ok {
				continue
			}
			if _, err := gm.nk.GroupCreate(ctx, AdminID, name, "", "", "", "", true, map[string]interface{}{}, 100); err != nil {
				gm.logger.WithField("group", name).WithField("err", err).Warn("Failed to create group (continuing)")
			}
		}
		// re-list
		groups, _, err = gm.nk.GroupsList(ctx, "", "", &maxmembers, &open, gm.maxGroups, "")
		if err != nil {
			gm.logger.WithField("err", err).Error("GroupsList failed after creation")
			return err
		}
	}

	// build local groups slice
	gm.groups = make([]*GroupInfo, 0, gm.maxGroups)
	for _, g := range groups {
		gi := &GroupInfo{
			ID:      g.Id,
			Name:    g.Name,
			Members: make(map[string]struct{}),
		}
		// Try to populate members set for faster decisions (best-effort)
		// We use GroupUsersList once per group (cheap at startup, up to 80 groups)
		memberState := 2 // member
		users, _, err := gm.nk.GroupUsersList(ctx, g.Id, 1000, &memberState, "")
		if err == nil {
			for _, u := range users {
				uid := u.GetUser().GetId()
				gi.Members[uid] = struct{}{}
				gm.userToGroup[uid] = len(gm.groups)
			}
		} else {
			gm.logger.WithField("group", g.Name).WithField("err", err).Warn("Failed to list group users (continuing)")
		}
		gm.groups = append(gm.groups, gi)
	}

	gm.logger.Info("GroupManager initialized with %d groups", len(gm.groups))
	return nil
}

// PickGroupIndex finds index of the group to add a user to.
// Strategy: pick group with minimum size; tie-break via nextTieIndex to create round-robin across equals.
func (gm *GroupManager) PickGroupIndex() int {
	bestIdx := 0
	bestSize := math.MaxInt32
	n := len(gm.groups)
	if n == 0 {
		return -1
	}
	start := gm.nextTieIndex % n
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		size := len(gm.groups[idx].Members)
		if size < bestSize {
			bestSize = size
			bestIdx = idx
		}
	}
	// advance tie index so next tie will prefer next group
	gm.nextTieIndex = (gm.nextTieIndex + 1) % n
	return bestIdx
}

// AssignUser assigns a user to an in-memory group and performs Nakama GroupUsersAdd.
// Returns group name and error (if any).
func (gm *GroupManager) AssignUser(ctx context.Context, userID string) (string, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// If user already assigned, return existing
	if idx, ok := gm.userToGroup[userID]; ok {
		return gm.groups[idx].Name, nil
	}

	idx := gm.PickGroupIndex()
	if idx < 0 || idx >= len(gm.groups) {
		return "", fmt.Errorf("no groups available")
	}

	gi := gm.groups[idx]
	// update Nakama first (so server authoritative membership exists); if this fails we don't mutate cache
	if err := gm.nk.GroupUsersAdd(ctx, "", gi.ID, []string{userID}); err != nil {
		return "", fmt.Errorf("GroupUsersAdd failed: %w", err)
	}

	// update cache
	gi.Members[userID] = struct{}{}
	gm.userToGroup[userID] = idx
	return gi.Name, nil
}

// RemoveUser removes a user from their group (both Nakama and cache).
func (gm *GroupManager) RemoveUser(ctx context.Context, userID string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	idx, ok := gm.userToGroup[userID]
	if !ok {
		return nil // not assigned
	}
	gi := gm.groups[idx]

	if err := gm.nk.GroupUserLeave(ctx, gi.ID, userID, ""); err != nil {
		// log and still update cache to avoid leaking memory if Nakama transient error;
		// you may prefer to retry or keep it until reconciliation.
		gm.logger.WithField("err", err).Warn("GroupUsersRemove failed (continuing with cache update)")
	}

	delete(gi.Members, userID)
	delete(gm.userToGroup, userID)
	return nil
}

// GetUserGroupName returns the cached group name for a user (if any)
func (gm *GroupManager) GetUserGroupName(userID string) (string, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	if idx, ok := gm.userToGroup[userID]; ok && idx < len(gm.groups) {
		return gm.groups[idx].Name, true
	}
	return "", false
}

var groupManager *GroupManager

func handlePlayerJoin(ctx context.Context, nk runtime.NakamaModule, userID string, sessionID string, logger runtime.Logger) {
	// If user already has a group, just ensure stream join and account metadata update
	if gname, ok := groupManager.GetUserGroupName(userID); ok {
		// update account metadata
		groupdata := map[string]any{"group": map[string]any{"name": gname}}
		_ = nk.AccountUpdateId(ctx, userID, "", groupdata, "", "", "", "", "")
		return
	}

	// assign via GroupManager (this will call GroupUsersAdd and update cache)
	gname, err := groupManager.AssignUser(ctx, userID)
	if err != nil {
		logger.WithField("err", err).Error("AssignUser failed")
		return
	}

	// join the stream
	if _, err := nk.StreamUserJoin(StreamMode, "", "", gname, userID, sessionID, false, false, ""); err != nil {
		logger.WithField("err", err).Error("Stream join failed")
	}

	groupdata := map[string]any{
		"group": map[string]any{
			"name": gname,
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
	GroupName string
	Players   map[string]*Player
	//Score     map[string]int //player scores
}

func distanceMeters(a, b Position) float64 {
	const R = 6371000.0 // Earth radius in meters
	dLat := (b.Lat - a.Lat) * math.Pi / 180.0
	dLon := (b.Lon - a.Lon) * math.Pi / 180.0

	lat1 := a.Lat * math.Pi / 180.0
	lat2 := b.Lat * math.Pi / 180.0

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1)*math.Cos(lat2)
	c := 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	return R * c
}

func (m *GlobalMatch) MatchInit(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, params map[string]interface{}) (interface{}, int, string) {
	groupName, _ := params["group_name"].(string)
	if groupName == "" {
		groupName = "unknown_group"
	}
	state := &MatchState{
		Debug:     true,
		GroupName: groupName,
		Players:   make(map[string]*Player),
	}
	tickRate := 1
	label := fmt.Sprintf("match=%s", groupName)
	logger.Info("Initialized match for %s", groupName)
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
			distance := distanceMeters(prevPos, newPos)
			if distance > 30 {
				logger.Warn("Invalid movement from %s: prev=%+v new=%+v", userID, prevPos, newPos)
				continue
			}
			if distance < 1 {
				logger.Warn("Negligible movement from %s: prev=%+v new=%+v", userID, prevPos, newPos)
				continue // negligible movement
			}
		}

		player.Position = newPos
		data, _ := json.Marshal(map[string]any{
			"user_id": userID,
			"lat":     newPos.Lat,
			"lon":     newPos.Lon,
		})
		dispatcher.BroadcastMessage(1, data, nil, nil, false)
		updatePlayerPosition(nk, userID, player.SessionID, newPos)
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
	//groupName := getUserGroup(ctx, nk, userID)
	groupName := ""
	groups, _, err := nk.UserGroupsList(ctx, userID, 1, nil, "")
	if err == nil && len(groups) > 0 {
		groupName = groups[0].Group.Name
	}
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

	groupManager = NewGroupManager(nk, logger, MaxGroups, 6)
	if err := groupManager.Init(ctx); err != nil {
		logger.WithField("err", err).Error("GroupManager.Init failed")
		return err
	}

	// Register session start as before but now use groupManager instead of handlePlayerJoin reading storage lock
	if err := initializer.RegisterEventSessionStart(
		func(ctx context.Context, logger runtime.Logger, evt *api.Event) {
			userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
			sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
			// try to ensure user assigned and stream joined
			handlePlayerJoin(ctx, nk, userID, sessionID, logger)
			// store the local map for quick access (if you still use userGroups)
			if gname, ok := groupManager.GetUserGroupName(userID); ok {
				userGroups[userID] = gname
			}
		},
	); err != nil {
		return err
	}

	if err := initializer.RegisterRpc("get_match", func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
		return rpcGetMatch(ctx, nk, logger)
	}); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := InitContentSync(ctx, logger, db, nk, initializer); err != nil {
		return err
	}

	logger.Info("On connection module loaded.")
	return nil
}
