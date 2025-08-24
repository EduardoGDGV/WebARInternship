package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

const (
	GroupNamePrefix = "AutoGroup"
	MaxGroupSize    = 5
	RetryCount      = 5
	RetryDelay      = 100 * time.Millisecond
	LockCollection  = "locks"
	JoinLockKey     = "join_lock"
	LeaveLockKey    = "leave_lock"
)

//
// --- Lock Helpers ---
//

func acquireLock(nk runtime.NakamaModule, key, userID string) bool {
	for attempt := 1; attempt <= RetryCount; attempt++ {
		records, err := nk.StorageRead(context.Background(), []*runtime.StorageRead{{
			Collection: LockCollection,
			Key:        key,
			UserID:     userID,
		}})
		if err != nil {
			return false
		}

		if len(records) > 0 && string(records[0].Value) == `{"locked":true}` {
			time.Sleep(RetryDelay)
		} else {
			val, _ := json.Marshal(map[string]interface{}{"locked": true})
			_, err := nk.StorageWrite(context.Background(), []*runtime.StorageWrite{{
				Collection: LockCollection,
				Key:        key,
				UserID:     userID,
				Value:      string(val),
			}})
			return err == nil
		}
	}
	return false
}

func releaseLock(nk runtime.NakamaModule, key, userID string) {
	val, _ := json.Marshal(map[string]interface{}{"locked": false})
	_, _ = nk.StorageWrite(context.Background(), []*runtime.StorageWrite{{
		Collection: LockCollection,
		Key:        key,
		UserID:     userID,
		Value:      string(val),
	}})
}

//
// --- Group Helpers ---
//

func createGroup(nk runtime.NakamaModule, ownerID, name string, logger runtime.Logger) (*api.Group, error) {
	group, err := nk.GroupCreate(context.Background(), ownerID, name, "", "", "", "", true, map[string]interface{}{}, MaxGroupSize)
	if err != nil {
		return nil, err
	}
	logger.Info("Created group: %s", group.Id)
	return group, nil
}

func getGroups(nk runtime.NakamaModule, logger runtime.Logger) ([]*api.Group, error) {
	members := 10
	open := true
	groups, _, err := nk.GroupsList(context.Background(), GroupNamePrefix, "", &members, &open, 200, "")
	if err != nil {
		return nil, err
	}
	logger.Info("Fetched %d matching groups", len(groups))
	return groups, nil
}

//
// --- Player Join ---
//

func handlePlayerJoin(nk runtime.NakamaModule, userID string, logger runtime.Logger) {
	member_state := 2
	if !acquireLock(nk, JoinLockKey, userID) {
		logger.Error("Could not acquire join lock for user %s", userID)
		return
	}
	defer releaseLock(nk, JoinLockKey, userID)

	groups, err := getGroups(nk, logger)
	if err != nil {
		logger.Error("Error fetching groups: %v", err)
		return
	}

	if len(groups) == 0 {
		g, err := createGroup(nk, userID, fmt.Sprintf("%s_%d", GroupNamePrefix, 1), logger)
		if err == nil {
			_ = nk.GroupUsersAdd(context.Background(), "", g.Id, []string{userID})
		}
		return
	}

	lastGroup := groups[len(groups)-1]
	members, _, _ := nk.GroupUsersList(context.Background(), lastGroup.Id, 100, &member_state, "")

	if len(members) >= MaxGroupSize {
		g, err := createGroup(nk, userID, fmt.Sprintf("%s_%d", GroupNamePrefix, len(groups)+1), logger)
		if err != nil {
			return
		}
		_ = nk.GroupUsersAdd(context.Background(), "", g.Id, []string{userID})

		toMove := []string{}
		for i := 0; i < 2 && i < len(members); i++ {
			toMove = append(toMove, members[i].User.Id)
		}
		if len(toMove) > 0 {
			_ = nk.GroupUsersAdd(context.Background(), "", g.Id, toMove)
			_ = nk.GroupUsersKick(context.Background(), "", lastGroup.Id, toMove)
		}
	} else {
		if len(groups) > 1 {
			prevGroup := groups[len(groups)-2]
			prevMembers, _, _ := nk.GroupUsersList(context.Background(), prevGroup.Id, 100, &member_state, "")
			if len(prevMembers) <= len(members) {
				_ = nk.GroupUsersAdd(context.Background(), "", prevGroup.Id, []string{userID})
			} else {
				_ = nk.GroupUsersAdd(context.Background(), "", lastGroup.Id, []string{userID})
			}
		} else {
			_ = nk.GroupUsersAdd(context.Background(), "", lastGroup.Id, []string{userID})
		}
	}
}

//
// --- Player Leave ---
//

func handlePlayerLeave(nk runtime.NakamaModule, userID string, logger runtime.Logger) {
	member_state := 2
	if !acquireLock(nk, LeaveLockKey, userID) {
		logger.Error("Could not acquire leave lock for user %s", userID)
		return
	}
	defer releaseLock(nk, LeaveLockKey, userID)

	groups, err := getGroups(nk, logger)
	if err != nil || len(groups) < 1 {
		return
	}

	for _, g := range groups {
		members, _, _ := nk.GroupUsersList(context.Background(), g.Id, 100, &member_state, "")
		found := false
		for _, m := range members {
			if m.User.Id == userID {
				_ = nk.GroupUsersKick(context.Background(), "", g.Id, []string{userID})
				found = true
				break
			}
		}
		if found && len(groups) > 1 {
			lastGroup := groups[len(groups)-1]
			secondLast := groups[len(groups)-2]
			lastMembers, _, _ := nk.GroupUsersList(context.Background(), lastGroup.Id, 100, &member_state, "")
			secondLastMembers, _, _ := nk.GroupUsersList(context.Background(), secondLast.Id, 100, &member_state, "")

			if len(lastMembers) > 0 {
				moveID := lastMembers[0].User.Id
				_ = nk.GroupUsersAdd(context.Background(), "", g.Id, []string{moveID})
				_ = nk.GroupUsersKick(context.Background(), "", lastGroup.Id, []string{moveID})
			}
			lastMembers, _, _ = nk.GroupUsersList(context.Background(), lastGroup.Id, 100, &member_state, "")

			if len(lastMembers) == 0 {
				_ = nk.GroupDelete(context.Background(), lastGroup.Id)
				logger.Info("Deleted empty group: %s", lastGroup.Id)
				return
			}

			if len(lastMembers)+len(secondLastMembers) <= MaxGroupSize {
				moveIDs := []string{}
				for _, member := range lastMembers {
					moveIDs = append(moveIDs, member.User.Id)
				}
				_ = nk.GroupUsersAdd(context.Background(), "", secondLast.Id, moveIDs)
				_ = nk.GroupDelete(context.Background(), lastGroup.Id)
				logger.Info("Merged and deleted group: %s", lastGroup.Id)
				return
			}

			if len(secondLastMembers) < len(lastMembers) {
				moveID := lastMembers[0].User.Id
				_ = nk.GroupUsersAdd(context.Background(), "", secondLast.Id, []string{moveID})
				_ = nk.GroupUsersKick(context.Background(), "", lastGroup.Id, []string{moveID})
				return
			}
		}
	}
}

//
// --- Init Module ---
//

func eventSessionStart(ctx context.Context, logger runtime.Logger, evt *api.Event) {
	logger.Debug("process event session start: %+v", evt)
    //handlePlayerJoin(nk, evt.UserId, logger)
}

func eventSessionEnd(ctx context.Context, logger runtime.Logger, evt *api.Event) {
	logger.Debug("process event session end: %+v", evt)
   //handlePlayerLeave(nk, evt.UserId, logger)
}

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterEventSessionStart(eventSessionStart); err != nil {
		return err
	}
	
	if err := initializer.RegisterEventSessionEnd(eventSessionEnd); err != nil {
		return err
	}

	logger.Info("Group balancing module loaded (Go).")
	return nil
}
