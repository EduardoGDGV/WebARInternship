package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
)

// Configuration
const wpBase = "http://wordpress:80/wp-json/wp/v2" // WordPress base REST URL
const httpTimeout = 10 * time.Second

// Storage collections
var collectionByType = map[string]string{
	"event":   "events",
	"asset2d": "assets2d",
	"card":    "cards",
	"item":    "items",
	"quiz":    "quizzes",
}

// Data structures (payloads / storage models)

// Event (map anchor + relations)
type Event struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	Image        string  `json:"image"`        // resolved URL (may be empty)
	Requirements []int   `json:"requirements"` // IDs
	Rewards      []int   `json:"rewards"`      // IDs
	UpdatedAt    string  `json:"updated_at,omitempty"`
}

// Asset2D (visual resource)
type Asset2D struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Image     string `json:"image"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Card
type Card struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Front     string `json:"front"` // resolved URLs
	Back      string `json:"back"`
	GroupCard bool   `json:"group_card"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Item
type Item struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Image2D   string `json:"image_2d"`
	Image3D   string `json:"image_3d"` // could be URL or file name
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Quiz
type Quiz struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Question     string   `json:"question"`
	Alternatives []string `json:"alternatives"` // always array length 4
	Answer       string   `json:"answer"`       // "A"|"B"|"C"|"D"
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

// Generic incoming push payload (from WP)
type WPIncoming struct {
	ID      int             `json:"id"`
	Type    string          `json:"type"` // asset2d, card, item, quiz, event
	Title   string          `json:"title,omitempty"`
	Status  string          `json:"status,omitempty"`  // publish, update, delete, trash
	Content json.RawMessage `json:"content,omitempty"` // optional: already-shaped content
}

// HTTP client / WP fetch

var httpClient = &http.Client{Timeout: httpTimeout}

// getArrayFromMap -> []any
func getArrayFromMap(m map[string]any, keys ...string) []any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch x := v.(type) {
			case []any:
				fmt.Printf("found array for key %s", k)
				return x
			default:
				fmt.Printf("found non-array for key %s: %T", k, x)
				// if comma separated string
				if s, ok := x.(string); ok {
					parts := strings.Split(s, ",")
					out := make([]any, 0, len(parts))
					for _, p := range parts {
						out = append(out, strings.TrimSpace(p))
					}
					return out
				}
			}
		}
	}
	return nil
}

// parseFloatV extracts a float64 from interface values (string/float)
func parseFloatV(v any) (float64, error) {
	if v == nil {
		return 0, errors.New("nil")
	}
	switch x := v.(type) {
	case float64:
		fmt.Printf("parseFloatV: got float64 %f\n", x)
		return x, nil
	case int:
		fmt.Printf("parseFloatV: got int %d\n", x)
		return float64(x), nil
	case string:
		fmt.Printf("parseFloatV: got string %s\n", x)
		if x == "" {
			return 0, errors.New("empty")
		}
		return strconv.ParseFloat(x, 64)
	default:
		fmt.Printf("parseFloatV: unsupported type %T\n", v)
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

// parseIntArray converts []any to []int (skips non-numeric)
func parseIntArray(arr []any) []int {
	out := []int{}
	for _, v := range arr {
		switch x := v.(type) {
		case float64:
			fmt.Printf("parseIntArray: got float64 %f\n", x)
			out = append(out, int(x))
		case int:
			fmt.Printf("parseIntArray: got int %d\n", x)
			out = append(out, x)
		case string:
			fmt.Printf("parseIntArray: got string %s\n", x)
			if i, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
				out = append(out, i)
			}
		}
	}
	return out
}

// Fetch from WordPress (initial population)

// fetchPostsForType fetches WP posts for a given post type (rest endpoint e.g. /wp/v2/event)
func fetchPostsForType(logger runtime.Logger, postType string) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/%s?per_page=100", wpBase, postType)
	logger.Info("WP fetch: %s", url)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("wp fetch error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wp fetch status %d: %s", resp.StatusCode, string(b))
	}
	var posts []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&posts); err != nil {
		return nil, fmt.Errorf("wp decode posts error: %w", err)
	}
	return posts, nil
}

// Convert WP post map -> typed model

func buildEventFromWP(logger runtime.Logger, post map[string]any) (Event, error) {
	// find id/title/status/meta/acf
	idf := int(post["id"].(float64))
	var title string
	if v, ok := post["title"]; ok {
		t := v.(map[string]any)
		if rendered, ok := t["rendered"].(string); ok {
			title = rendered
		} else {
			logger.Warn("title.rendered not found for post id %v", idf)
		}
	} else {
		logger.Warn("missing title field for post id %v", idf)
	}

	// coordinates
	var lat, lon float64
	if v, ok := post["lat"]; ok {
		if f, err := parseFloatV(v); err == nil {
			lat = f
		}
	}
	if v, ok := post["lon"]; ok {
		if f, err := parseFloatV(v); err == nil {
			lon = f
		}
	}

	// image
	var imgURL string
	if v := post["image"]; v != nil {
		imgURL = v.(string)
	} else {
		logger.Error("failed to resolve event image URL")
	}

	// requirements / rewards
	req := parseIntArray(getArrayFromMap(post, "requirements"))
	rew := parseIntArray(getArrayFromMap(post, "rewards"))

	ev := Event{
		ID:           idf,
		Title:        title,
		Lat:          lat,
		Lon:          lon,
		Image:        imgURL,
		Requirements: req,
		Rewards:      rew,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	return ev, nil
}

func buildAsset2DFromWP(logger runtime.Logger, post map[string]any) (Asset2D, error) {
	logger.Info("Building Asset2D from WP post")
	idf := int(post["id"].(float64))
	logger.Info("Asset2D ID %d", idf)
	var title string
	logger.Info("Building Asset2D from WP post ID %d", idf)
	if v, ok := post["title"]; ok {
		t := v.(map[string]any)
		if rendered, ok := t["rendered"].(string); ok {
			title = rendered
		} else {
			logger.Warn("title.rendered not found for post id %v", idf)
		}
	} else {
		logger.Warn("missing title field for post id %v", idf)
	}
	logger.Info("Asset title %s", title)

	var imgURL string
	if v, ok := post["image"]; ok {
		logger.Info("Found image field for asset2d id %d", idf)
		imgURL = v.(string)
	} else {
		logger.Error("failed to resolve event image url")
	}
	logger.Info("Image URL %s", imgURL)

	return Asset2D{
		ID:        idf,
		Title:     title,
		Image:     imgURL,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func buildCardFromWP(logger runtime.Logger, post map[string]any) (Card, error) {
	idf := int(post["id"].(float64))
	var title string
	if v, ok := post["title"]; ok {
		t := v.(map[string]any)
		if rendered, ok := t["rendered"].(string); ok {
			title = rendered
		} else {
			logger.Warn("title.rendered not found for post id %v", idf)
		}
	} else {
		logger.Warn("missing title field for post id %v", idf)
	}

	var front, back string
	if v, ok := post["front_image"]; ok {
		front = v.(string)
	}
	if v, ok := post["back_image"]; ok {
		back = v.(string)
	}

	group := false
	if v, ok := post["group_card"]; ok {
		group = v.(bool)
	}

	return Card{
		ID:        idf,
		Title:     title,
		Front:     front,
		Back:      back,
		GroupCard: group,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func buildItemFromWP(logger runtime.Logger, post map[string]any) (Item, error) {
	logger.Info("Building Item from WP post")
	idf := int(post["id"].(float64))
	logger.Info("Item ID %d", idf)
	var title string
	if v, ok := post["title"]; ok {
		t := v.(map[string]any)
		if rendered, ok := t["rendered"].(string); ok {
			title = rendered
		} else {
			logger.Warn("title.rendered not found for post id %v", idf)
		}
	} else {
		logger.Warn("missing title field for post id %v", idf)
	}

	// Parse images
	var img2d, img3d string
	if v, ok := post["2d"]; ok {
		img2d = v.(string)
	}
	if v, ok := post["3d"]; ok {
		img3d = v.(string)
	}

	return Item{
		ID:        idf,
		Title:     title,
		Image2D:   img2d,
		Image3D:   img3d,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func buildQuizFromWP(logger runtime.Logger, post map[string]any) (Quiz, error) {
	idf := int(post["id"].(float64))
	var title string
	if v, ok := post["title"]; ok {
		t := v.(map[string]any)
		if rendered, ok := t["rendered"].(string); ok {
			title = rendered
		} else {
			logger.Warn("title.rendered not found for post id %v", idf)
		}
	} else {
		logger.Warn("missing title field for post id %v", idf)
	}

	var question string
	if v, ok := post["question"]; ok {
		question = v.(string)
	} else {
		logger.Error("failed to resolve quiz question")
	}

	var alts []string
	if v, ok := post["alternatives"]; ok {
		alts = v.([]string)
	} else {
		logger.Error("failed to resolve quiz alternatives")
	}

	var answer string
	if v, ok := post["answer"]; ok {
		answer = v.(string)
	} else {
		logger.Error("failed to resolve answer question")
	}

	return Quiz{
		ID:           idf,
		Title:        title,
		Question:     question,
		Alternatives: alts,
		Answer:       answer,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Storage helpers

func storageKeyFor(id int) string {
	return fmt.Sprintf("%d", id)
}

func writeToStorage(ctx context.Context, nk runtime.NakamaModule, collection string, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	rec := &runtime.StorageWrite{
		Collection: collection,
		Key:        key,
		UserID:     "",
		Value:      string(b),
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{rec})
	return err
}

func deleteFromStorage(ctx context.Context, nk runtime.NakamaModule, collection string, key string) error {
	return nk.StorageDelete(ctx, []*runtime.StorageDelete{
		{Collection: collection, Key: key, UserID: ""},
	})
}

// Notification helper
func notifyAll(ctx context.Context, nk runtime.NakamaModule, event string, payload any) {
	// payload should be serializable
	content := map[string]any{"data": payload}
	if err := nk.NotificationSendAll(ctx, event, content, 1, false); err != nil {
		// can't return error here; log via runtime logger is available in rpc entrypoints
	}
}

// wp_push_content
func rpcWpPushContent(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	// Accept either JSON object matching WPIncoming or raw map
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		logger.Error("Invalid payload JSON: %v", err)
		return "", err
	}

	pretty, _ := json.MarshalIndent(raw, "", "  ")
	logger.Info("Received wp_push_content payload:\n%s", string(pretty))

	// Identify type and id
	typeVal := ""
	if t, ok := raw["type"].(string); ok {
		typeVal = t
	}
	if typeVal == "" {
		logger.Error("Payload missing 'type'")
		return "", fmt.Errorf("missing type")
	}

	id := int(raw["id"].(float64))
	if id == 0 {
		logger.Error("Payload missing or invalid 'id'")
		return "", fmt.Errorf("missing id")
	}

	status := ""
	if s, ok := raw["status"].(string); ok {
		status = s
	}

	// Handle delete/trash
	if status == "delete" || status == "trash" {
		coll, ok := collectionByType[typeVal]
		if !ok {
			logger.Error("Unknown type on delete: %s", typeVal)
			return "", fmt.Errorf("unknown type: %s", typeVal)
		}
		key := storageKeyFor(id)
		if err := deleteFromStorage(ctx, nk, coll, key); err != nil {
			logger.Error("storage delete failed: %v", err)
			// continue to notify anyway
		}
		notifyAll(ctx, nk, "delete", map[string]any{"type": typeVal, "id": id})
		logger.Info("Deleted %s %d", typeVal, id)
		return `{"success":true}`, nil
	}

	// Handle publish/update
	// Build typed model and write to storage
	coll, ok := collectionByType[typeVal]
	if !ok {
		logger.Error("unknown content type: %s", typeVal)
		return "", fmt.Errorf("unknown type: %s", typeVal)
	}

	key := storageKeyFor(id)

	switch typeVal {
	case "event":
		ev, err := buildEventFromWP(logger, raw)
		if err != nil {
			logger.Error("build event failed: %v", err)
			return "", err
		}
		if err := writeToStorage(ctx, nk, coll, key, ev); err != nil {
			logger.Error("storage write failed: %v", err)
			return "", err
		}
		notifyAll(ctx, nk, "update", ev)
		logger.Info("Stored event %d", ev.ID)
	case "asset2d":
		as, err := buildAsset2DFromWP(logger, raw)
		if err != nil {
			logger.Error("build asset2d failed: %v", err)
			return "", err
		}
		if err := writeToStorage(ctx, nk, coll, key, as); err != nil {
			logger.Error("storage write failed: %v", err)
			return "", err
		}
		notifyAll(ctx, nk, "update", as)
		logger.Info("Stored asset2d %d", as.ID)
	case "card":
		ca, err := buildCardFromWP(logger, raw)
		if err != nil {
			logger.Error("build card failed: %v", err)
			return "", err
		}
		if err := writeToStorage(ctx, nk, coll, key, ca); err != nil {
			logger.Error("storage write failed: %v", err)
			return "", err
		}
		notifyAll(ctx, nk, "update", ca)
		logger.Info("Stored card %d", ca.ID)
	case "item":
		it, err := buildItemFromWP(logger, raw)
		if err != nil {
			logger.Error("build item failed: %v", err)
			return "", err
		}
		if err := writeToStorage(ctx, nk, coll, key, it); err != nil {
			logger.Error("storage write failed: %v", err)
			return "", err
		}
		notifyAll(ctx, nk, "update", it)
		logger.Info("Stored item %d", it.ID)
	case "quiz":
		qz, err := buildQuizFromWP(logger, raw)
		if err != nil {
			logger.Error("build quiz failed: %v", err)
			return "", err
		}
		if err := writeToStorage(ctx, nk, coll, key, qz); err != nil {
			logger.Error("storage write failed: %v", err)
			return "", err
		}
		notifyAll(ctx, nk, "update", qz)
		logger.Info("Stored quiz %d", qz.ID)
	default:
		logger.Error("unsupported type: %s", typeVal)
		return "", fmt.Errorf("unsupported type: %s", typeVal)
	}

	return `{"success":true}`, nil
}

// Client RPC to get content
// payload JSON optionally: {"type":"event"} or empty for all
func rpcGetContent(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	// parse optional filter
	var filter map[string]any
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &filter)
	}
	var requestedType string
	if filter != nil {
		if t, ok := filter["type"].(string); ok {
			requestedType = t
		}
	}

	results := []map[string]any{}

	// iterate collections
	for t, coll := range collectionByType {
		if requestedType != "" && requestedType != t {
			continue
		}
		objects, _, err := nk.StorageList(ctx, "", "", coll, 1000, "")
		if err != nil {
			logger.Error("storage list failed for %s: %v", coll, err)
			continue
		}
		for _, obj := range objects {
			// obj.Value is JSON string
			var m map[string]any
			if err := json.Unmarshal([]byte(obj.Value), &m); err != nil {
				// if cannot parse, skip
				logger.Error("storage unmarshal failed: %v", err)
				continue
			}
			results = append(results, m)
		}
	}

	resB, _ := json.Marshal(results)
	return string(resB), nil
}

// Init module
func InitContentSync(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	// Register central RPC used by WP notifier
	if err := initializer.RegisterRpc("wp_push_content", rpcWpPushContent); err != nil {
		return err
	}
	// Register client-facing fetch RPC
	if err := initializer.RegisterRpc("get_content", rpcGetContent); err != nil {
		return err
	}

	// Optionally, pre-populate storage on startup by fetching WP posts if storage empty.
	// We'll attempt to fetch each type if its collection is empty.
	for t, coll := range collectionByType {
		objects, _, err := nk.StorageList(ctx, "", "", coll, 1, "")
		if err != nil {
			logger.Error("storage list error on init: %v", err)
			continue
		}
		if len(objects) > 0 {
			logger.Info("collection %s already has data, skipping initial fetch", coll)
			continue
		}
		// fetch posts for this type
		posts, err := fetchPostsForType(logger, t)
		if err != nil {
			logger.Error("failed to fetch posts for %s: %v", t, err)
			continue
		}
		if len(posts) == 0 {
			logger.Info("no posts for %s", t)
			continue
		}
		writes := []*runtime.StorageWrite{}
		for _, p := range posts {
			logger.Info("Full post payload: %+v", p)
			idf := int(p["id"].(float64))
			var key = storageKeyFor(idf)
			switch t {
			case "event":
				logger.Info("Building event for initial fetch")
				ev, err := buildEventFromWP(logger, p)
				if err != nil {
					logger.Error("build event failed: %v", err)
					continue
				}
				b, _ := json.Marshal(ev)
				writes = append(writes, &runtime.StorageWrite{Collection: coll, Key: key, UserID: "", Value: string(b)})
			case "asset2d":
				logger.Info("Building asset2d for initial fetch")
				as, err := buildAsset2DFromWP(logger, p)
				if err != nil {
					logger.Error("build asset2d failed: %v", err)
					continue
				}
				b, _ := json.Marshal(as)
				writes = append(writes, &runtime.StorageWrite{Collection: coll, Key: key, UserID: "", Value: string(b)})
			case "card":
				logger.Info("Building card for initial fetch")
				ca, err := buildCardFromWP(logger, p)
				if err != nil {
					logger.Error("build card failed: %v", err)
					continue
				}
				b, _ := json.Marshal(ca)
				writes = append(writes, &runtime.StorageWrite{Collection: coll, Key: key, UserID: "", Value: string(b)})
			case "item":
				logger.Info("Building item for initial fetch")
				it, err := buildItemFromWP(logger, p)
				if err != nil {
					logger.Error("build item failed: %v", err)
					continue
				}
				b, _ := json.Marshal(it)
				writes = append(writes, &runtime.StorageWrite{Collection: coll, Key: key, UserID: "", Value: string(b)})
			case "quiz":
				logger.Info("Building quiz for initial fetch")
				qz, err := buildQuizFromWP(logger, p)
				if err != nil {
					logger.Error("build quiz failed: %v", err)
					continue
				}
				b, _ := json.Marshal(qz)
				writes = append(writes, &runtime.StorageWrite{Collection: coll, Key: key, UserID: "", Value: string(b)})
			}
		}
		if len(writes) > 0 {
			if _, err := nk.StorageWrite(ctx, writes); err != nil {
				logger.Error("initial storage write failed for %s: %v", coll, err)
			} else {
				logger.Info("Initial data written for %s (%d items)", coll, len(writes))
			}
		}
	}

	logger.Info("ContentSync module initialized")
	return nil
}
