package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/heroiclabs/nakama-common/runtime"
)

// World parameters
type Position struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

const (
	CELL_SIZE             float64 = 0.0002
	TICK_RATE             int     = 20
	UPDATES_PER_SECOND    int     = 1
	BROADCASTS_PER_SECOND int     = 2
)

var mapBounds = [][2]float64{
	{-23.557045162755653, -46.73422584856919},
	{-23.55147505044313, -46.73130018212596},
	{-23.554510643537427, -46.72538454895334},
	{-23.55966804391334, -46.72839059081891},
}

func cellIndices(lat, lon float64) (int, int) {
	return int(math.Floor(lat / CELL_SIZE)), int(math.Floor(lon / CELL_SIZE))
}

func getNeighborCells(latIndex, lonIndex int) [][2]int {
	n := make([][2]int, 0, 9)
	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			n = append(n, [2]int{latIndex + di, lonIndex + dj})
		}
	}
	return n
}

func pointInPolygon(lat, lon float64, poly [][2]float64) bool {
	inside := false
	n := len(poly)
	if n < 3 {
		return false
	}
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		intersect := ((yi > lon) != (yj > lon)) &&
			(lat < (xj-xi)*(lon-yi)/(yj-yi)+xi)
		if intersect {
			inside = !inside
		}
	}
	return inside
}

func distanceMeters(a, b Position) float64 {
	dLat := a.Lat - b.Lat
	dLon := a.Lon - b.Lon
	return math.Sqrt(dLat*dLat+dLon*dLon) * 111000
}

// Data sent to clients about users
type UserData struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	GroupID   int    `json:"group_id"`
	GroupName string `json:"group_name"`
}

// Update packet
type update struct {
	UserID string  `json:"user_id"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

// Cell and Pools
type Cell struct {
	mu      sync.Mutex
	players map[string]struct{} // set of userIDs
}

var updatePool = sync.Pool{
	New: func() any {
		buf := make([]update, 0, 64)
		return &buf
	},
}

func getUpdateSlice() *[]update {
	return updatePool.Get().(*[]update)
}

func putUpdateSlice(p *[]update) {
	*p = (*p)[:0]
	updatePool.Put(p)
}

// Player state in world
type PlayerState struct {
	UserID    string
	Username  string
	GroupID   int
	GroupName string
	Pos       Position
	CellKey   int64
	LastTick  int64
	MatchID   int
	playerMu  sync.RWMutex
}

// WorldEngine (singleton)
type WorldEngine struct {
	// Concurrent player states map
	playerState sync.Map // userID -> *PlayerState

	// Cell maps divided by match
	cellsMu []sync.RWMutex
	cells   map[int]map[int64]*Cell // matchID -> cellKey -> Cell

	// Per-match buffered updates
	updatesMu         []sync.Mutex
	cellMatchUpdates  map[int]map[int64]*[]update // matchID -> cellKey -> pointer to slice (pooled)
	groupMatchUpdates map[int]map[int]*[]update   // matchID -> groupID -> pointer to slice (pooled)
	joinUpdates       map[int]*[]UserData         // per-match join events (pooled)

}

var (
	worldOnce sync.Once
	world     *WorldEngine
)

func GetWorldEngine() *WorldEngine {
	worldOnce.Do(func() {
		world = &WorldEngine{
			//playerCell:        make(map[string]int64),
			cells:             make(map[int]map[int64]*Cell),
			cellMatchUpdates:  make(map[int]map[int64]*[]update),
			groupMatchUpdates: make(map[int]map[int]*[]update),
			joinUpdates:       make(map[int]*[]UserData),
			cellsMu:           make([]sync.RWMutex, NumMatches),
            updatesMu:         make([]sync.Mutex, NumMatches),
		}
		for matchId := range NumMatches {
			// Initialize maps
			world.cells[matchId] = make(map[int64]*Cell)
			world.cellMatchUpdates[matchId] = make(map[int64]*[]update)
			world.groupMatchUpdates[matchId] = make(map[int]*[]update)
			world.joinUpdates[matchId] = nil
		}

	})
	return world
}

// helper to encode cell key in a single int64
func cellKey(i, j int) int64 {
	return (int64(i) << 32) | int64(uint32(j))
}

func (w *WorldEngine) ensureCellExists(matchId int, cellId int64) *Cell {
	// First check without lock
	w.cellsMu[matchId].RLock()
	matchCells, ok := w.cells[matchId]
	if ok {
		if cell, ok2 := matchCells[cellId]; ok2 {
			w.cellsMu[matchId].RUnlock()
			return cell
		}
	}
	w.cellsMu[matchId].RUnlock()

	// Upgrade to write lock
	w.cellsMu[matchId].Lock()
	// Re-check match map, because another goroutine may have created it meanwhile
	matchCells, ok = w.cells[matchId]
	if !ok {
		matchCells = make(map[int64]*Cell)
		w.cells[matchId] = matchCells
	}

	// Re-check cell
	cell, ok2 := matchCells[cellId]
	if !ok2 {
		cell = &Cell{players: make(map[string]struct{})}
		matchCells[cellId] = cell
	}

	w.cellsMu[matchId].Unlock()
	return cell
}

func (w *WorldEngine) ensureMatchUpdates(matchId int, groupId *int, cellId *int64, up update) {
	w.updatesMu[matchId].Lock()
	defer w.updatesMu[matchId].Unlock()

	if cellId != nil {
		// Cell updates
		_, ok := w.cellMatchUpdates[matchId][*cellId]
		if !ok {
			w.cellMatchUpdates[matchId][*cellId] = getUpdateSlice()
		}
		*w.cellMatchUpdates[matchId][*cellId] = append(*w.cellMatchUpdates[matchId][*cellId], up)
	}

	if groupId != nil {
		// Group updates
		_, ok := w.groupMatchUpdates[matchId][*groupId]
		if !ok {
			w.groupMatchUpdates[matchId][*groupId] = getUpdateSlice()
		}
		*w.groupMatchUpdates[matchId][*groupId] = append(*w.groupMatchUpdates[matchId][*groupId], up)
	}
}

// AddPlayer registers player to world, cells remain persistent and are created lazily and retained
func (w *WorldEngine) AddPlayer(p *PlayerState) {
	w.playerState.Store(p.UserID, p)

	if p.Pos.Lat != 0 && p.Pos.Lon != 0 {
		li, lj := cellIndices(p.Pos.Lat, p.Pos.Lon)
		ck := cellKey(li, lj)
		c := w.ensureCellExists(p.MatchID, ck)

		c.mu.Lock()
		c.players[p.UserID] = struct{}{}
		c.mu.Unlock()

		p.playerMu.Lock()
		p.CellKey = ck
		p.playerMu.Unlock()
	}
}

// RemovePlayer deletes player from world
func (w *WorldEngine) RemovePlayer(userID string) {
	// remove from player maps safely
	val, ok := w.playerState.Load(userID)
	if !ok {
		return
	}
	ps := val.(*PlayerState)

	ps.playerMu.RLock()
	ck := ps.CellKey
	ps.playerMu.RUnlock()

	w.playerState.Delete(userID)

	if ck != 0 {
		// Remove from cell set
		c := w.ensureCellExists(ps.MatchID, ck)
		c.mu.Lock()
		delete(c.players, userID)
		c.mu.Unlock()
	}
}

// FetchAllPublicUsers returns UserData for all known players, used for initial global snapshot on join
func (w *WorldEngine) FetchAllPublicUsers() []UserData {
	out := make([]UserData, 0)

	w.playerState.Range(func(_, value any) bool {
		ps := value.(*PlayerState)
		out = append(out, UserData{
			UserID:    	ps.UserID,
			Username: 	ps.Username,
			GroupID:  	ps.GroupID,
			GroupName: 	ps.GroupName,
		})
		return true
	})
	return out
}


// Validate, move and append updates across matches
func (w *WorldEngine) ProcessMovement(userID string, pos Position, tick int64) {
	// quick read to fetch player state pointer
	val, ok := w.playerState.Load(userID)
	if !ok {
		return
	}
	ps := val.(*PlayerState)

	// validation
	if math.IsNaN(pos.Lat) || math.IsNaN(pos.Lon) || pos.Lat < -90 || pos.Lat > 90 || pos.Lon < -180 || pos.Lon > 180 {
		return
	}
	// rate/speed checks
	if ps.Pos.Lat != 0 && ps.Pos.Lon != 0 {
		if tick-ps.LastTick < int64(TICK_RATE/UPDATES_PER_SECOND) {
			return
		}
		if distanceMeters(ps.Pos, pos) > 30 {
			return
		}
	}
	if !pointInPolygon(pos.Lat, pos.Lon, mapBounds) {
		return
	}

	// compute old/new keys
	ps.playerMu.RLock()
	oldKey := ps.CellKey
	ps.playerMu.RUnlock()
	li, lj := cellIndices(pos.Lat, pos.Lon)
	newKey := cellKey(li, lj)

	// Determine lock order to avoid deadlocks
	if oldKey != 0 {
		if oldKey != newKey {
			// Move between cells
			newCell := w.ensureCellExists(ps.MatchID, newKey)
			oldCell := w.ensureCellExists(ps.MatchID, oldKey)
			var first, second *Cell
			if oldKey < newKey {
				first, second = oldCell, newCell
			} else {
				first, second = newCell, oldCell
			}
			// Lock in order
			first.mu.Lock()
			second.mu.Lock()
			// remove from old
			delete(oldCell.players, userID)
			// add to new
			newCell.players[userID] = struct{}{}
			// unlock in reverse order
			second.mu.Unlock()
			first.mu.Unlock()
		}
	} else {
		// Start from new cell
		newCell := w.ensureCellExists(ps.MatchID, newKey)
		newCell.mu.Lock()
		// add to new
		newCell.players[userID] = struct{}{}
		newCell.mu.Unlock()
	}
	// finally update authoritative maps
	ps.playerMu.Lock()
	ps.Pos = pos
	ps.LastTick = tick
	ps.CellKey = newKey
	ps.playerMu.Unlock()

	// Build update entry
	up := update{UserID: userID, Lat: pos.Lat, Lon: pos.Lon}
	neighbors := getNeighborCells(li, lj)
	// Append to all neighboring cells' update buffers
	for matchId := range NumMatches {
		for _, nc := range neighbors {
			nk := cellKey(nc[0], nc[1])
			w.ensureMatchUpdates(matchId, nil, &nk, up)
		}
	}
	// Also append to group-specific updates, allways contained within a single match
	w.ensureMatchUpdates(ps.MatchID, &ps.GroupID, nil, up)
}

// Returns marshaled JSON for match updates and resets the buffer
func (w *WorldEngine) FetchMatchUpdates(matchID int, cellKey *int64, groupID *int) ([]byte, error) {
	w.updatesMu[matchID].Lock()

	if groupID != nil {
		gmap := w.groupMatchUpdates[matchID]
		if gmap == nil {
			w.updatesMu[matchID].Unlock()
			return nil, nil
		}
		bufPtr := gmap[*groupID]
		if bufPtr == nil || len(*bufPtr) == 0 {
			w.updatesMu[matchID].Unlock()
			return nil, nil
		}
		delete(gmap, *groupID)
		w.updatesMu[matchID].Unlock()
		updates := *bufPtr
		payload, err := json.Marshal(updates)
		putUpdateSlice(bufPtr)
		return payload, err
	}

	if cellKey != nil {
		cmap := w.cellMatchUpdates[matchID]
		if cmap == nil {
			w.updatesMu[matchID].Unlock()
			return nil, nil
		}
		bufPtr := cmap[*cellKey]
		if bufPtr == nil || len(*bufPtr) == 0 {
			w.updatesMu[matchID].Unlock()
			return nil, nil
		}
		delete(cmap, *cellKey)
		w.updatesMu[matchID].Unlock()
		updates := *bufPtr
		payload, err := json.Marshal(updates)
		putUpdateSlice(bufPtr)
		return payload, err
	}
	return nil, nil
}

// Match implementation
type GlobalMatch struct{}

type Player struct {
	ID       string
	GroupID  int
	Presence runtime.Presence
}

type MatchState struct {
	Debug         bool
	Players       map[string]*Player
	lastBroadcast int64
	MatchID       int
}

func (m *GlobalMatch) MatchInit(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, params map[string]interface{}) (interface{}, int, string) {
	matchID := 0
	if idx, ok := params["match_id"]; ok {
		if idxi, ok2 := idx.(float64); ok2 {
			matchID = int(idxi)
		} else if idxi, ok3 := idx.(int); ok3 {
			matchID = idxi
		}
	}
	state := &MatchState{
		Debug:         true,
		Players:       make(map[string]*Player),
		lastBroadcast: 0,
		MatchID:       matchID,
	}

	label := fmt.Sprintf("global_match_%d", matchID)
	logger.Info("Initialized " + label)
	_ = GetWorldEngine()
	return state, TICK_RATE, label
}

func (m *GlobalMatch) MatchJoinAttempt(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presence runtime.Presence, metadata map[string]string) (interface{}, bool, string) {
	return state, true, ""
}

func (m *GlobalMatch) MatchJoin(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	world := GetWorldEngine()

	joins := make([]UserData, 0, len(presences))
	for _, p := range presences {
		uid := p.GetUserId()
		if _, exists := s.Players[uid]; exists {
			logger.Warn("duplicate local join " + uid)
			continue
		}
		username := p.GetUsername()
		groupID := groupManager.userToGroup[uid]
		groupName := groupManager.groups[groupID].Name
		s.Players[uid] = &Player{
			ID:       uid,
			GroupID:  groupID,
			Presence: p,
		}
		joins = append(joins, UserData{UserID: uid, Username: username, GroupID: groupID, GroupName: groupName})

		// register to world with zero position
		ws := &PlayerState{
			UserID:    uid,
			Username:  username,
			GroupID:   groupID,
			GroupName: groupName,
			Pos:       Position{Lat: 0, Lon: 0},
			CellKey:   0,
			LastTick:  0,
			MatchID:   s.MatchID,
		}
		world.AddPlayer(ws)
		logger.Info(fmt.Sprintf("%s joined match %d", username, s.MatchID))
	}

	// Send global public user snapshot (all players across matches)
	allUsers := world.FetchAllPublicUsers()
	if len(allUsers) > 0 {
		payload, _ := json.Marshal(allUsers)
		// send only to joining presences
		dispatcher.BroadcastMessage(10, payload, presences, nil, false)
	}

	// Notify all players of joins
	if len(joins) > 0 {
		payload, _ := json.Marshal(joins)
		localPres := make([]runtime.Presence, 0, len(s.Players))
		for _, pl := range s.Players {
			localPres = append(localPres, pl.Presence)
		}
		if len(localPres) > 0 {
			dispatcher.BroadcastMessage(10, payload, localPres, nil, false)
		}
		for matchid := range NumMatches {
			if matchid == s.MatchID {
				continue
			}
			world.updatesMu[matchid].Lock()
			if world.joinUpdates[matchid] == nil {
				up := make([]UserData, 0, 16)
				world.joinUpdates[matchid] = &up
			}
			*world.joinUpdates[matchid] = append(*world.joinUpdates[matchid], joins...)
			world.updatesMu[matchid].Unlock()
		}
	}
	return s
}

func (m *GlobalMatch) MatchLeave(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, presences []runtime.Presence) interface{} {
	s := state.(*MatchState)
	world := GetWorldEngine()

	for _, p := range presences {
		uid := p.GetUserId()
		delete(s.Players, uid)
		world.RemovePlayer(uid)
		logger.Info(fmt.Sprintf("%s left match %d", uid, s.MatchID))
	}
	return s
}

func (m *GlobalMatch) MatchLoop(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, messages []runtime.MatchData) interface{} {
	s := state.(*MatchState)
	world := GetWorldEngine()

	// Process client position updates (opcode == 1)
	for _, msg := range messages {
		if msg.GetOpCode() != 1 {
			continue
		}
		userID := msg.GetUserId()
		var newPos Position
		if err := json.Unmarshal(msg.GetData(), &newPos); err != nil {
			logger.Warn(fmt.Sprintf("invalid pos payload from %s: %v", userID, err))
			continue
		}
		// forward to world for authoritative validation & routing
		world.ProcessMovement(userID, newPos, tick)
	}

	// Throttle broadcasts
	if tick-s.lastBroadcast < int64(TICK_RATE/BROADCASTS_PER_SECOND) {
		return s
	}

	// Broadcast cell updates
	world.cellsMu[s.MatchID].RLock()
	cellMap := world.cells[s.MatchID] // cellKey -> *Cell
	world.cellsMu[s.MatchID].RUnlock()
	for ck, cell := range cellMap {
		// Get batched updates for cell
		payload, err := world.FetchMatchUpdates(s.MatchID, &ck, nil)
		if err != nil || len(payload) == 0 {
			continue
		}
		// Collect presences for players inside this cell
		cell.mu.Lock()
		pres := make([]runtime.Presence, 0, len(cell.players))
		for uid := range cell.players {
			if p, ok := s.Players[uid]; ok {
				pres = append(pres, p.Presence)
			}
		}
		cell.mu.Unlock()
		if len(pres) > 0 {
			dispatcher.BroadcastMessage(1, payload, pres, nil, false)
		}
	}

	// Broadcast group updates for this match
	groupMap := world.groupMatchUpdates[s.MatchID]
	for groupID := range groupMap {
		payload, err := world.FetchMatchUpdates(s.MatchID, nil, &groupID)
		if err != nil || len(payload) == 0 {
			continue
		}
		pres := make([]runtime.Presence, 0)
		for _, p := range s.Players {
			if p.GroupID == groupID {
				pres = append(pres, p.Presence)
			}
		}
		if len(pres) > 0 {
			dispatcher.BroadcastMessage(2, payload, pres, nil, false)
		}
	}

	// Broadcast join updates from other matches
	world.updatesMu[s.MatchID].Lock()
	buf := world.joinUpdates[s.MatchID]
	if buf != nil && len(*buf) > 0 {
		payload, _ := json.Marshal(*buf)
		world.joinUpdates[s.MatchID] = nil
		world.updatesMu[s.MatchID].Unlock()
		dispatcher.BroadcastMessage(10, payload, nil, nil, false)
	} else {
		world.updatesMu[s.MatchID].Unlock()
	}

	s.lastBroadcast = tick
	return s
}

func (m *GlobalMatch) MatchTerminate(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, graceSeconds int) interface{} {
	// allow termination
	return nil
}

func (m *GlobalMatch) MatchSignal(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, dispatcher runtime.MatchDispatcher, tick int64, state interface{}, data string) (interface{}, string) {
	return state, ""
}

func NewGlobalMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule) (runtime.Match, error) {
	return &GlobalMatch{}, nil
}

func rpcGetGlobalMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	groupID, ok := groupManager.userToGroup[userID]
	if !ok {
		return "", runtime.NewError("user has no group assigned", 3)
	}
	groupsPerMatch := MaxGroups / NumMatches

	matchIndex := int(groupID / groupsPerMatch)

	mm := GetMatchManager()
	matchID := mm.matchIDs[matchIndex]
	return matchID, nil
}
