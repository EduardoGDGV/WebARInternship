import { Client } from "@heroiclabs/nakama-js";

// Catch unhandled promise rejections globally
process.on("unhandledRejection", (reason) => {
  console.warn("Unhandled Rejection (ignored):", reason);
});

const client = new Client("defaultkey", "127.0.0.1", "7350", false);

const NUM_BOTS = 800;       // total bots
const BATCH_SIZE = 5;       // spawn bots per batch
const CONCURRENCY_LIMIT = 20; // max bots simultaneously connecting
const bots = [];
let cleaningUp = false;

// Utility to wait
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Create a single bot
async function createBot(i) {
  if (cleaningUp) return;

  const email = `bot${i}@example.com`;
  const password = "botpassword";

  try {
    const session = await client.authenticateEmail(email, password, true);
    const socket = client.createSocket();
    let socketClosed = false;

    // Handle socket errors safely
    socket.onerror = (err) => {
      if (socketClosed) return;
      socketClosed = true;
      console.error(`Bot ${i} socket error:`, err);
      setTimeout(() => {
        try { socket.close(); } catch {}
      }, 100);
    };

    socket.onclose = () => {
      socketClosed = true;
    };

    try {
      await socket.connect(session, true);
      await sleep(100);
      console.log(`Bot ${i} connected`);
    } catch (e) {
      console.warn(`Bot ${i} connection attempt failed`, e);
    }

    // Initial random position
    let lat = -23.55742 + (Math.random() - 0.5) * 0.005;
    let lon = -46.73034 + (Math.random() - 0.5) * 0.005;

    async function botLoop() {
      // Random walk
      lat += (Math.random() - 0.5) * 0.0002;
      lon += (Math.random() - 0.5) * 0.0002;

      if (cleaningUp || socketClosed) return;
      try {
        await socket.rpc("update_position", JSON.stringify({ lat, lon }));
      } catch (e) {
        console.error(`Bot ${i} failed sending position:`, e);
      }

      if (!cleaningUp && !socketClosed) setTimeout(botLoop, 1000);
    }

    botLoop();
    bots.push({ session, socket });

  } catch (e) {
    console.error(`Bot ${i} failed:`, e);
  }
}

// Spawn bots in batches with concurrency control
async function spawnBots() {
  let active = 0;
  for (let i = 0; i < NUM_BOTS; i++) {
    // Wait if concurrency limit reached
    while (active >= CONCURRENCY_LIMIT) {
      await sleep(200);
    }
    active++;
    createBot(i).finally(() => active--);

    // Small delay between batches to prevent overload
    if (i % BATCH_SIZE === 0) await sleep(200);
  }

  // Wait until all active bots finish connecting
  while (active > 0) await sleep(100);
  console.log(`All ${NUM_BOTS} bots spawned`);
}

// Cleanup all bots
async function cleanupBots() {
  if (cleaningUp) return;
  cleaningUp = true;

  console.log("Cleaning up bots...");
  for (const b of bots) {
    try { b.socket.close(); } catch {}
  }
  console.log("All bots cleaned up");
  process.exit();
}

// Handle Ctrl+C
process.on("SIGINT", cleanupBots);

// Start
(async () => {
  await spawnBots();
  // Auto-cleanup after 80s
  setTimeout(cleanupBots, 80000);
})();
