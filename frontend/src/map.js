/*

  Example of a simple location-based game map using Leaflet and Nakama.
  Players are represented as markers on the map, and their positions are updated in real-time.
  Buildings are fetched from the server and displayed as markers with custom icons.
  Players in the same group have blue markers, others have red markers.
  The map is centered around a specific location, and players move in a circular path for demonstration.
  Cell-based visibility is implemented to optimize performance, only showing players in relevant cells.
  
  Missing: 1. Remove player markers on disconnect.
           2. Real position updates from GPS or other sources.
           3. Error handling and reconnection logic for the socket.
           4. UI elements for better interaction (e.g., showing player names, building info).
           5. Correct assets and their positions.
           6. Map design and styling.
           etc......
           
*/


import { Client, Session } from "@heroiclabs/nakama-js";
import L from "leaflet";

const client = new Client("defaultkey", "127.0.0.1", "7350", false);
let session = null;
let socket = null;

let myMarker = null;
let myGroup = null;
let playerMarkers = new Map(); // userId -> marker
let playerLabels = new Map();  // userId -> label (divIcon)
let cellPlayers = new Map();   // cellKey -> Set(userId)
let playerCell = new Map();    // userId -> physical cellKey
let relevantCells = new Set(); // current + neighbor cells
let buildingMarkers = [];

// Icons
const redIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-red.png"
});
const blueIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-blue.png"
});
const playerIcon = new L.Icon({
  iconUrl: "http://localhost:8081/wp-content/uploads/2025/10/player.jpg",
  iconSize: [40, 40],
  iconAnchor: [20, 40],
});

const CELL_SIZE = 0.001;
const centerLat = -23.55574;
const centerLon = -46.72980;
let currentMap = null;

const mapBounds = [
  [-23.557045162755653, -46.73422584856919],
  [-23.55147505044313, -46.73130018212596],
  [-23.554510643537427, -46.72538454895334],
  [-23.55966804391334, -46.72839059081891]
]

function cellKey(lat, lon) {
  return `${parseFloat(lat).toFixed(5)},${parseFloat(lon).toFixed(5)}`;
}

function getCell(lat, lon) {
  const baseLat = Math.floor(lat / CELL_SIZE) * CELL_SIZE;
  const baseLon = Math.floor(lon / CELL_SIZE) * CELL_SIZE;
  return [baseLat, baseLon];
}

function determineCells(lat, lon) {
  const [baseLat, baseLon] = getCell(lat, lon);
  const offsetLat = lat - baseLat;
  const offsetLon = lon - baseLon;

  const keys = [cellKey(baseLat, baseLon)];

  if (offsetLat > 0) keys.push(cellKey(baseLat + CELL_SIZE, baseLon));
  else if (offsetLat < 0) keys.push(cellKey(baseLat - CELL_SIZE, baseLon));
  if (offsetLon > 0) keys.push(cellKey(baseLat, baseLon + CELL_SIZE));
  else if (offsetLon < 0) keys.push(cellKey(baseLat, baseLon - CELL_SIZE));
  if (offsetLat != 0 && offsetLon != 0) {
    keys.push(cellKey(
      baseLat + Math.sign(offsetLat) * CELL_SIZE,
      baseLon + Math.sign(offsetLon) * CELL_SIZE
    ));
  }
  return keys;
}

// Leaflet Map
function initLeaflet(mapDivId, lat = centerLat, lon = centerLon) {
  const map = L.map(mapDivId).setView([lat, lon], 17);
  L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
    minZoom: 16,    // lowest zoom
    maxZoom: 19,   // highest zoom supported by the tile server
  }).addTo(map);
  myMarker = L.marker([lat, lon], { icon: playerIcon }).addTo(map).bindPopup(`<b>You</b>`);

  const latitudes = mapBounds.map(p => p[0]);
  const longitudes = mapBounds.map(p => p[1]);
  const southWest = L.latLng(Math.min(...latitudes), Math.min(...longitudes));
  const northEast = L.latLng(Math.max(...latitudes), Math.max(...longitudes));
  const maxBoundsRect = L.latLngBounds(southWest, northEast);

  map.setMaxBounds(maxBoundsRect);
  map.on('drag', () => {
      map.panInsideBounds(maxBoundsRect, { animate: false });
  });
  
  currentMap = map;
  return map;
}

// Session / Socket
async function initSession() {
  if (session) return session;
  const sessionObj = JSON.parse(localStorage.getItem("session"));
  if (!sessionObj) return (window.location.href = "index.html");

  session = new Session(
    sessionObj.token,
    sessionObj.refresh_token,
    sessionObj.created_at,
    sessionObj.expires_at,
    sessionObj.refresh_expires_at,
    sessionObj.user_id,
    sessionObj.username
  );
  return session;
}

async function initSocket() {
  socket = client.createSocket();
  await socket.connect(session, true);
  console.log("Socket connected!");
  return socket;
}

// Buildings
async function fetchBuildings() {
  const result = await socket.rpc("get_buildings", "{}");
  return JSON.parse(result.payload);
}

async function addBuildingsToMap(map) {
  const buildings = await fetchBuildings();

  buildingMarkers.forEach(m => map.removeLayer(m));
  buildingMarkers = [];

  if (!buildings || buildings.length === 0) return;
  buildings.forEach(bld => {
    if (bld.lat != null && bld.lon != null) {
      const lat = parseFloat(bld.lat);
      const lon = parseFloat(bld.lon);
      if (isNaN(lat) || isNaN(lon)) return;

      const icon = L.icon({
        iconUrl: bld.image || "default.png",
        iconSize: [40, 40]
      });

      const marker = L.marker([lat, lon], { icon })
        .addTo(map)
        .bindPopup(`<b>Building</b>`);

      marker.options.buildingId = bld.id;
      buildingMarkers.push(marker);
    }
  });
}

// Player labels (name above marker)
function createPlayerLabel(userId, lat, lon) {
  const shortId = userId.length > 8 ? userId.slice(0, 8) + "..." : userId;
  const label = L.divIcon({
    className: "player-label",
    html: `<div style="text-align:center; color:white; font-weight:bold; text-shadow:1px 1px 2px black;">${shortId}</div>`,
    iconSize: [100, 20],
    iconAnchor: [50, 45],
  });
  const labelMarker = L.marker([lat, lon], { icon: label, interactive: false }).addTo(currentMap);
  playerLabels.set(userId, labelMarker);
}

// Player Markers
function updatePlayerMarker(userId, lat, lon, group) {
  if (!lat || !lon || isNaN(lat) || isNaN(lon)) return;

  const isSelf = userId === session.user_id;
  const icon = isSelf ? playerIcon : (group === myGroup?.name ? blueIcon : redIcon);
  
  let marker = playerMarkers.get(userId);
  if (playerMarkers.has(userId)) {
    marker.setLatLng([lat, lon]);
  } else {
    marker = L.marker([lat, lon], { icon }).addTo(currentMap)
    playerMarkers.set(userId, marker);
  }

  // Update label
  if (playerLabels.has(userId)) {
    playerLabels.get(userId).setLatLng([lat, lon]);
  } else {
    createPlayerLabel(userId, lat, lon);
  }
}

function removePlayerMarker(userId) {
  const marker = playerMarkers.get(userId);
  if (marker){
    currentMap.removeLayer(marker);
    playerMarkers.delete(userId);
  }

  const label = playerLabels.get(userId);
  if (label){
    currentMap.removeLayer(label);
    playerLabels.delete(userId);
  }

  const cell = playerCell.get(userId);
  if (cell) {
    const set = cellPlayers.get(cell);
    if (set) set.delete(userId);
    if (set && set.size === 0) cellPlayers.delete(cell);
    playerCell.delete(userId);
  }
}

// Remove markers from irrelevant cells
function cleanupCells(newRelevant) {
  for (const cell of relevantCells) {
    if (!newRelevant.has(cell)) {
      const set = cellPlayers.get(cell);
      if (set) {
        for (const userId of set){
          if (playerMarkers.get(userId)?.options.icon == blueIcon) continue;
          removePlayerMarker(userId);
        }
      }
      cellPlayers.delete(cell);
    }
  }
  relevantCells = newRelevant;
}

// Stream Handlers
function setupStreamHandlers() {
  socket.onstreamdata = (stream) => {
    try {
      const payload = JSON.parse(stream.data);
      if (payload.leave) {
        if (payload.leave !== session.user_id) removePlayerMarker(payload.leave);
        return;
      }

      const { UserID, Pos } = payload;
      if (!UserID || !Pos || UserID === session.user_id) return;

      const [cLat, cLon] = getCell(Pos.lat, Pos.lon);
      const newCell = cellKey(cLat, cLon);

      if (!relevantCells.has(newCell) && Pos.group != myGroup.name) return;

      if (!cellPlayers.has(newCell)) cellPlayers.set(newCell, new Set());
      cellPlayers.get(newCell).add(UserID);
      playerCell.set(UserID, newCell);

      updatePlayerMarker(UserID, Pos.lat, Pos.lon, Pos.group);
    } catch (err) {
      console.error("Failed handling stream data:", err);
    }
  }
  socket.onnotification = (notification) => {
    const payload = notification.content;

    if (notification.subject === "building_update") {
      const bld = payload.data;
      const lat = parseFloat(bld.lat);
      const lon = parseFloat(bld.lon);
      if (isNaN(lat) || isNaN(lon)) return;

      const existing = buildingMarkers.find(m => m.options.buildingId === bld.id);
      if (existing) {
        existing.setLatLng([lat, lon]);
        if (bld.image) existing.setIcon(L.icon({ iconUrl: bld.image, iconSize: [40, 40] }));
      } else {
        const marker = L.marker([lat, lon], {
          icon: L.icon({ iconUrl: bld.image || "default.png", iconSize: [40, 40] })
        }).addTo(currentMap)
          .bindPopup(`<a href="${bld.link || '#'}" target="_blank">${bld.title || 'Building'}</a>`);
        marker.options.buildingId = bld.id;
        buildingMarkers.push(marker);
      }
    }

    if (notification.subject === "building_delete") {
      const bld = payload.data;
      const index = buildingMarkers.findIndex(m => m.options.buildingId === bld.id);
      if (index !== -1) {
        currentMap.removeLayer(buildingMarkers[index]);
        buildingMarkers.splice(index, 1);
      }
    }
  }
}

// Position Updates
function startPositionUpdates() {
  const INTERVAL_MS = 1000;
  let lat = centerLat;
  let lon = centerLon;
  setInterval(async () => {
    if (!myMarker) return;

    lat += (Math.random() - 0.5) * 0.0002;
    lon += (Math.random() - 0.5) * 0.0002;

    myMarker.setLatLng([lat, lon]);

    // Update label position
    if (playerLabels.has(session.user_id)) {
      playerLabels.get(session.user_id).setLatLng([lat, lon]);
    } else {
      createPlayerLabel(session.user_id, lat, lon);
    }

    // keep cells in sync
    const newRelevant = new Set(determineCells(lat, lon));
    cleanupCells(newRelevant);

    const [cLat, cLon] = getCell(lat, lon);
    const physicalCell = cellKey(cLat, cLon);
    playerCell.set(session.user_id, physicalCell);
    if (!cellPlayers.has(physicalCell)) cellPlayers.set(physicalCell, new Set());
    const players = cellPlayers.get(physicalCell);
    if (!players.has(session.user_id)) players.add(session.user_id);

    try {
      const payload = { lat, lon };
      await socket.rpc("update_position", JSON.stringify(payload));
    } catch (e) {
      console.error("Failed sending position update", e);
    }
  }, INTERVAL_MS);
}

// Main
export async function initMap(mapDivId) {
  await initSession();
  await initSocket();

  const map = initLeaflet(mapDivId);
  await addBuildingsToMap(map);

  const account = await client.getAccount(session);
  const metadata = typeof account.user.metadata === "string"
    ? JSON.parse(account.user.metadata)
    : account.user.metadata;
  myGroup = metadata?.group;

  setupStreamHandlers();
  startPositionUpdates();
}