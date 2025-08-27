package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "sync"
    "time"
	"database/sql"

    "github.com/heroiclabs/nakama-common/runtime"
)

// WordPress Post & Media structs
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

// Building struct with a single image URL
type Building struct {
    ID    int     `json:"id"`
    Lat   float64 `json:"lat"`
    Lon   float64 `json:"lon"`
    Image string  `json:"image"`
}

// Cache
var (
    buildingsCache []Building
    cacheMutex     sync.RWMutex
)

// WordPress endpoints
const wpCategoryID = 3
const wpPostsURL = "http://wordpress:80/wp-json/wp/v2/posts?categories=%d&per_page=100"
const wpMediaURL = "http://wordpress:80/wp-json/wp/v2/media/%d"

// Fetch image URL from media ID
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

// Fetch all buildings from WordPress
func fetchBuildingsFromWP(logger runtime.Logger) ([]Building, error) {
    url := fmt.Sprintf(wpPostsURL, wpCategoryID)
	logger.Info("Fetching WP posts from %s", url)
    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("error fetching WP posts: %w", err)
    }
    defer resp.Body.Close()

    body, _ := ioutil.ReadAll(resp.Body)

    var posts []WPPost
    if err := json.Unmarshal(body, &posts); err != nil {
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
        if imgID, ok := acf["image"].(float64); ok { // single image ID
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

// Background cache refresher
func refreshBuildingsCache(logger runtime.Logger, nk runtime.NakamaModule) {
    for {
        blds, err := fetchBuildingsFromWP(logger)
        if err != nil {
            logger.Error("Failed to fetch buildings: %v", err)
        } else {
            cacheMutex.Lock()
            buildingsCache = blds
            cacheMutex.Unlock()
            logger.Info("Buildings cache updated, %d entries", len(blds))

			content := map[string]interface{}{
				"message": "buildings_updated",
			}

            // Notify all users
            if err := nk.NotificationSend(
				context.Background(),
				"",          // userID (empty = all users)
				"",          // username
				content,     // content as map
				0,           // code (optional, can be 0)
				"buildings_update", // subject
				false,       // persistent
			); err != nil {
				logger.Error("Failed to send notification: %v", err)
			}
        }
        time.Sleep(time.Minute)
    }
}

// RPC handler
func rpcGetBuildings(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    cacheMutex.RLock()
    defer cacheMutex.RUnlock()

    data, err := json.Marshal(buildingsCache)
    if err != nil {
        return "", runtime.NewError("failed to marshal buildings cache", 3)
    }
    return string(data), nil
}

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
    if err := initializer.RegisterRpc("get_buildings", rpcGetBuildings); err != nil {
        return err
    }

    // Initial fetch
    blds, err := fetchBuildingsFromWP(logger)
    if err != nil {
        logger.Error("Initial fetch failed: %v", err)
    } else {
        cacheMutex.Lock()
        buildingsCache = blds
        cacheMutex.Unlock()
        logger.Info("Initial buildings cache populated")
    }

	// Wait for WordPress before fetching
    wpURL := fmt.Sprintf(wpPostsURL, wpCategoryID)
    if err := waitForWP(logger, wpURL, time.Minute); err != nil {
        return err
    }

    // Start periodic refresh
    go refreshBuildingsCache(logger, nk)
    return nil
}
