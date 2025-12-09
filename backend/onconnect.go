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
	"fmt"
	"math"
	"sync"

	//"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

const (
	GroupNamePrefix = "Group"
	MaxGroups       = 80
	NumMatches	  	= 4
	AdminID         = "5c6f4519-0ba6-4fd2-b26d-f3639c3bf1e3"
)

// GroupManager
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
	mu           sync.RWMutex   // Read/write lock for concurrency
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

// Initialize/list/create groups in Nakama and populate the in-memory groups array
func (gm *GroupManager) Init(ctx context.Context) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// load groups from Nakama
	maxmembers := 200
	open := true
	groups, _, err := gm.nk.GroupsList(ctx, "", "", &maxmembers, &open, gm.maxGroups, "")
	if err != nil {
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
			// *The names will be mapped to a set of names in the future
			name := fmt.Sprintf("%s_%d", GroupNamePrefix, i)
			if _, ok := existing[name]; ok {
				continue
			}
			// *Has to have an Admin for creation
			if _, err := gm.nk.GroupCreate(ctx, AdminID, name, "", "", "", "", true, map[string]any{}, 100); err != nil {
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
		// GroupUsersList once per group (cheap at startup, up to 80 groups)
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

// GetNextGroup finds index of the group to add a user to
// Pick group with minimum size and tie-break via nextTieIndex to create round-robin across equals
func (gm *GroupManager) GetNextGroup() int {
    n := len(gm.groups)
    start := gm.nextTieIndex

    if n == 0 {
        return -1
    }
	bestIdx := 0
	bestSize := math.MaxInt32

	for i := range n {
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

// Assigns a user to a group (Nakama + Cache)
func (gm *GroupManager) AssignUser(ctx context.Context, userID string) (string, error) {
    // Try to reserve group index under lock
    gm.mu.Lock()
    if idx, ok := gm.userToGroup[userID]; ok {
        name := gm.groups[idx].Name
        gm.mu.Unlock()
        return name, nil
    }

    idx := gm.GetNextGroup()
    if idx < 0 || idx >= len(gm.groups) {
        gm.mu.Unlock()
        return "", fmt.Errorf("no groups available")
    }
    group := gm.groups[idx]
    gm.mu.Unlock()
    // Perform external call (no gm lock)
    if err := gm.nk.GroupUsersAdd(ctx, "", group.ID, []string{userID}); err != nil {
        return "", fmt.Errorf("GroupUsersAdd failed: %w", err)
    }

    // After external success, acquire lock and update cache
    gm.mu.Lock()
    defer gm.mu.Unlock()
    if existingIdx, ok := gm.userToGroup[userID]; ok {
        // someone else already assigned concurrently, prefer existing assignment
        return gm.groups[existingIdx].Name, nil
    }
    // commit to cache
    group.Members[userID] = struct{}{}
    gm.userToGroup[userID] = idx
    return group.Name, nil
}

// RemoveUser removes a user from their group, both in Nakama and cache (unused right now)
func (gm *GroupManager) RemoveUser(ctx context.Context, userID string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	idx, ok := gm.userToGroup[userID]
	if !ok {
		return nil // not assigned
	}
	group := gm.groups[idx]

	if err := gm.nk.GroupUserLeave(ctx, group.ID, userID, ""); err != nil {
		// log and still update cache to avoid leaking memory if Nakama transient error
		gm.logger.WithField("err", err).Warn("GroupUsersRemove failed (continuing with cache update)")
	}

	delete(group.Members, userID)
	delete(gm.userToGroup, userID)
	return nil
}

// GetUserGroup returns the cached group name and index for a user (if any)
func (gm *GroupManager) GetUserGroup(nk runtime.NakamaModule, ctx context.Context, userID string) (string, string, bool) {
	gm.mu.RLock()
	if idx, ok := gm.userToGroup[userID]; ok && idx < len(gm.groups) {
		id := gm.groups[idx].ID
        name := gm.groups[idx].Name
        gm.mu.RUnlock()
        return id, name, true
	}
	gm.mu.RUnlock()

	// Fallback query Nakama (in case cache missed it)
	groups, _, err := nk.UserGroupsList(ctx, userID, 1, nil, "")
	if err != nil {
		return "", "", false
	}
	if len(groups) != 0 {
		group := groups[0].GetGroup()
		// update cache
		gm.mu.Lock()
		for i, g := range gm.groups {
			if g.ID == group.GetId() {
				g.Members[userID] = struct{}{}
				gm.userToGroup[userID] = i
				break
			}
		}
		gm.mu.Unlock()
		return group.Id, group.Name, true
	}
	return "", "", false
}

// Initialize global group manager
var groupManager *GroupManager

func handlePlayerJoin(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	// check if already assigned
	_, name, assigned := groupManager.GetUserGroup(nk, ctx, userID)
	if assigned {
		return name, nil
	}
	// assign via GroupManager (call GroupUsersAdd and update cache)
	name, err := groupManager.AssignUser(ctx, userID)
	if err != nil {
		logger.WithField("err", err).Error("AssignUser failed")
		return "", err
	}
	return name, nil
}

var globalMatchManager *MatchManager
var once sync.Once

type MatchManager struct {
	matchIDs [NumMatches]string // persistent matches
}

func GetMatchManager() *MatchManager {
	once.Do(func() {
		globalMatchManager = &MatchManager{}
	})
	return globalMatchManager
}

// Init Module
func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterMatch("global_match", NewGlobalMatch); err != nil {
		return err
	}
	
	// create persistent matches
	mm := GetMatchManager()

	for i := range NumMatches {
		params := map[string]any{
			"match_id": i,
		}
		matchID, err := nk.MatchCreate(ctx, "global_match", params)
		if err != nil {
			logger.Error("Failed creating persistent match %d: %v", i, err)
			return err
		}
		mm.matchIDs[i] = matchID
		logger.Info("Created global match %d : %s", i, matchID)
	}

	if err := initializer.RegisterRpc("get_match", rpcGetGlobalMatch); err != nil {
		return err
	}

	groupManager = NewGroupManager(nk, logger, MaxGroups, 6)
	if err := groupManager.Init(ctx); err != nil {
		logger.WithField("err", err).Error("GroupManager.Init failed")
		return err
	}

	if err := initializer.RegisterRpc("join_group", handlePlayerJoin); err != nil {
		return err
	}

	if err := InitContentSync(ctx, logger, db, nk, initializer); err != nil {
		return err
	}

	logger.Info("On connection module loaded.")
	return nil
}
