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
	StreamMode = 2
)

//
// --- Stream Helpers ---
//

func rpcJoinCell(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Decode payload JSON
    var data struct {
        Lat float64 `json:"lat"`
        Lon float64 `json:"lon"`
    }
    if err := json.Unmarshal([]byte(payload), &data); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }

    // Join the cell stream
    label := fmt.Sprintf("cell_%f_%f", data.Lat, data.Lon)
    if _, err := nk.StreamUserJoin(StreamMode, "", "", label, userID, sessionID, false, false, ""); err != nil {
        return "", fmt.Errorf("failed to join cell: %w", err)
    }

    return `{"ok":true}`, nil
}

func rpcLeaveCell(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Optional: decode payload if you want to specify a group/cell
    var data struct {
        Lat float64 `json:"lat,omitempty"`
        Lon float64 `json:"lon,omitempty"`
    }
    _ = json.Unmarshal([]byte(payload), &data) // ignore error if optional

    // Leave all cell streams if lat/lon provided
    if data.Lat != 0 || data.Lon != 0 {
        label := fmt.Sprintf("cell_%f_%f", data.Lat, data.Lon)
        _ = nk.StreamUserLeave(StreamMode, "", "", label, userID, sessionID)
    }
	return `{"ok":true}`, nil
}

func sendCellData(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)

    // Decode payload only to extract lat/lon for the cell label
    var payloadMap map[string]interface{}
    if err := json.Unmarshal([]byte(payload), &payloadMap); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }

    lat, okLat := payloadMap["lat"].(float64)
    lon, okLon := payloadMap["lon"].(float64)
    data, okData := payloadMap["data"]
    if !okLat || !okLon || !okData {
        return "", runtime.NewError("missing lat, lon, or data fields", 3)
    }

    // Construct the JSON object to send: { "user_id": "...", "data": ... }
    msgBytes, err := json.Marshal(map[string]interface{}{
        "user_id": userID,
        "data":    data,
		"from_group": false,
    })
    if err != nil {
        return "", fmt.Errorf("failed to marshal message: %w", err)
    }

    // Send to the cell stream
    cellLabel := fmt.Sprintf("cell_%f_%f", lat, lon)
    if err := nk.StreamSend(StreamMode, "", "", cellLabel, string(msgBytes), nil, true); err != nil {
        return "", fmt.Errorf("failed to send to cell stream: %w", err)
    }

    return `{"ok":true}`, nil
}

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

func rpcLeaveGroup(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Payload is expected to be the group name
    groupName := payload

    if err := nk.StreamUserLeave(StreamMode, "", "", groupName, userID, sessionID); err != nil {
        return "", err
    }
    return `{"ok":true}`, nil
}

func sendGroupData(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    sessionID := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)

    // Decode payload to use as `data` for message
    var payloadMap map[string]interface{}
    if err := json.Unmarshal([]byte(payload), &payloadMap); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }

    data, okData := payloadMap["data"]
    if !okData {
        return "", runtime.NewError("missing data field", 3)
    }

    // Fetch all groups the user belongs to
    groups, _, err := nk.UserGroupsList(ctx, userID, 100, nil, "")
    if err != nil {
        logger.WithField("err", err).Error("UserGroupsList failed")
        return "", err
    }

    // Prepare message: { "user_id": ..., "data": ... }
    msgBytes, err := json.Marshal(map[string]interface{}{
        "user_id": userID,
        "data":    data,
		"from_group": true,
    })
    if err != nil {
        return "", fmt.Errorf("failed to marshal message: %w", err)
    }
    msg := string(msgBytes)

    // Send to each group stream
    for _, g := range groups {
        groupLabel := g.GetGroup().Name
        if metaPresence, err := nk.StreamUserGet(StreamMode, "", "", groupLabel, userID, sessionID); err != nil {
            logger.WithField("err", err).Error("Stream user get error")
        } else if metaPresence != nil {
            if err := nk.StreamSend(StreamMode, "", "", groupLabel, msg, nil, true); err != nil {
                logger.WithField("group", groupLabel).WithField("err", err).Error("Failed to send to group stream")
            }
        }
    }

    return `{"ok":true}`, nil
}

func rpcSendLocation(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    if _, err := sendCellData(ctx, logger, db, nk, payload); err != nil {
		logger.WithField("err", err).Error("failed to send to cell stream")
		return "", err
	}
    if _, err := sendGroupData(ctx, logger, db, nk, payload); err != nil {
        logger.WithField("err", err).Error("failed to send to group stream")
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

	logger.Info("Last group %s has %d members", lastGroup.Id, len(members) + 1)

	if len(members) + 1 >= MaxGroupSize {
		g, err := createGroup(nk, userID, fmt.Sprintf("%s_%d", GroupNamePrefix, len(groups)+1), logger)
		if err != nil {
			return
		}
		_ = nk.GroupUsersAdd(context.Background(), "", g.Id, []string{userID})
		nk.StreamUserJoin(StreamMode, "", "", g.Name, userID, sessionID, false, false, "")

		for i := 0; i < 2 && i < len(members); i++ {
			_ = nk.GroupUsersAdd(context.Background(), "", g.Id, []string{members[i].User.Id})
			_ = nk.GroupUsersKick(context.Background(), "", lastGroup.Id, []string{members[i].User.Id})
			content := map[string]interface{}{
				"leave": lastGroup.Id,
				"enter": g.Id,
			}
			nk.NotificationSend(ctx, members[i].User.Id, "group_move", content, 1, "", false)
		}
	} else {
		if len(groups) > 1 {
			prevGroup := groups[len(groups)-2]
			prevMembers, _, _ := nk.GroupUsersList(context.Background(), prevGroup.Id, 100, &member_state, "")
			if len(prevMembers) <= len(members) {
				_ = nk.GroupUsersAdd(context.Background(), "", prevGroup.Id, []string{userID})
				nk.StreamUserJoin(StreamMode, "", "", prevGroup.Name, userID, sessionID, false, false, "")
			} else {
				_ = nk.GroupUsersAdd(context.Background(), "", lastGroup.Id, []string{userID})
				nk.StreamUserJoin(StreamMode, "", "", lastGroup.Name, userID, sessionID, false, false, "")
			}
		} else {
			_ = nk.GroupUsersAdd(context.Background(), "", lastGroup.Id, []string{userID})
			nk.StreamUserJoin(StreamMode, "", "", lastGroup.Name, userID, sessionID, false, false, "")
		}
	}
}

//
// --- Player Leave ---
//

func handlePlayerLeave(ctx context.Context, nk runtime.NakamaModule, userID string, sessionID string, logger runtime.Logger) {
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
				nk.StreamUserLeave(StreamMode, "", "", g.Name, userID, sessionID)
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
				content := map[string]interface{}{
					"leave": lastGroup.Id,
					"enter": g.Id,
				}
				nk.NotificationSend(ctx, moveID, "group_move", content, 1, "", false)
			}
			lastMembers, _, _ = nk.GroupUsersList(context.Background(), lastGroup.Id, 100, &member_state, "")

			if len(lastMembers) == 0 {
				_ = nk.GroupDelete(context.Background(), lastGroup.Id)
				logger.Info("Deleted empty group: %s", lastGroup.Id)
				return
			}

			if len(lastMembers)+len(secondLastMembers) <= MaxGroupSize {
				content := map[string]interface{}{
					"leave": lastGroup.Id,
					"enter": secondLast.Id,
				}
				for _, member := range lastMembers {
					_ = nk.GroupUsersAdd(context.Background(), "", secondLast.Id, []string{member.User.Id})
					nk.NotificationSend(ctx, member.User.Id, "group_move", content, 1, "", false)
				}
				_ = nk.GroupDelete(context.Background(), lastGroup.Id)
				logger.Info("Merged and deleted group: %s", lastGroup.Id)
				return
			}

			if len(secondLastMembers) < len(lastMembers) {
				moveID := lastMembers[0].User.Id
				_ = nk.GroupUsersAdd(context.Background(), "", secondLast.Id, []string{moveID})
				_ = nk.GroupUsersKick(context.Background(), "", lastGroup.Id, []string{moveID})
				content := map[string]interface{}{
					"leave": lastGroup.Id,
					"enter": secondLast.Id,
				}
				nk.NotificationSend(ctx, moveID, "group_move", content, 1, "", false)
				return
			}
		}
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

    if err := initializer.RegisterEventSessionEnd(
        func(ctx context.Context, logger runtime.Logger, evt *api.Event) {
            userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
			sessionID, _ := ctx.Value(runtime.RUNTIME_CTX_SESSION_ID).(string)
            handlePlayerLeave(ctx, nk, userID, sessionID, logger)
        },
    ); err != nil {
        return err
    }

	if err := InitBuildings(ctx, logger, db, nk, initializer); err != nil {
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

	if err := initializer.RegisterRpc("rpcLeaveGroup", rpcLeaveGroup); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	logger.Info("Group balancing module loaded (Go).")
	return nil
}
