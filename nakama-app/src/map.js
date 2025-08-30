import { Client, Session } from "@heroiclabs/nakama-js";
import L from "leaflet";

const client = new Client("defaultkey", "127.0.0.1", "7350", false);
let session = null;
let socket = null;
let markers = {}; // { playerId: { marker, lastUpdate } }
let myMarker = null;
let myGroup = null;
let buildingMarkers = [];
const CELL_SIZE = 0.002; // ~200m

var redIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-red.png"
});
var blueIcon = new L.Icon({
  iconUrl: "https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-blue.png"
});

async function fetchBuildings() {
  if (!socket) throw new Error("Socket not initialized");

  // Call the RPC we created in Nakama
  const result = await socket.rpc("get_buildings", "{}");
  const buildings = JSON.parse(result.payload);
  return buildings; // Array of {id, lat, lon, image}
}

async function addBuildingsToMap(map) {
  const buildings = await fetchBuildings();

  // Clear previous markers if needed
  buildingMarkers.forEach((m) => map.removeLayer(m));
  buildingMarkers = [];

  buildings.forEach((bld) => {
    if (bld.lat != null && bld.lon != null) {
      const icon = L.icon({
        iconUrl: bld.image ? bld.image : "default.png",
        iconSize: [40, 40],
      });
      const marker = L.marker([bld.lat, bld.lon], { icon })
        .addTo(map)
        .bindPopup(`<a href="${bld.link}" target="_blank">${bld.slug}</a>`);
      buildingMarkers.push(marker);
    }
  });
}

function getCell(lat, lon) {
  return {
    cellLat: Math.floor(lat / CELL_SIZE) * CELL_SIZE,
    cellLon: Math.floor(lon / CELL_SIZE) * CELL_SIZE,
  };
}

function getNeighborCells(cell) {
  const neighbors = [];
  for (let dLat = -1; dLat <= 1; dLat++) {
    for (let dLon = -1; dLon <= 1; dLon++) {
      neighbors.push({
        lat: cell.cellLat + dLat * CELL_SIZE,
        lon: cell.cellLon + dLon * CELL_SIZE,
      });
    }
  }
  return neighbors;
}

async function joinCell(lat, lon) {
  await socket.rpc("rpcjoincell", JSON.stringify({ lat, lon }));
}

async function leaveCell(lat, lon) {
  await socket.rpc("rpcleavecell", JSON.stringify({ lat, lon }));
}

/** Restore session from localStorage */
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

/** Connect socket */
async function initSocket() {
  socket = client.createSocket();
  await socket.connect(session, true);
  console.log("Socket connected!");
  return socket;
}

/** Setup map */
function initLeaflet(mapDivId, lat = 37.7749, lon = -122.4194) {
  const map = L.map(mapDivId).setView([lat, lon], 13);
  L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png").addTo(map);

  myMarker = L.marker([lat, lon]).addTo(map).bindPopup("You");
  return map;
}

/** Handle stream events */
function setupStreamHandlers(map) {
  socket.onstreamdata = (streamData) => {
    const msg = JSON.parse(streamData.data);
    const playerId = msg.user_id;
    const pos = msg.data;

    if (playerId === session.user_id) {
      // keep map centered on me
      //map.setView([myMarker.getLatLng().lat, myMarker.getLatLng().lng]);
      return;
    }

    const icon = msg.group ? blueIcon : redIcon;
    const now = Date.now();

    if (markers[playerId]) {
      // update existing marker if last update is older than 500ms
      markers[playerId].marker.setIcon(icon);
      if (now - markers[playerId].lastUpdate > 500) {
        markers[playerId].marker.setLatLng([pos.lat, pos.lon]);
        markers[playerId].lastUpdate = now;
      }
    } else {
      // create new marker
      const marker = L.marker([pos.lat, pos.lon], { icon })
        .addTo(map)
        .bindPopup(`Player: ${playerId}`);
      markers[playerId] = { marker, lastUpdate: Date.now() };
    }
  };

  socket.onstreampresence = (presence) => {
    presence.leaves.forEach((leave) => {
      const playerId = leave.user_id;
      if (markers[playerId]) {
        map.removeLayer(markers[playerId].marker);
        delete markers[playerId];
      }
    });
  };

  // Listen for notifications
  socket.onnotification = (notification) => {
    console.log("Received notification:", notification);
    if (notification.subject === "buildings_update") {
        console.log("Refreshing buildings...");
        addBuildingsToMap(map); // re-fetch RPC data
    }
  };

  // Periodic cleanup
  setInterval(() => {
    const now = Date.now();
    for (const playerId in markers) {
      if (now - markers[playerId].lastUpdate > 2000) {
        map.removeLayer(markers[playerId].marker);
        delete markers[playerId];
      }
    }
  }, 500);
}

/** Position update loop with optimized neighborhood change */
function startPositionUpdates(map, currentCell, lat, lon) {
  setInterval(async () => {
    // random walk (demo only)
    lat += (Math.random() - 0.5) * 0.001;
    lon += (Math.random() - 0.5) * 0.001;
    myMarker.setLatLng([lat, lon]);

    const newCell = getCell(lat, lon);

    if (
      newCell.cellLat !== currentCell.cellLat ||
      newCell.cellLon !== currentCell.cellLon
    ) {
      const dLat = Math.sign(newCell.cellLat - currentCell.cellLat);
      const dLon = Math.sign(newCell.cellLon - currentCell.cellLon);

      // leave 3 cells behind and join 3 in front
      if (dLat !== 0) {
        await leaveCell(currentCell.cellLat - dLat * CELL_SIZE, currentCell.cellLon - CELL_SIZE);
        await leaveCell(currentCell.cellLat - dLat * CELL_SIZE, currentCell.cellLon);
        await leaveCell(currentCell.cellLat - dLat * CELL_SIZE, currentCell.cellLon + CELL_SIZE);

        await joinCell(newCell.cellLat + dLat * CELL_SIZE, newCell.cellLon - CELL_SIZE);
        await joinCell(newCell.cellLat + dLat * CELL_SIZE, newCell.cellLon);
        await joinCell(newCell.cellLat + dLat * CELL_SIZE, newCell.cellLon + CELL_SIZE);
      }
      if (dLon !== 0) {
        await leaveCell(currentCell.cellLat - CELL_SIZE, currentCell.cellLon - dLon * CELL_SIZE);
        await leaveCell(currentCell.cellLat, currentCell.cellLon - dLon * CELL_SIZE);
        await leaveCell(currentCell.cellLat + CELL_SIZE, currentCell.cellLon - dLon * CELL_SIZE);

        await joinCell(newCell.cellLat - CELL_SIZE, newCell.cellLon + dLon * CELL_SIZE);
        await joinCell(newCell.cellLat, newCell.cellLon + dLon * CELL_SIZE);
        await joinCell(newCell.cellLat + CELL_SIZE, newCell.cellLon + dLon * CELL_SIZE);
      }
      currentCell = newCell;
    }

    // send location
    await socket.rpc(
      "rpcsendlocation",
      JSON.stringify({
        lat: currentCell.cellLat,
        lon: currentCell.cellLon,
        data: { lat, lon },
        group: myGroup.name,
      })
    );
  }, 1000);
}

/** Main entry */
export async function initMap(mapDivId) {
  await initSession();
  await initSocket();

  const map = initLeaflet(mapDivId);
  // Fetch buildings once at start
  await addBuildingsToMap(map);
  let currentCell = getCell(37.7749, -122.4194);

  // join initial neighborhood
  const neighbors = getNeighborCells(currentCell);
  for (const c of neighbors) {
    await joinCell(c.lat, c.lon);
  }

  setupStreamHandlers(map);

  // get the user's group
  const account = await client.getAccount(session);
  const user = account.user;
  // parse metadata
  const metadata = typeof user.metadata === "string"
    ? JSON.parse(user.metadata)
    : user.metadata;
  myGroup = metadata.group;
  startPositionUpdates(map, currentCell, 37.7749, -122.4194);
}