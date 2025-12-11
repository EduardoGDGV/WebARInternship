package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

type Inventory struct {
	Items map[int]int `json:"items"`
	Cards map[int]int `json:"cards"`
}

func invDefault() Inventory {
	return Inventory{
		Items: map[int]int{},
		Cards: map[int]int{},
	}
}

func loadInventory(ctx context.Context, nk runtime.NakamaModule, coll, key string) (*Inventory, *api.StorageObject, error) {
	objects, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{Collection: coll, Key: key, UserID: ""},
	})
	if err != nil {
		return nil, nil, err
	}
	if len(objects) == 0 {
		inv := invDefault()
		return &inv, nil, nil
	}
	var inv Inventory
	if err := json.Unmarshal([]byte(objects[0].Value), &inv); err != nil {
		return nil, nil, err
	}
	return &inv, objects[0], nil
}

func saveInventory(ctx context.Context, nk runtime.NakamaModule, coll, key string, inv Inventory, version string) error {
	b, _ := json.Marshal(inv)
	_, err := nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection: coll,
			Key:        key,
			UserID:     "",
			Value:      string(b),
			Version:    version, // CAS for concurrency safety
		},
	})
	return err
}

// Ensure user is a member of the specified group
func requireGroupMember(ctx context.Context, nk runtime.NakamaModule, userId, groupId string) error {
	memberstate := 2
	members, _, err := nk.GroupUsersList(ctx, groupId, 100, &memberstate, "")
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.User.Id == userId {
			return nil
		}
	}
	return errors.New("not a group member")
}

func rpcGetInventory(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	var req struct {
		GroupID string `json:"groupId,omitempty"`
	}
	_ = json.Unmarshal([]byte(payload), &req)
	var coll, key string
	if req.GroupID != "" {
		// Group inventory
		if err := requireGroupMember(ctx, nk, userID, req.GroupID); err != nil {
			return "", runtime.NewError("forbidden", 3)
		}
		coll = "group_inventory"
		key = req.GroupID
	} else {
		// Player inventory
		coll = "player_inventory"
		key = userID
	}
	inv, _, err := loadInventory(ctx, nk, coll, key)
	if err != nil {
		return "", err
	}
	res, _ := json.Marshal(inv)
	return string(res), nil
}

func rpcUseItem(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	var req struct {
		GroupID string `json:"groupId,omitempty"`
		ItemID  int    `json:"itemId"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", err
	}

	var coll, key string
	if req.GroupID != "" {
		if err := requireGroupMember(ctx, nk, userID, req.GroupID); err != nil {
			return "", runtime.NewError("forbidden", 3)
		}
		coll = "group_inventory"
		key = req.GroupID
	} else {
		coll = "player_inventory"
		key = userID
	}

	inv, oldObj, err := loadInventory(ctx, nk, coll, key)
	if err != nil {
		return "", err
	}
	oldVersion := ""
	if oldObj != nil {
		oldVersion = oldObj.Version
	}

	qty := inv.Items[req.ItemID]
	if qty <= 0 {
		return "", fmt.Errorf("not enough quantity")
	}
	inv.Items[req.ItemID]--

	if err := saveInventory(ctx, nk, coll, key, *inv, oldVersion); err != nil {
		return "", err
	}
	return `{"success":true}`, nil
}

// Client RPC to get events
func rpcGetVisibleEvents(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    // Load all events from storage
    objects, _, err := nk.StorageList(ctx, "", "", "events", 1000, "")
    if err != nil {
        return "", runtime.NewError("cannot load events", 13)
    }

    type VisibleEvent struct {
        ID    int     `json:"id"`
        Title string  `json:"title"`
        PosX  float64 `json:"posX"`
        PosY  float64 `json:"posY"`
        Icon  string  `json:"icon"`
        Type  string  `json:"type"`
    }
    visible := make([]VisibleEvent, 0, len(objects))

    for _, obj := range objects {
        var full map[string]any
        _ = json.Unmarshal([]byte(obj.Value), &full)

        ev := VisibleEvent{
            ID:    int(full["id"].(float64)),
            Title: full["title"].(string),
            PosX:  full["posX"].(float64),
            PosY:  full["posY"].(float64),
            Icon:  full["icon"].(string),
            Type:  full["type"].(string),
        }
        visible = append(visible, ev)
    }
    b, _ := json.Marshal(visible)
    return string(b), nil
}

func rpcCompleteEvent(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID, _ := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    var response struct {
        EventID int            `json:"eventId"`
        Answers []string       `json:"answers"`   // answers to quiz questions (if any)
        UseItems map[int]int   `json:"useItems"`  // itemID -> quantity to consume (if any)
        GroupID string         `json:"groupId"`
    }
    if err := json.Unmarshal([]byte(payload), &response); err != nil {
        return "", runtime.NewError("invalid payload", 3)
    }

	// Resolve event from cache
    contentCache.muEvents.RLock()
    ev, ok := contentCache.Events[response.EventID]
    contentCache.muEvents.RUnlock()
    if !ok {
        return "", runtime.NewError("event not found", 13)
    }

    // Check group membership
    if err := requireGroupMember(ctx, nk, userID, response.GroupID); err != nil {
        return "", runtime.NewError("forbidden", 3)
    }

    // Handle quiz if event includes one
    if len(ev.Requirements) > 0 {
        for _, reqID := range ev.Requirements {
            contentCache.muQuizzes.RLock()
            quiz, ok := contentCache.Quizzes[reqID]
            contentCache.muQuizzes.RUnlock()
            if ok {
                // Event requirement includes a quiz verify answers
                if len(response.Answers) != 1 {
                    return "", runtime.NewError("incorrect quiz answer", 3)
                }
                if !strings.EqualFold(response.Answers[0], quiz.Answer) {
                    return "", runtime.NewError("incorrect quiz answer", 3)
                }
            }
        }
    }

    // Load inventories once
    gInv, gObj, err := loadInventory(ctx, nk, "group_inventory", response.GroupID)
    if err != nil {
        return "", err
    }
    pInv, pObj, err := loadInventory(ctx, nk, "player_inventory", userID)
    if err != nil {
        return "", err
    }

    // Check & consume required items (if any)
    if len(response.UseItems) > 0 {
        // Validate first
        for itemID, qty := range response.UseItems {
            if qty <= 0 {
                continue
            }
            if gInv.Items[itemID] < qty {
                return "", runtime.NewError("missing required items", 3)
            }
        }
        // Then consume atomically
        for itemID, qty := range response.UseItems {
            if qty <= 0 {
                continue
            }
            gInv.Items[itemID] -= qty
        }
    }

    // Apply rewards (from cache)
    for _, rewardID := range ev.Rewards {
        // Try cards first
        contentCache.muCards.RLock()
        card, ok := contentCache.Cards[rewardID]
        contentCache.muCards.RUnlock()
        if ok {
            if card.GroupCard {
                gInv.Cards[rewardID]++
            } else {
                pInv.Cards[rewardID]++
            }
            continue
        }
        // Otherwise, item
        contentCache.muItems.RLock()
        _, itemExists := contentCache.Items[rewardID]
        contentCache.muItems.RUnlock()
        if itemExists {
            gInv.Items[rewardID]++
            continue
        }
        logger.Warn("reward ID not found in items or cards", map[string]interface{}{
            "rewardID": rewardID,
        })
    }

    // Save inventories with optimistic concurrency
    if err := saveInventory(ctx, nk, "group_inventory", response.GroupID, *gInv, gObj.Version); err != nil {
        return "", err
    }
    if err := saveInventory(ctx, nk, "player_inventory", userID, *pInv, pObj.Version); err != nil {
        return "", err
    }
    return `{"success":true}`, nil
}

func InitContent(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterRpc("fetch_events", rpcGetVisibleEvents); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("get_inventory", rpcGetInventory); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("complete_event", rpcCompleteEvent); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("use_item", rpcUseItem); err != nil {
		return err
	}
	return nil
}
