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
	MaxGroupSize    = 10
	RetryCount      = 5
	RetryDelay      = 100 * time.Millisecond
	LockCollection  = "locks"
	JoinLockKey     = "join_lock"
	StreamMode = 2
)

type Player struct {
    group string `json:"group"`
    items map[string]interface{} `json:"items"`
}

type GroupMeta struct {
	items map[string]interface{} `json:"items"`
}

//
// --- Player Helpers ---
//

func getPlayer(ctx context.Context, nk runtime.NakamaModule, userID string) (*Player, error) {
    acc, err := nk.AccountGetId(ctx, userID)
    if err != nil {
        return nil, err
    }
    var p Player
    if acc.User.Metadata != "" {
		_ = json.Unmarshal([]byte(acc.User.Metadata), &p)
	}
    return &p, nil
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

	// update player metadata
    p, err := getPlayer(ctx, nk, userID)
    if err != nil {
        return "", err
    }
	if p != nil {
		if err := nk.AccountUpdateId(ctx, userID, "", map[string]interface{}{
			"group": groupName,
		}, "", "", "", "", ""); err != nil {
			return "", err
		}
	}

    return `{"ok":true}`, nil
}

func sendGroupData(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)

    var payloadMap map[string]interface{}
    if err := json.Unmarshal([]byte(payload), &payloadMap); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }
    data, ok := payloadMap["data"]
    if !ok {
        return "", runtime.NewError("missing data field", 3)
    }

    p, err := getPlayer(ctx, nk, userID)
    if err != nil {
        return "", err
    }
    if p.group == "" {
        return `{"ok":false}`, nil
    }

    msgBytes, _ := json.Marshal(map[string]interface{}{
        "user_id":    userID,
        "data":       data,
        "from_group": true,
    })

    if err := nk.StreamSend(StreamMode, "", "", p.group, string(msgBytes), nil, true); err != nil {
        logger.WithField("group", p.group).WithField("err", err).Error("Failed to send to group stream")
        return "", err
    }

    return `{"ok":true}`, nil
}

//
// --- Lock Helpers ---
//

func acquireLock(nk runtime.NakamaModule, key, userID string) bool {
	for attempt := 1; attempt <= RetryCount; attempt++ {
		records, err := nk.StorageRead(context.Background(), []*runtime.StorageRead{{
			Collection: LockCollection,
			Key:        key,
			UserID:     "",
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
				Value:      string(val),
				UserID:     "",
				PermissionRead:  2, // public
    			PermissionWrite: 2, // public
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
		Value:      string(val),
		UserID:     "",
		PermissionRead:  2, // public
    	PermissionWrite: 2, // public
	}})
}

//
// --- Group Helpers ---
//

func createGroup(nk runtime.NakamaModule, userID string, name string, logger runtime.Logger) (*api.Group, error) {
	group, err := nk.GroupCreate(context.Background(), userID, name, "", "", "", "", true, map[string]interface{}{}, MaxGroupSize)
	if err != nil {
		return nil, err
	}
	logger.Info("Created group: %s", group.Id)
	return group, nil
}

func getGroups(nk runtime.NakamaModule, logger runtime.Logger) ([]*api.Group, error) {
	members := 10
	open := true
	groups, _, err := nk.GroupsList(context.Background(), "", "", &members, &open, 100, "")
	if err != nil {
		return nil, err
	}
	logger.Info("Fetched %d matching groups", len(groups))
	return groups, nil
}

//
// --- Player Join ---
//

func handlePlayerJoin(ctx context.Context, nk runtime.NakamaModule, userID string, sessionID string, logger runtime.Logger) {
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
			nk.StreamUserJoin(StreamMode, "", "", g.Name, userID, sessionID, false, false, "")
		}else{
			logger.Error("Error creating group: %v", err)
		}
		return
	}

	lastGroup := groups[len(groups)-1]
	members, _, _ := nk.GroupUsersList(context.Background(), lastGroup.Id, 100, &member_state, "")

	if len(members) + 1 >= MaxGroupSize {
		g, err := createGroup(nk, userID, fmt.Sprintf("%s_%d", GroupNamePrefix, len(groups)+1), logger)
		if err != nil {
			return
		}
		_ = nk.GroupUsersAdd(context.Background(), "", g.Id, []string{userID})
		nk.StreamUserJoin(StreamMode, "", "", g.Name, userID, sessionID, false, false, "")
	} else {
		_ = nk.GroupUsersAdd(context.Background(), "", lastGroup.Id, []string{userID})
		nk.StreamUserJoin(StreamMode, "", "", lastGroup.Name, userID, sessionID, false, false, "")
	}
}

//
// --- Init Module ---
//

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterEventSessionStart(
        func(ctx context.Context, logger runtime.Logger, evt *api.Event) {
            userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
			sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
            handlePlayerJoin(ctx, nk, userID, sessionID, logger)
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