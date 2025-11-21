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

// map + markers state
let myMarker = null;
let myGroup = { id: null, name: null };
let playerMarkers = new Map(); // userId -> marker
let playerLabels = new Map();  // userId -> label (divIcon)
let players = new Map();       // userId -> { username, groupId }
let groups = new Map();        // groupId -> { groupname }
let eventMarkers = [];
let currentMap = null;

// settings
const centerLat = -23.55574;
const centerLon = -46.72980;
const FOV = 20;
const TEXT_DECODER = new TextDecoder();

const mapBounds = [
  [-23.557045162755653, -46.73422584856919],
  [-23.55147505044313, -46.73130018212596],
  [-23.554510643537427, -46.72538454895334],
  [-23.55966804391334, -46.72839059081891]
]

function distanceMeters(lat1, lon1, lat2, lon2) {
  const dLat = lat1 - lat2;
  const dLon = lon1 - lon2;
  return Math.sqrt(dLat * dLat + dLon * dLon) * 111000; // approximate meters
}

// Leaflet Map
function initLeaflet(mapDivId, lat = centerLat, lon = centerLon) {
  const map = L.map(mapDivId).setView([lat, lon], 17);
  L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
    minZoom: 16,    // lowest zoom
    maxZoom: 19,   // highest zoom supported by the tile server
  }).addTo(map);
  currentMap = map;
  myMarker = L.marker([lat, lon]).addTo(map).bindPopup(`<b>You</b><br>Group: ${myGroup.name || 'None'}`);
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

    const marker = L.marker([lat, lon])
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
function updatePlayerMarker(userId, username, lat, lon, groupId) {
  if (!lat || !lon || isNaN(lat) || isNaN(lon)) return;

  // custom player icon
  // const isSelf = userId === session.user_id;
  // + color based on group

  let group_name = null;
  if (groups.has(groupId)) {
    group_name = groups.get(groupId).group_name
  } else {
    console.log("Unknown group for player:", userId, groupId);
    return; // unknown group, skip for now
  }

  let marker = playerMarkers.get(userId);
  if (playerMarkers.has(userId)) {
    marker.setLatLng([lat, lon]);
  } else {
    marker = L.marker([lat, lon])
      .addTo(currentMap)
      .bindPopup(`<b>${username}</b><br>Group: ${group_name}`);
    playerMarkers.set(userId, marker);
  }

  // Update label
  if (playerLabels.has(userId)) {
    playerLabels.get(userId).setLatLng([lat, lon]);
  } else {
    createPlayerLabel(userId, username, group_name, lat, lon);
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
}

// Prune markers farther than FOV from my current marker
function cleanupMarkers() {
  if (!myMarker) return;
  const myLatLng = myMarker.getLatLng();
  const myLat = myLatLng.lat;
  const myLon = myLatLng.lng;

  for (const [userId, p] of players.entries()) {
    if (userId === session.user_id) continue;
    if (p.lat == null || p.lon == null) continue;
    const d = distanceMeters(myLat, myLon, p.lat, p.lon);
    if (d > FOV || (p.groupId && p.groupId === myGroup.id)) {
      // remove if we have a marker for them
      removePlayerMarker(userId);
    } else {
      // if they are within range and we lack a marker, create it
      if (!playerMarkers.has(userId)) {
        updatePlayerMarker(userId, p.username, p.lat, p.lon, p.groupId);
      }
    }
  }
}

// Match Handlers
function setupSocketHandlers() {
  // get players joining/leaving the match
  socket.onmatchpresence = (matchpresence) => {
    console.log("Match presence:", matchpresence);
    
    if (matchpresence.leaves) {
      matchpresence.leaves.forEach(leave => {
        if (leave.user_id !== session.user_id) removePlayerMarker(leave.user_id);
      });
    }
  };

  // get match data
  socket.onmatchdata = (matchData) => {
    try {
      const opCode = matchData.op_code;
      const data = JSON.parse(TEXT_DECODER.decode(matchData.data));
      console.log("Match data received:", opCode, data);

      if (opCode === 10) {
        // add/update cache for each user in payload
        data.forEach(user => {
          const userId = user.user_id;
          const record = {
            username: user.username,
            groupId: user.group_id,
          };
          players.set(userId, record);
          if (user.group_id !== null && !groups.has(user.group_id) && user.group_name) {
            groups.set(user.group_id, { group_name: user.group_name });
          }

          // if the payload includes the current user, update our local group info
          if (userId === session.user_id) {
            myGroup.id = user.group_id;
            myGroup.name = user.group_name;
            // update our marker popup if present
            if (myMarker) myMarker.bindPopup(`<b>You</b><br>Group: ${myGroup.name || 'None'}`);
          }
        });

        // create markers only for nearby and group players
        cleanupMarkers();
        return;
      }

      if (opCode === 1) {
        // batched position updates
        data.forEach(update => {
          // position update { user_id, lat, lon }
          const userId = update.user_id;
          if (userId === session.user_id) return; // ignore own echo
          if (!players.has(userId)) return; // unknown player, wait for join info
          const player = players.get(userId);
          if (player.groupId === myGroup.id) return; // from group, fetch through group data (op_code 2)
          const pos = { lat: update.lat, lon: update.lon };

          // if they're within FOV, create/update a marker, otherwise remove if exists
          if (myMarker) {
            const myPos = myMarker.getLatLng();
            const d = distanceMeters(myPos.lat, myPos.lng, pos.lat, pos.lon);
            if (d <= FOV) {
              updatePlayerMarker(userId, player.username, pos.lat, pos.lon, player.groupId);
            } else {
              // remove marker if far
              if (playerMarkers.has(userId)) removePlayerMarker(userId);
            }
          } else {
            // if we don't know our location yet, create marker to be pruned later
            updatePlayerMarker(userId, player.username, pos.lat, pos.lon, player.groupId);
          }
        });
        return;
      }

      if (opCode === 2) {
        // batched group updates
        data.forEach(update => {
          // position update { user_id, lat, lon }
          const userId = update.user_id;
          if (userId === session.user_id) return; // ignore own echo
          if (myGroup.id === null) {
            myGroup.id = players.get(userId)?.groupId || null;
            myGroup.name = myGroup.id ? groups.get(myGroup.id)?.group_name || null : null;
            if (myMarker) myMarker.bindPopup(`<b>You</b><br>Group: ${myGroup.name || 'None'}`);
          }
          if (!players.has(userId)) return; // unknown player, wait for join info
          const player = players.get(userId);
          const pos = { lat: update.lat, lon: update.lon };
          // group players are always shown
          updatePlayerMarker(userId, "GROUP " + player.username, pos.lat, pos.lon, player.groupId);
        });
        return;
      }

      // ignore other opCodes
    } catch (err) {
      console.error("Error decoding match data:", err);
    }
  };

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

/*
let lastUpdateTime = 0;
const UPDATE_INTERVAL = 1000; // ms

function startPositionUpdates() {
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

    lat += (Math.random() - 0.5) * 0.0001;
    lon += (Math.random() - 0.5) * 0.0001;

    myMarker.setLatLng([lat, lon]);

    if(circle) currentMap.removeLayer(circle);
    circle = L.circle([lat, lon], {
      radius: FOV, // meters
      color: 'blue',        // outline color
      fillColor: '#30f',    // fill color
      fillOpacity: 0.2,     // transparency
    }).addTo(currentMap);

    // Update label position
    playerLabels.get(session.user_id)?.setLatLng([lat, lon]);

    try {
      const payload = { lat, lon };
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
  const res = await socket.rpc("join_global_match");
  if (!res) return;
  matchID = res.payload;
  console.log("Joining match:", matchID);
  if (!matchID) return;
  await socket.joinMatch(matchID);

  const map = initLeaflet(mapDivId);
  const events = await fetchEvents();
  addEventsToMap(map, events);

  setupSocketHandlers();
  startPositionUpdates();
}