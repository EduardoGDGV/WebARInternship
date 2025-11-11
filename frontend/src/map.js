/*

  Example of a simple location-based game map using Leaflet and Nakama.
  Players are represented as markers on the map, and their positions are updated in real-time.
  Buildings are fetched from the server and displayed as markers with custom icons.
  Players in the same group have blue markers, others have red markers.
  The map is centered around a specific location, and players move in a circular path for demonstration.
  Cell-based visibility is implemented to optimize performance, only showing players in relevant cells.
  
  Missing: 1. Remove player markers on disconnect.
           2. Error handling and reconnection logic for the socket.
           3. Map design and styling.
           etc......
           
*/


import { Client, Session } from "@heroiclabs/nakama-js";
import L from "leaflet";

const client = new Client("defaultkey", "127.0.0.1", "7350", false);
let session = null;
let socket = null;
let matchID = null;

let myMarker = null;
let myGroup = null;
let playerMarkers = new Map(); // userId -> marker
let playerLabels = new Map();  // userId -> label (divIcon)
let cellPlayers = new Map();   // cellKey -> Set(userId)
let playerCell = new Map();    // userId -> physical cellKey
let relevantCells = new Set(); // current + neighbor cells
let eventMarkers = [];

// Icons
const redIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-red.png",
  iconAnchor: [12, 41]
});
const blueIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-blue.png",
  iconAnchor: [12, 41]
});
const playerIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-blue.png",
  iconAnchor: [12, 41]
});

const CELL_SIZE = 0.0004;
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
  // center of current cell
  const centerLat = baseLat + CELL_SIZE / 2;
  const centerLon = baseLon + CELL_SIZE / 2;
  const offsetLat = lat - centerLat;
  const offsetLon = lon - centerLon;
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
  currentMap = map;
  myMarker = L.marker([lat, lon], { icon: playerIcon }).addTo(map).bindPopup(`<b>You</b><br>Group: ${myGroup.name || 'None'}`);
  createPlayerLabel(session.user_id, session.username, myGroup, lat, lon);

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
async function fetchEvents() {
  try {
    const result = await socket.rpc("get_content", JSON.stringify({ type: "event" }));
    const events = JSON.parse(result.payload);
    console.log("Fetched events:", events);
    return events;
  } catch (err) {
    console.error("Failed to fetch events:", err);
    return [];
  }
}

function addEventsToMap(map, events) {
  // Remove old event markers
  eventMarkers.forEach(m => map.removeLayer(m));
  eventMarkers = [];

  events.forEach(ev => {
    if (!ev.lat || !ev.lon) return;
    const lat = parseFloat(ev.lat);
    const lon = parseFloat(ev.lon);
    if (isNaN(lat) || isNaN(lon)) return;

    const icon = L.icon({
      iconUrl: ev.image || "http://localhost:8081/wp-content/uploads/default-event.png",
      iconSize: [60, 60],
    });

    const popupContent = `
      <div style="text-align:center;">
        <strong>${ev.title}</strong><br>
        ${ev.requirements?.length ? `<p>Requirements: ${ev.requirements.join(", ")}</p>` : ""}
        ${ev.rewards?.length ? `<p>Rewards: ${ev.rewards.join(", ")}</p>` : ""}
        ${ev.expire_at ? `<p>Expires: ${new Date(ev.expire_at * 1000).toLocaleString()}</p>` : ""}
      </div>
    `;

    const marker = L.marker([lat, lon], { icon })
      .addTo(map)
      .bindPopup(popupContent);

    marker.options.eventId = ev.id;
    eventMarkers.push(marker);
  });
}

// Player labels (name above marker)
function createPlayerLabel(userId, username, group, lat, lon) {
  const shortId = username.length > 8 ? username.slice(0, 8) + "..." : username;
  const label = L.divIcon({
    className: "player-label",
    html: `<div style="text-align:center; color:white; font-weight:bold; text-shadow:1px 1px 2px black;">${shortId}</div>`,
    iconSize: [100, 20],
    iconAnchor: [50, 60],
  });
  const labelMarker = L.marker([lat, lon], { icon: label, interactive: false })
    .addTo(currentMap)
    .bindPopup(`<b>${username}</b><br>Group: ${group || 'None'}`);
  playerLabels.set(userId, labelMarker);
}

// Player Markers
function updatePlayerMarker(userId, username, lat, lon, group) {
  if (!lat || !lon || isNaN(lat) || isNaN(lon)) return;

  const isSelf = userId === session.user_id;
  const icon = isSelf ? playerIcon : (group === myGroup?.name ? blueIcon : redIcon);
  
  let marker = playerMarkers.get(userId);
  if (playerMarkers.has(userId)) {
    marker.setLatLng([lat, lon]);
  } else {
    marker = L.marker([lat, lon], { icon })
      .addTo(currentMap)
      .bindPopup(`<b>${username}</b><br>Group: ${group || 'None'}`);
    playerMarkers.set(userId, marker);
  }

  // Update label
  if (playerLabels.has(userId)) {
    playerLabels.get(userId).setLatLng([lat, lon]);
  } else {
    createPlayerLabel(userId, username, group, lat, lon);
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

let cellOverlays = new Map(); // cellKey -> rectangle layer
function drawCellBorder(cellKeyStr) {
  const [lat, lon] = cellKeyStr.split(",").map(parseFloat);
  const bounds = [
    [lat, lon],
    [lat + CELL_SIZE, lon + CELL_SIZE]
  ];

  const rectangle = L.rectangle(bounds, {
    color: "#00ff00",
    weight: 1,
    fillOpacity: 0.05
  }).addTo(currentMap);

  cellOverlays.set(cellKeyStr, rectangle);
}

// Remove markers from irrelevant cells
function cleanupCells(newRelevant) {
  for (const cell of relevantCells) {
    if (!newRelevant.has(cell)) {
      const set = cellPlayers.get(cell);
      if (set) {
        for (const userId of set){
          if (playerMarkers.get(userId)?.options.icon == blueIcon || userId == session.user_id) continue;
          removePlayerMarker(userId);
        }
      }
      cellPlayers.delete(cell);

      // Remove cell rectangle
      const rect = cellOverlays.get(cell);
      if (rect) {
        currentMap.removeLayer(rect);
        cellOverlays.delete(cell);
      }
    }
  }

  // Draw new cell borders
  for (const cell of newRelevant) {
    if (!cellOverlays.has(cell)) {
      drawCellBorder(cell);
    }
  }

  relevantCells = newRelevant;
}

// Stream Handlers
function setupStreamHandlers() {
  socket.onstreampresence = (streampresence) => {
    console.log("Received presence event for stream", streampresence);
    if(streampresence.joins){
      streampresence.joins.forEach((join) => {
        console.log("New user joined: %o", join.username);
      });
    }
    if(streampresence.leaves){
      streampresence.leaves.forEach((leave) => {
        console.log("User left: %o", leave.username);
        if (leave.user_id !== session.user_id) removePlayerMarker(leave.user_id);
      });
    }
  }

  socket.onstreamdata = async (stream) => {
    console.log("Received stream data:", stream);
    try {
      const { UserID, Pos } = JSON.parse(stream.data);
      if (!UserID || !Pos || UserID === session.user_id) return;

      const users = await client.getUsers(session, UserID);
      if (!users || !users.users || users.users.length === 0) return;
      const User = users.users[0];
      const Metadata = User.metadata;
      const Group = Metadata.group || null;

      const [cLat, cLon] = getCell(Pos.lat, Pos.lon);
      const newCell = cellKey(cLat, cLon);

      if (!relevantCells.has(newCell) && Group.name && Group.name != myGroup.name) return;

      if (!cellPlayers.has(newCell)) cellPlayers.set(newCell, new Set());
      cellPlayers.get(newCell).add(UserID);
      playerCell.set(UserID, newCell);

      updatePlayerMarker(UserID, User.username, Pos.lat, Pos.lon, Group.name? Group.name : null);
    } catch (err) {
      console.error("Failed handling stream data:", err);
    }
  }

  socket.onmatchdata = async (matchData) => {
    try {
      const opCode = matchData.op_code;
      const data = JSON.parse(new TextDecoder().decode(matchData.data));

      console.log("Decoded match data:", data);
      const UserID = data.user_id;
      const Pos = { lat: data.lat, lon: data.lon };
      // handle position updates
      if (opCode === 1) {
        if (!UserID || !Pos || UserID === session.user_id) return;
        const users = await client.getUsers(session, UserID);
        if (!users || !users.users || users.users.length === 0) return;
        const User = users.users[0];
        const Metadata = User.metadata;
        const Group = Metadata.group || null;

        const [cLat, cLon] = getCell(Pos.lat, Pos.lon);
        const newCell = cellKey(cLat, cLon);

        if (!relevantCells.has(newCell) && Group.name && Group.name != myGroup.name) return;

        if (!cellPlayers.has(newCell)) cellPlayers.set(newCell, new Set());
        cellPlayers.get(newCell).add(UserID);
        playerCell.set(UserID, newCell);

        updatePlayerMarker(UserID, User.username, Pos.lat, Pos.lon, Group.name? Group.name : null);
      }

    } catch (err) {
      console.error("Error decoding match data:", err);
    }
  }

  socket.onnotification = (notification) => {
    const payload = notification.content?.data;
    const subject = notification.subject;

    // Handle WP content updates/deletes
    if (subject === "update") {
      console.log("Event update received:", payload);
      // Update or replace event marker
      const index = eventMarkers.findIndex(m => m.options.eventId === payload.id);
      if (index !== -1) {
        currentMap.removeLayer(eventMarkers[index]);
        eventMarkers.splice(index, 1);
      }
      addEventsToMap(currentMap, [payload]);
    }

    if (subject === "delete" && payload?.type === "event") {
      console.log("Event delete received:", payload);
      const index = eventMarkers.findIndex(m => m.options.eventId === payload.id);
      if (index !== -1) {
        currentMap.removeLayer(eventMarkers[index]);
        eventMarkers.splice(index, 1);
      }
    }
  }
}

let lastUpdateTime = 0;
const UPDATE_INTERVAL = 1000; // ms

/*function startPositionUpdates() {
  if (!("geolocation" in navigator)) {
    alert("Geolocation is not supported by your browser.");
    return;
  }

  // Watch the user's position
  const watchId = navigator.geolocation.watchPosition(
    async (pos) => {
       const now = Date.now();
      if (now - lastUpdateTime < UPDATE_INTERVAL) return; // skip if too soon
      lastUpdateTime = now;

      const lat = pos.coords.latitude;
      const lon = pos.coords.longitude;

      if (!myMarker) return;

      // Update your own marker position
      myMarker.setLatLng([lat, lon]);
      currentMap.panTo([lat, lon]);

      // Update name label position
      if (playerLabels.has(session.user_id)) {
        playerLabels.get(session.user_id).setLatLng([lat, lon]);
      } else {
        createPlayerLabel(session.user_id, session.username, myGroup, lat, lon);
      }

      // Update visible cells
      const newRelevant = new Set(determineCells(lat, lon));
      cleanupCells(newRelevant);

      const [cLat, cLon] = getCell(lat, lon);
      const physicalCell = cellKey(cLat, cLon);
      playerCell.set(session.user_id, physicalCell);
      if (!cellPlayers.has(physicalCell)) cellPlayers.set(physicalCell, new Set());
      cellPlayers.get(physicalCell).add(session.user_id);

      try {
        const payload = { lat, lon };
        await socket.rpc("update_position", JSON.stringify(payload));
      } catch (e) {
        console.error("Failed sending position update", e);
      }
    },
    (err) => {
      console.error("Geolocation error:", err);
      alert("Could not get location: " + err.message);
    },
    {
      enableHighAccuracy: true, // use GPS if available
      maximumAge: 0,            // do not use cached location
      timeout: 10000            // wait up to 10s for a fix
    }
  );
}
*/
// Position Updates (OLD - for simulated testing without GPS)
function startPositionUpdates() {
  const INTERVAL_MS = 1000;
  let lat = centerLat;
  let lon = centerLon;
  let circle = null;
  setInterval(async () => {
    if (!myMarker) return;

    lat += (Math.random() - 0.5) * 0.0002;
    lon += (Math.random() - 0.5) * 0.0002;

    myMarker.setLatLng([lat, lon]);

    if(circle) currentMap.removeLayer(circle);
    circle = L.circle([lat, lon], {
      radius: 20, // meters
      color: 'blue',        // outline color
      fillColor: '#30f',    // fill color
      fillOpacity: 0.2,     // transparency
    }).addTo(currentMap);

    // Update label position
    playerLabels.get(session.user_id)?.setLatLng([lat, lon]);

    // keep cells in sync
    const newRelevant = new Set(determineCells(lat, lon));
    cleanupCells(newRelevant);

    const [cLat, cLon] = getCell(lat, lon);
    const physicalCell = cellKey(cLat, cLon);
    playerCell.set(session.user_id, physicalCell);
    if (!cellPlayers.has(physicalCell)) cellPlayers.set(physicalCell, new Set());
    cellPlayers.get(physicalCell).add(session.user_id);

    try {
      const payload = { lat, lon };
      //await socket.rpc("update_position", JSON.stringify(payload));
      var opCode = 1;
      socket.sendMatchState(matchID, opCode, JSON.stringify(payload));
    } catch (e) {
      console.error("Failed sending position update", e);
    }
  }, INTERVAL_MS);
}

// Main
export async function initMap(mapDivId) {
  await initSession();
  await initSocket();
  
  if (!socket) return;
  const res = await socket.rpc("get_match");
  if (!res) return;
  matchID = res.payload;
  console.log("Joining match:", matchID);
  if (!matchID) return;
  await socket.joinMatch(matchID);

  const account = await client.getAccount(session);
  const metadata = typeof account.user.metadata === "string"
    ? JSON.parse(account.user.metadata)
    : account.user.metadata;
  myGroup = metadata?.group;

  const map = initLeaflet(mapDivId);
  const events = await fetchEvents();
  addEventsToMap(map, events);

  setupStreamHandlers();
  startPositionUpdates();
}