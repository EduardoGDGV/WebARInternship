import { Client, Session } from "@heroiclabs/nakama-js";
import L from "leaflet";

const client = new Client("defaultkey", "127.0.0.1", "7350", false);
let session = null;
let socket = null;

// --- Markers ---
let buildingMarkers = []; // array of building markers with .options.buildingId
let userMarkers = {}; // { cellKey: { playerId: { marker, lastUpdate } } }
let myMarker = null;
let myGroup = null;

const CELL_SIZE = 0.002; // ~200m

// --- Icons ---
const redIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-red.png"
});
const blueIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-blue.png"
});

// --- Buildings ---
async function fetchBuildings() {
  if (!socket) throw new Error("Socket not initialized");
  const result = await socket.rpc("get_buildings", "{}");
  return JSON.parse(result.payload);
}

async function addBuildingsToMap(map) {
  const buildings = await fetchBuildings();

  // Remove old markers
  buildingMarkers.forEach(m => map.removeLayer(m));
  buildingMarkers = [];

  buildings.forEach(bld => {
    if (bld.lat != null && bld.lon != null) {
      const lat = parseFloat(bld.lat);
      const lon = parseFloat(bld.lon);
      if (isNaN(lat) || isNaN(lon)) return;

      const icon = L.icon({
        iconUrl: bld.image || "default.png",
        iconSize: [40, 40],
      });

      const marker = L.marker([lat, lon], { icon })
        .addTo(map)
        .bindPopup(`<a href="${bld.link || '#'}" target="_blank">${bld.title || 'Building'}</a>`);

      marker.options.buildingId = bld.id; // keep building ID for updates
      buildingMarkers.push(marker);
    }
  });
}

// --- Cells ---
function getCell(lat, lon) {
  return {
    lat: Math.floor(lat / CELL_SIZE) * CELL_SIZE,
    lon: Math.floor(lon / CELL_SIZE) * CELL_SIZE,
  };
}

function cellKey(cell) {
  return `${cell.lat.toFixed(6)},${cell.lon.toFixed(6)}`;
}

// --- RPCs ---
async function joinCell(lat, lon) {
  await socket.rpc("rpcjoincell", JSON.stringify({ lat, lon }));
}

async function leaveCell(lat, lon) {
  await socket.rpc("rpcleavecell", JSON.stringify({ lat, lon }));
}

// --- Session / Socket ---
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

// --- Leaflet Map ---
function initLeaflet(mapDivId, lat = 37.7749, lon = -122.4194) {
  const map = L.map(mapDivId).setView([lat, lon], 15);
  L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png").addTo(map);
  myMarker = L.marker([lat, lon]).addTo(map).bindPopup("You");
  return map;
}

// --- Stream Handlers ---
function setupStreamHandlers(map) {
  // Player position updates
  socket.onstreamdata = (streamData) => {
    const msg = JSON.parse(streamData.data);
    const playerId = msg.user_id;
    if (playerId === session.user_id) return;

    const pos = msg.data;
    const cell = getCell(msg.lat, msg.lon);
    const cKey = cellKey(cell);

    const myPos = myMarker.getLatLng();
    const d = map.distance([pos.lat, pos.lon], [myPos.lat, myPos.lng]);

    let icon = redIcon;
    if (msg.group) icon = blueIcon;
    else if (d > CELL_SIZE * 111000) {
      if (userMarkers[cKey] && userMarkers[cKey][playerId]) {
        map.removeLayer(userMarkers[cKey][playerId].marker);
        delete userMarkers[cKey][playerId];
      }
      return;
    }

    if (!userMarkers[cKey]) userMarkers[cKey] = {};

    if (userMarkers[cKey][playerId]) {
      userMarkers[cKey][playerId].marker.setIcon(icon);
      userMarkers[cKey][playerId].marker.setLatLng([pos.lat, pos.lon]);
      userMarkers[cKey][playerId].lastUpdate = Date.now();
    } else {
      const marker = L.marker([pos.lat, pos.lon], { icon })
        .addTo(map)
        .bindPopup(`Player: ${playerId}`);
      marker.options.playerId = playerId;
      userMarkers[cKey][playerId] = { marker, lastUpdate: Date.now() };
    }
  };

  // Building updates
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
        }).addTo(map)
          .bindPopup(`<a href="${bld.link || '#'}" target="_blank">${bld.title || 'Building'}</a>`);
        marker.options.buildingId = bld.id;
        buildingMarkers.push(marker);
      }
    }

    if (notification.subject === "building_delete") {
      const bld = payload.data;
      const index = buildingMarkers.findIndex(m => m.options.buildingId === bld.id);
      if (index !== -1) {
        map.removeLayer(buildingMarkers[index]);
        buildingMarkers.splice(index, 1);
      }
    }

    if (notification.subject === "buildings_update") {
      // Refresh all buildings
      addBuildingsToMap(map).catch(err => console.error("Failed to refresh buildings:", err));
    }
  };
}

// --- Position Updates ---
function determineStreams(lat, lon, cell) {
  const offsetLat = lat - cell.lat;
  const offsetLon = lon - cell.lon;
  const streams = [{ lat: cell.lat, lon: cell.lon }];

  if (offsetLat > 0) streams.push({ lat: cell.lat + CELL_SIZE, lon: cell.lon });
  else if (offsetLat < 0) streams.push({ lat: cell.lat - CELL_SIZE, lon: cell.lon });
  if (offsetLon > 0) streams.push({ lat: cell.lat, lon: cell.lon + CELL_SIZE });
  else if (offsetLon < 0) streams.push({ lat: cell.lat, lon: cell.lon - CELL_SIZE });

  if (offsetLat !== 0 && offsetLon !== 0) {
    streams.push({
      lat: cell.lat + (offsetLat > 0 ? CELL_SIZE : -CELL_SIZE),
      lon: cell.lon + (offsetLon > 0 ? CELL_SIZE : -CELL_SIZE)
    });
  }

  return streams.slice(0, 4);
}

function startPositionUpdates(map, currentCell, lat, lon) {
  let activeStreams = new Set();

  setInterval(async () => {
    lat += (Math.random() - 0.5) * 0.001;
    lon += (Math.random() - 0.5) * 0.001;
    myMarker.setLatLng([lat, lon]);

    const newCell = getCell(lat, lon);
    const needed = determineStreams(lat, lon, newCell);

    for (const c of needed) {
      const key = cellKey(c);
      if (!activeStreams.has(key)) {
        await joinCell(c.lat, c.lon);
        activeStreams.add(key);
      }
    }

    for (const key of Array.from(activeStreams)) {
      if (!needed.some(c => cellKey(c) === key)) {
        const [clat, clon] = key.split(",").map(Number);
        await leaveCell(clat, clon);
        if (userMarkers[key]) {
          for (const pid in userMarkers[key]) map.removeLayer(userMarkers[key][pid].marker);
          delete userMarkers[key];
        }
        activeStreams.delete(key);
      }
    }

    await socket.rpc("rpcsendlocation", JSON.stringify({
      lat: newCell.lat,
      lon: newCell.lon,
      data: { lat, lon },
      group: myGroup?.name
    }));

    currentCell = newCell;
  }, 1000);
}

// --- Main ---
export async function initMap(mapDivId) {
  await initSession();
  await initSocket();
  const map = initLeaflet(mapDivId);
  await addBuildingsToMap(map);

  let currentCell = getCell(37.7749, -122.4194);
  const streams = determineStreams(37.7749, -122.4194, currentCell);

  for (const c of streams) await joinCell(c.lat, c.lon);

  setupStreamHandlers(map);

  const account = await client.getAccount(session);
  const user = account.user;
  const metadata = typeof user.metadata === "string" ? JSON.parse(user.metadata) : user.metadata;
  myGroup = metadata.group;

  startPositionUpdates(map, currentCell, 37.7749, -122.4194);
}
