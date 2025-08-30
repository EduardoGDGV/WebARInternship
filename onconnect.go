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
	GroupNamePrefix = "Group"
	MaxGroups		= 80
	RetryCount      = 5
	RetryDelay      = 100 * time.Millisecond
	LockCollection  = "locks"
	JoinLockKey     = "join_lock"
	GroupSizeKey     = "max_group_size"
    NextGroupKey     = "next_group"
	StreamMode = 2
	AdminID = "319e1542-46ed-42fa-aa71-3d26dc6c976e"
)

//
// --- Read/Write in storage ---
//

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
        Collection:     LockCollection,
        Key:            key,
        Value:          string(val),
        UserID:         "",
        PermissionRead: 2,
        PermissionWrite: 2,
    }})
    if err != nil {
        fmt.Printf("Failed to write %s: %v\n", key, err)
    }
}

//
// --- Stream Helpers ---
//

func rpcJoinGroup(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Payload is expected to be the group name
    groupName := payload

    _, err := nk.StreamUserJoin(StreamMode, "", "", groupName, userID, sessionID, false, false, "")
    if err != nil {
        return "", err
    }

    return `{"ok":true}`, nil
}

//
// --- Lock Helpers ---
//

func acquireLock(nk runtime.NakamaModule, key, userID string) bool {
	for attempt := 1; attempt <= RetryCount; attempt++ {
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
			time.Sleep(RetryDelay)
			continue
		}

		// Otherwise, write lock = true
		val, _ := json.Marshal(map[string]interface{}{"locked": true})
		_, err = nk.StorageWrite(context.Background(), []*runtime.StorageWrite{
			{
				Collection:     LockCollection,
				Key:            key,
				Value:          string(val),
				UserID:         "",
				PermissionRead: 2, // public
				PermissionWrite: 2, // public
			},
		})
		return err == nil
	}
	return false
}

func releaseLock(nk runtime.NakamaModule, key, userID string) {
	val, _ := json.Marshal(map[string]interface{}{"locked": false})
	_, _ = nk.StorageWrite(context.Background(), []*runtime.StorageWrite{
		{
			Collection:     LockCollection,
			Key:            key,
			Value:          string(val),
			UserID:         "",
			PermissionRead: 2, // public
			PermissionWrite: 2, // public
		},
	})
}

//
// --- Player Join ---
//

func handlePlayerJoin(ctx context.Context, nk runtime.NakamaModule, userID string, sessionID string, logger runtime.Logger) {
    // Acquire lock so only one joiner mutates shared state at a time
    if !acquireLock(nk, JoinLockKey, userID) {
        logger.Error("Could not acquire join lock for user %s", userID)
        return
    }
    defer releaseLock(nk, JoinLockKey, userID)

    // List all available groups
    maxmembers := 100
    open := true
    groups, _, err := nk.GroupsList(ctx, "", "", &maxmembers, &open, 80, "")
    if err != nil {
        logger.Error("Error fetching groups: %v", err)
        return
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

    groupdata := map[string]interface{}{
        "group": map[string]interface{}{
            "id": groups[nextGroup].Id,
            "name": groups[nextGroup].Name,
        },
    }

    if err := nk.AccountUpdateId(ctx, userID, "", groupdata, "", "", "", "", ""); err != nil {
        logger.WithField("err", err).Error("Account update error.")
    }
}

//
// --- Init Module ---
//

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	members := 100
	open := true
	groups, _, err := nk.GroupsList(context.Background(), "", "", &members, &open, 100, "")
	if err != nil {
		return err
	}

	// If no groups found, create MaxGroups
    if len(groups) == 0 {
        logger.Info("No groups found, creating groups...")
        for i := 1; i <= MaxGroups; i++ {
            name := fmt.Sprintf("%s_%d", GroupNamePrefix, i)
            _, err := nk.GroupCreate(context.Background(), AdminID, name, "", "", "", "", true, map[string]interface{}{
				"items": map[string]interface{}{"test": "test",},
            }, 100)
            if err != nil {
                logger.Error("Failed to create group %s: %v", name, err)
                return err
            }
        }
    }

	if err := initializer.RegisterEventSessionStart(
        func(ctx context.Context, logger runtime.Logger, evt *api.Event) {
            userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
			sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
			groups, _, err := nk.UserGroupsList(ctx, userID, 1, nil, "")
			if err != nil {
				return
			}
			if len(groups) != 0 {
				group := groups[0]
				if _, err := nk.StreamUserJoin(StreamMode, "", "", group.GetGroup().Name, userID, sessionID, false, false, ""); err != nil {
					logger.Error("Failed stream join for user %s: %v", userID, err)
					return
				}
			}else{
				handlePlayerJoin(ctx, nk, userID, sessionID, logger)
			}
        },
    ); err != nil {
        return err
    }

    /*if err := initializer.RegisterEventSessionEnd(
        func(ctx context.Context, logger runtime.Logger, evt *api.Event) {
            userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
			sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
            handlePlayerLeave(ctx, nk, userID, sessionID, logger)
        },
    ); err != nil {
        return err
    }*/

	if err := InitBuildings(ctx, logger, db, nk, initializer); err != nil {
		logger.Error("Failed to init buildings module: %v", err)
		return err
	}

	if err := initializer.RegisterRpc("rpcJoinCell", rpcJoinCell); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterRpc("rpcLeaveCell", rpcLeaveCell); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterRpc("rpcSendLocation", rpcSendLocation); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterRpc("rpcJoinGroup", rpcJoinGroup); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	logger.Info("Group balancing module loaded (Go).")
	return nil
}