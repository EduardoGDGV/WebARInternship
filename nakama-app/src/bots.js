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

    // Metadata might come as a JSON string
    const metadata = typeof user.metadata === "string"
      ? JSON.parse(user.metadata)
      : user.metadata || {};

    const myGroup = metadata.group || { id: null, name: "NoGroup" };

    let lat = 37.7749 + (Math.random() - 0.5) * 0.02;
    let lon = -122.4194 + (Math.random() - 0.5) * 0.02;
    let cell = getCell(lat, lon);
    let neighbors = getNeighborCells(cell);

    for (const n of neighbors) {
      try {
        await socket.rpc("rpcjoincell", JSON.stringify({ lat: n.lat, lon: n.lon }));
      } catch {}
    }
    console.log(`Bot ${i} joined neighborhood around`, cell);

    // Register notification listener ONCE
    socket.onnotification = async (notification) => {
      if (notification.subject === "buildings_update") {
        try {
          await socket.rpc("get_buildings", "{}");
        } catch {}
      }
    };

    const interval = setInterval(async () => {
      if (cleaningUp) return; // stop sending updates if cleaning

      lat += (Math.random() - 0.5) * 0.0002;
      lon += (Math.random() - 0.5) * 0.0002;

      const newCell = getCell(lat, lon);
      if (newCell.cellLat !== cell.cellLat || newCell.cellLon !== cell.cellLon) {
        try {
          const dLat = Math.sign(newCell.cellLat - cell.cellLat);
          const dLon = Math.sign(newCell.cellLon - cell.cellLon);

          if (dLat !== 0) {
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: cell.cellLat - dLat * CELL_SIZE, lon: cell.cellLon - CELL_SIZE }));
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: cell.cellLat - dLat * CELL_SIZE, lon: cell.cellLon }));
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: cell.cellLat - dLat * CELL_SIZE, lon: cell.cellLon + CELL_SIZE }));
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: newCell.cellLat + dLat * CELL_SIZE, lon: newCell.cellLon - CELL_SIZE }));
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: newCell.cellLat + dLat * CELL_SIZE, lon: newCell.cellLon }));
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: newCell.cellLat + dLat * CELL_SIZE, lon: newCell.cellLon + CELL_SIZE }));
          }
          if (dLon !== 0) {
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: cell.cellLat - CELL_SIZE, lon: cell.cellLon - dLon * CELL_SIZE }));
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: cell.cellLat, lon: cell.cellLon - dLon * CELL_SIZE }));
            await socket.rpc("rpcleavecell", JSON.stringify({ lat: cell.cellLat + CELL_SIZE, lon: cell.cellLon - dLon * CELL_SIZE }));
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: newCell.cellLat - CELL_SIZE, lon: newCell.cellLon + dLon * CELL_SIZE }));
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: newCell.cellLat, lon: newCell.cellLon + dLon * CELL_SIZE }));
            await socket.rpc("rpcjoincell", JSON.stringify({ lat: newCell.cellLat + CELL_SIZE, lon: newCell.cellLon + dLon * CELL_SIZE }));
          }
          cell = newCell;
        } catch (e) {
          console.error(`Bot ${i} failed cell change`, e);
        }
      }

      try {
        await socket.rpc(
          "rpcsendlocation",
          JSON.stringify({
            lat: cell.cellLat,
            lon: cell.cellLon,
            data: { lat, lon },
            group: myGroup.name,
          })
        );
      } catch (e) {
        console.error(`Bot ${i} failed sending position`, e);
      }
    }, 1000);

    bots.push({ session, socket, interval, lat, lon, cell });
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

  // Stop all intervals first
  for (const b of bots) {
    clearInterval(b.interval);
  }

  // Leave neighborhoods & close sockets
  for (const b of bots) {
    let neighbors = getNeighborCells(b.cell);
    for (const n of neighbors) {
      try {
        await b.socket.rpc("rpcleavecell", JSON.stringify({ lat: n.lat, lon: n.lon }));
      } catch {}
    }
    try { b.socket.close(); } catch {}
    // Optional but heavy: delete account
    // try { await client.deleteAccount(b.session); } catch {}
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
