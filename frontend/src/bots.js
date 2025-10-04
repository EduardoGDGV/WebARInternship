import { Client } from "@heroiclabs/nakama-js";

// Catch unhandled promise rejections globally
process.on("unhandledRejection", (reason) => {
  console.warn("Unhandled Rejection (ignored):", reason);
});

const client = new Client("defaultkey", "127.0.0.1", "7350", false);

const NUM_BOTS = 800;     // total bots
const BATCH_SIZE = 5;    // spawn bots per batch
const bots = [];
let cleaningUp = false;

// Create a single bot
async function createBot(i) {
  if (cleaningUp) return;

  const email = `bot${i}@example.com`;
  const password = "botpassword";

  try {
    const session = await client.authenticateEmail(email, password, true);
    const socket = client.createSocket();

    let socketClosed = false;

    socket.onerror = (err) => {
      if (socketClosed) return;
      socketClosed = true;
      console.error(`Bot ${i} socket error:`, err);
      setTimeout(() => {
        try { socket.close(); } catch {}
      }, 0);
    };

    socket.onclose = () => { socketClosed = true };

    await socket.connect(session, true);

    // Initial random position
    let lat = -23.55742 + (Math.random() - 0.5) * 0.02;
    let lon = -46.73034 + (Math.random() - 0.5) * 0.02;

    async function botLoop() {
      if (cleaningUp || socketClosed) return;

      // Random walk
      lat += (Math.random() - 0.5) * 0.0002;
      lon += (Math.random() - 0.5) * 0.0002;

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

// Spawn bots in batches
async function spawnBots() {
  for (let i = 0; i < NUM_BOTS; i += BATCH_SIZE) {
    const batch = [];
    for (let j = i; j < i + BATCH_SIZE && j < NUM_BOTS; j++) {
      batch.push(createBot(j));
    }
    await Promise.all(batch);
    console.log(`Batch ${i / BATCH_SIZE + 1} spawned`);
    await new Promise((r) => setTimeout(r, 150));
  }
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
  // Auto-cleanup after 60s
  setTimeout(cleanupBots, 60000);
})();
