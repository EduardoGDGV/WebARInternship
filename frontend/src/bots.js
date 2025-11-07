import { Client } from "@heroiclabs/nakama-js";

// Catch unhandled promise rejections
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
  const username = `bot${i}`;

  try {
    const session = await client.authenticateEmail(email, password, true, username);
    const socket = client.createSocket();
    let socketClosed = false;

    // Handle socket errors safely
    socket.onerror = (err) => {
      if (socketClosed) return;
      socketClosed = true;
      console.error(`Bot ${i} socket error:`, err);
      setTimeout(() => {
        try { socket.close(); } catch {
          return;
        }
      }, 100);
    };

    socket.onclose = () => {
      socketClosed = true;
    };

    try {
      await socket.connect(session, true);
      await sleep(400);
      console.log(`Bot ${i} connected`);
    } catch (e) {
      console.warn(`Bot ${i} connection attempt failed`, e);
    }

    // Initial random position
    let lat = -23.55574 + (Math.random() - 0.5) * 0.002;
    let lon = -46.72980 + (Math.random() - 0.5) * 0.002;
    let matchID = null;
    if (!socket) return;
    try {
      const res = await socket.rpc("get_match");
      if (!res) return;
      matchID = res.payload;
      console.log(`Bot ${i} joining match:`, matchID);
      if (!matchID) return;
      await socket.joinMatch(matchID);
    } catch (e) {
      console.error(`Bot ${i} failed joining match:`, e);
      return;
    }

    async function botLoop() {
      // Random walk
      lat += (Math.random() - 0.5) * 0.0002;
      lon += (Math.random() - 0.5) * 0.0002;

      if (cleaningUp || socketClosed) return;
      try {
        var opCode = 1;
        socket.sendMatchState(matchID, opCode, JSON.stringify({ lat, lon }));
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
      await sleep(250);
    }
    active++;
    createBot(i).finally(() => active--);

    // Small delay between batches to prevent overload
    if (active % BATCH_SIZE === 0 && active !== 0) await sleep(500);
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
    // Delete the bot account using its session
    if (b.session) {
      try {
        await client.deleteAccount(b.session);
        console.log(`Deleted account for ${b.session.username || b.session.user_id}`);
      } catch (err) {
        console.warn("Failed to delete account:", err);
      }
    }
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
