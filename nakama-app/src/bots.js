import { Client } from "@heroiclabs/nakama-js";

const client = new Client("defaultkey", "127.0.0.1", "7350", false);
const NUM_BOTS = 800;          // total number of bots
const BATCH_SIZE = 100;        // spawn this many bots at a time
const bots = [];
const CELL_SIZE = 0.002;       // cell size for RPC joins

let cleaningUp = false;        // flag to stop intervals when cleaning

// --- Cell utilities ---
function getCell(lat, lon) {
  return {
    cellLat: Math.floor(lat / CELL_SIZE) * CELL_SIZE,
    cellLon: Math.floor(lon / CELL_SIZE) * CELL_SIZE
  };
}

function cellKey(c) {
  return `${c.lat},${c.lon}`;
}

function determineStreams(lat, lon, cell) {
  const offsetLat = lat - cell.cellLat;
  const offsetLon = lon - cell.cellLon;
  const streams = [{ lat: cell.cellLat, lon: cell.cellLon }];

  if (offsetLat > 0) streams.push({ lat: cell.cellLat + CELL_SIZE, lon: cell.cellLon });
  else if (offsetLat < 0) streams.push({ lat: cell.cellLat - CELL_SIZE, lon: cell.cellLon });

  if (offsetLon > 0) streams.push({ lat: cell.cellLat, lon: cell.cellLon + CELL_SIZE });
  else if (offsetLon < 0) streams.push({ lat: cell.cellLat, lon: cell.cellLon - CELL_SIZE });

  if (offsetLat !== 0 && offsetLon !== 0) {
    streams.push({ lat: cell.cellLat + (offsetLat > 0 ? CELL_SIZE : -CELL_SIZE),
                  lon: cell.cellLon + (offsetLon > 0 ? CELL_SIZE : -CELL_SIZE) });
  }

  return streams.slice(0, 4); // max 4 streams
}

// --- Create a single bot ---
async function createBot(i) {
  if (cleaningUp) return;

  const email = `bot${i}@example.com`;
  const password = "botpassword";

  try {
    const session = await client.authenticateEmail(email, password, true);
    const socket = client.createSocket();
    await socket.connect(session, true);

    const account = await client.getAccount(session);
    const user = account.user;

    const metadata = typeof user.metadata === "string"
      ? JSON.parse(user.metadata)
      : user.metadata || {};

    const myGroup = metadata.group || { id: null, name: "NoGroup" };

    let lat = 37.7749 + (Math.random() - 0.5) * 0.02;
    let lon = -122.4194 + (Math.random() - 0.5) * 0.02;
    let cell = getCell(lat, lon);
    let activeStreams = new Set();

    // join initial streams
    const initialStreams = determineStreams(lat, lon, cell);
    for (const s of initialStreams) {
      try {
        await socket.rpc("rpcjoincell", JSON.stringify({ lat: s.lat, lon: s.lon }));
        activeStreams.add(cellKey(s));
      } catch {}
    }
    console.log(`Bot ${i} joined streams around`, cell);

    // Register notification listener ONCE
    socket.onnotification = async (notification) => {
      if (notification.subject === "buildings_update") {
        try {
          await socket.rpc("get_buildings", "{}");
        } catch {}
      }
    };

    const interval = setInterval(async () => {
      if (cleaningUp) return;

      // random walk
      lat += (Math.random() - 0.5) * 0.0002;
      lon += (Math.random() - 0.5) * 0.0002;

      const newCell = getCell(lat, lon);
      const needed = determineStreams(lat, lon, newCell);

      // join new streams
      for (const s of needed) {
        const key = cellKey(s);
        if (!activeStreams.has(key)) {
          try {
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: s.lat, lon: s.lon }));
            activeStreams.add(key);
          } catch {}
        }
      }

      // leave unused streams
      for (const key of Array.from(activeStreams)) {
        if (!needed.some(s => cellKey(s) === key)) {
          const [clat, clon] = key.split(",").map(Number);
          try {
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: clat, lon: clon }));
          } catch {}
          activeStreams.delete(key);
        }
      }

      // send location
      try {
        await socket.rpc(
          "rpcsendlocation",
          JSON.stringify({
            lat: newCell.cellLat,
            lon: newCell.cellLon,
            data: { lat, lon },
            group: myGroup.name,
          })
        );
      } catch (e) {
        console.error(`Bot ${i} failed sending position`, e);
      }

      cell = newCell;
    }, 1000);

    bots.push({ session, socket, interval, cell, activeStreams });
  } catch (e) {
    console.error(`Bot ${i} failed:`, e);
  }
}

// --- Spawn bots in batches ---
async function spawnBots() {
  for (let i = 0; i < NUM_BOTS; i += BATCH_SIZE) {
    const promises = [];
    for (let j = i; j < i + BATCH_SIZE && j < NUM_BOTS; j++) {
      promises.push(createBot(j));
    }
    await Promise.all(promises);
    console.log(`Batch ${i / BATCH_SIZE + 1} spawned`);
    await new Promise((r) => setTimeout(r, 2000));
  }
}

// --- Cleanup all bots ---
async function cleanupBots() {
  if (cleaningUp) return;
  cleaningUp = true;

  console.log("Cleaning up bots...");

  for (const b of bots) {
    clearInterval(b.interval);

    // leave all active streams
    for (const key of Array.from(b.activeStreams)) {
      const [clat, clon] = key.split(",").map(Number);
      try {
        await b.socket.rpc("rpcleavecell", JSON.stringify({ lat: clat, lon: clon }));
      } catch {}
    }

    try { b.socket.close(); } catch {}
  }

  console.log("All bots cleaned up");
  process.exit();
}

// --- Handle Ctrl+C ---
process.on("SIGINT", async () => {
  await cleanupBots();
});

// --- Start ---
(async () => {
  await spawnBots();

  // Auto-cleanup after 60s
  setTimeout(async () => {
    await cleanupBots();
  }, 60000);
})();
