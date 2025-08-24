import { Client, Session } from "@heroiclabs/nakama-js";
import L from "leaflet";

const client = new Client("defaultkey", "127.0.0.1", "7350", false);
let session = null;
let socket = null;
let markers = {}; // { cellKey: { playerId: { marker, lastUpdate } } }
let myMarker = null;

const CELL_SIZE = 0.002; // ~200m

var redIcon = new L.Icon({iconUrl:'https://raw.githubusercontent.com/pointhi/leaflet-color-markers/master/img/marker-icon-red.png'});

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
  const key = `${lat},${lon}`;
  if (!markers[key]) markers[key] = {}; // prepare slot for incoming markers
  await socket.rpc("rpcjoincell", JSON.stringify({ lat, lon }));
}

async function leaveCell(map, lat, lon) {
  const key = `${lat},${lon}`;
  if (markers[key]) {
    // remove all markers sequentially
    for (const playerId of Object.keys(markers[key])) {
      map.removeLayer(markers[key][playerId].marker);
      delete markers[key][playerId];
    }
  }
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
    const cell = getCell(pos.lat, pos.lon);
    const key = `${cell.cellLat},${cell.cellLon}`;

    if (playerId === session.user_id){
      map.setView([ myMarker.getLatLng().lat, myMarker.getLatLng().lng ]);
      return;
    }

    if (!markers[key]) markers[key] = {};

    if (markers[key][playerId]) {
      markers[key][playerId].marker.setLatLng([pos.lat, pos.lon]);
      markers[key][playerId].lastUpdate = Date.now();
    } else {
      const marker = L.marker([pos.lat, pos.lon], { icon: redIcon }).addTo(map)
        .bindPopup(`Player: ${playerId}`);
      markers[key][playerId] = { marker : marker, lastUpdate: Date.now() };
    }
  };

  socket.onstreampresence = (presence) => {
    presence.leaves.forEach((leave) => {
      const playerId = leave.user_id;

      // Remove the player marker immediately from all cells
      for (const cellKey in markers) {
        if (markers[cellKey][playerId]) {
          map.removeLayer(markers[cellKey][playerId].marker);
          delete markers[cellKey][playerId];
        }
      }
    });
  };

  // Periodic cleanup
  setInterval(() => {
    const now = Date.now();
    for (const cellKey in markers) {
      for (const playerId in markers[cellKey]) {
        if (now - markers[cellKey][playerId].lastUpdate > 2000) {
          map.removeLayer(markers[cellKey][playerId].marker);
          delete markers[cellKey][playerId];
        }
      }
    }
  }, 500); // check twice per second
}

/** Position update loop with optimized neighborhood change */
function startPositionUpdates(map, currentCell, lat, lon) {
  setInterval(async () => {
    // random walk
    lat += (Math.random() - 0.5) * 0.001;
    lon += (Math.random() - 0.5) * 0.001;
    myMarker.setLatLng([lat, lon]);

    const newCell = getCell(lat, lon);

    if (
      newCell.cellLat !== currentCell.cellLat ||
      newCell.cellLon !== currentCell.cellLon
    ) {
      // detect movement direction
      const dLat = Math.sign(newCell.cellLat - currentCell.cellLat);
      const dLon = Math.sign(newCell.cellLon - currentCell.cellLon);

      // leave 3 cells behind and join the 3 in front
      if (dLat !== 0) {
        await leaveCell(map, currentCell.cellLat - dLat * CELL_SIZE, currentCell.cellLon - CELL_SIZE);
        await leaveCell(map, currentCell.cellLat - dLat * CELL_SIZE, currentCell.cellLon);
        await leaveCell(map, currentCell.cellLat - dLat * CELL_SIZE, currentCell.cellLon + CELL_SIZE);
        await joinCell(newCell.cellLat + dLat * CELL_SIZE, newCell.cellLon - CELL_SIZE);
        await joinCell(newCell.cellLat + dLat * CELL_SIZE, newCell.cellLon);
        await joinCell(newCell.cellLat + dLat * CELL_SIZE, newCell.cellLon + CELL_SIZE);
      }
      if (dLon !== 0) {
        await leaveCell(map, currentCell.cellLat - CELL_SIZE, currentCell.cellLon - dLon * CELL_SIZE);
        await leaveCell(map, currentCell.cellLat, currentCell.cellLon - dLon * CELL_SIZE);
        await leaveCell(map, currentCell.cellLat + CELL_SIZE, currentCell.cellLon - dLon * CELL_SIZE);
        await joinCell(newCell.cellLat - CELL_SIZE, newCell.cellLon + dLon * CELL_SIZE);
        await joinCell(newCell.cellLat, newCell.cellLon + dLon * CELL_SIZE);
        await joinCell(newCell.cellLat + CELL_SIZE, newCell.cellLon + dLon * CELL_SIZE);
      }
      currentCell = newCell;
    }

    await socket.rpc("rpcSendCellData",
      JSON.stringify({
        lat: currentCell.cellLat,
        lon: currentCell.cellLon,
        data: { lat, lon },
      })
    );
  }, 1000);
}

/** Main entry */
export async function initMap(mapDivId) {
  await initSession();
  await initSocket();

  const map = initLeaflet(mapDivId);
  let currentCell = getCell(37.7749, -122.4194);

  // join initial neighborhood
  const neighbors = getNeighborCells(currentCell);
  for (const c of neighbors) {
    await joinCell(c.lat, c.lon);
  }

  setupStreamHandlers(map);
  startPositionUpdates(map, currentCell, 37.7749, -122.4194);
}
