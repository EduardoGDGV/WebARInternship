package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"fmt"
    "time"
	"io/ioutil"
	"net/http"

	"github.com/heroiclabs/nakama-common/runtime"
)

// WordPress structs
type WPPost struct {
	ID   int                    `json:"id"`
	ACF  map[string]interface{} `json:"acf"`
	Link string                 `json:"link"`
	Slug string                 `json:"slug"`
}

type WPMedia struct {
	ID        int    `json:"id"`
	SourceURL string `json:"source_url"`
}

// Nakama Building struct
type Building struct {
	ID    int     `json:"id"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Image string  `json:"image"`
}

// WordPress endpoints
const wpCategoryID = 3
const wpPostsURL = "http://wordpress:80/wp-json/wp/v2/posts?categories=%d&per_page=100"
const wpMediaURL = "http://wordpress:80/wp-json/wp/v2/media/%d"

// Fetch image URL from WordPress media ID
func fetchImageURL(id float64) (string, error) {
	mediaID := int(id)
	url := fmt.Sprintf(wpMediaURL, mediaID)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("error fetching media: %w", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var media WPMedia
	if err := json.Unmarshal(body, &media); err != nil {
		return "", fmt.Errorf("error unmarshalling media: %w", err)
	}

	return media.SourceURL, nil
}

// Fetch all buildings from WordPress and return as []Building
func fetchBuildingsFromWP(logger runtime.Logger) ([]Building, error) {
	url := fmt.Sprintf(wpPostsURL, wpCategoryID)
	logger.Info("Fetching WP posts from %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error fetching WP posts: %w", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	// Log status and raw body for debugging
	logger.Info("WP response status: %d", resp.StatusCode)
	logger.Info("WP raw body: %s", string(body))

	var posts []WPPost
	if err := json.Unmarshal(body, &posts); err != nil {
		// Log again here in case JSON fails
		logger.Error("Failed to unmarshal WP posts. Body was: %s", string(body))
		return nil, fmt.Errorf("error unmarshalling WP posts: %w", err)
	}

	var buildings []Building
	for _, post := range posts {
		acf := post.ACF
		if acf == nil {
			continue
		}

		lat, _ := acf["lat"].(float64)
		lon, _ := acf["lon"].(float64)

		var imageURL string
		if imgID, ok := acf["image"].(float64); ok {
			url, err := fetchImageURL(imgID)
			if err != nil {
				logger.Error("Failed to fetch image for ID %v: %v", imgID, err)
			} else {
				imageURL = url
			}
		}

		buildings = append(buildings, Building{
			ID:    post.ID,
			Lat:   lat,
			Lon:   lon,
			Image: imageURL,
		})
	}

	logger.Info("Fetched %d buildings from WordPress", len(buildings))
	return buildings, nil
}

// RPC called by WordPress to push updates
func rpcWpPushBuilding(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    logger.Error("PAYLOAD: %s", payload)

    // Parse payload
    var data struct {
        ID     int     `json:"id"`
        Lat    string  `json:"lat,omitempty"`
        Lon    string  `json:"lon,omitempty"`
        Image  string  `json:"image,omitempty"`
        Title  string  `json:"title,omitempty"`
        Status string  `json:"status,omitempty"`
    }
    if err := json.Unmarshal([]byte(payload), &data); err != nil {
        logger.Error("Failed to parse building payload: %v", err)
        return "", err
    }

    key := fmt.Sprintf("%d", data.ID)

    switch data.Status {
    case "delete", "trash":
        // Remove from storage
        err := nk.StorageDelete(ctx, []*runtime.StorageDelete{
            {Collection: "buildings", Key: key, UserID: ""},
        })
        if err != nil {
            logger.Error("Failed to delete building from storage: %v", err)
            return "", err
        }

        // Notify clients
        content := map[string]interface{}{"data": map[string]interface{}{"id": data.ID}}
        if err := nk.NotificationSendAll(ctx, "building_delete", content, 1, false); err != nil {
            logger.Error("Failed to send delete notification: %v", err)
        }

    case "update", "publish":
        // Convert Lat/Lon to float
        var latF, lonF float64
        if data.Lat != "" {
            if v, err := strconv.ParseFloat(data.Lat, 64); err == nil {
                latF = v
            } else {
                logger.Error("Invalid lat value: %v", data.Lat)
            }
        }
        if data.Lon != "" {
            if v, err := strconv.ParseFloat(data.Lon, 64); err == nil {
                lonF = v
            } else {
                logger.Error("Invalid lon value: %v", data.Lon)
            }
        }

        // Save/update in storage
        b := map[string]interface{}{
            "id":    data.ID,
            "lat":   latF,
            "lon":   lonF,
            "image": data.Image,
            "title": data.Title,
            "status": data.Status,
        }
        val, _ := json.Marshal(b)
        record := &runtime.StorageWrite{
            Collection: "buildings",
            Key:        key,
            UserID:     "",
            Value:      string(val),
        }
        if _, err := nk.StorageWrite(ctx, []*runtime.StorageWrite{record}); err != nil {
            logger.Error("Failed to write building to storage: %v", err)
            return "", err
        }

        // Notify clients
        content := map[string]interface{}{"data": b}
        if err := nk.NotificationSendAll(ctx, "building_update", content, 1, false); err != nil {
            logger.Error("Failed to send update notification: %v", err)
        }

    default:
        logger.Error("Unknown status in payload: %v", data.Status)
        return "", fmt.Errorf("unknown status: %s", data.Status)
    }

    return `{"success":true}`, nil
}

// RPC for clients to fetch all buildings from Nakama Storage
func rpcGetBuildings(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	objects, _, err := nk.StorageList(ctx, "", "", "buildings", 1000, "")
	if err != nil {
		return "", err
	}

	var buildings []Building
	for _, obj := range objects {
		var b Building
		if err := json.Unmarshal([]byte(obj.Value), &b); err == nil {
			buildings = append(buildings, b)
		}
	}

	data, _ := json.Marshal(buildings)
	return string(data), nil
}

// Wait for WordPress to respond before initial fetch
func waitForWP(logger runtime.Logger, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			logger.Info("WordPress is up!")
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		logger.Info("Waiting for WordPress...")
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("WordPress did not respond in %v", timeout)
}

// Module initializer
func InitBuildings(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	// Register RPCs
	if err := initializer.RegisterRpc("wp_push_building", rpcWpPushBuilding); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("get_buildings", rpcGetBuildings); err != nil {
		return err
	}

	// Wait for WordPress
	wpURL := fmt.Sprintf(wpPostsURL, wpCategoryID)
	if err := waitForWP(logger, wpURL, time.Minute); err != nil {
		return err
	}

	// Populate storage if empty
	objects, _, err := nk.StorageList(ctx, "", "", "buildings", 1, "")
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		logger.Info("No buildings in storage, fetching initial data from WordPress...")
		buildings, err := fetchBuildingsFromWP(logger)
		if err != nil {
			return err
		}
		var writes []*runtime.StorageWrite
		for _, b := range buildings {
			val, _ := json.Marshal(b)
			writes = append(writes, &runtime.StorageWrite{
				Collection: "buildings",
				Key:        fmt.Sprintf("%d", b.ID),
                UserID:     "",
				Value:      string(val),
			})
		}
		if len(writes) > 0 {
			if _, err := nk.StorageWrite(ctx, writes); err != nil {
				return err
			}
			logger.Info("Initial buildings saved to storage")
		}
	}

	logger.Info("Buildings module initialized")
	return nil
}
