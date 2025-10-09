import { Client } from "@heroiclabs/nakama-js";

// Initialize Nakama client
const client = new Client("defaultkey", "127.0.0.1", "7350", false);
let session = null;

// Sign Up with email/password
export async function signUp(email, password) {
  try {
    session = await client.authenticateEmail(email, password, true);
    localStorage.setItem("session", JSON.stringify(session));
    console.log("Signed up:", session);
    window.location.href = "src/map.html"; // redirect to map page
    return session;
  } catch (err) {
    console.error("SignUp failed:", err);
    throw err;
  }
}

// Log In with email/password
export async function logIn(email, password) {
  try {
    session = await client.authenticateEmail(email, password, false);
    localStorage.setItem("session", JSON.stringify(session));
    console.log("Signed up:", session);
    window.location.href = "src/map.html"; // redirect to map page
    return session;
  } catch (err) {
    console.error("Login failed:", err);
    throw err;
  }
}