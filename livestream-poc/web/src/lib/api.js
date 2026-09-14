const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || "http://localhost:8090";

export async function fetchToken(videoId, role) {
  const url = `${API_BASE_URL}/token?room=${encodeURIComponent(videoId)}&role=${role}`;
  const res = await fetch(url, { method: "POST" });
  if (!res.ok) {
    throw new Error(`token request failed: ${res.status} ${await res.text()}`);
  }
  return res.text();
}

export async function postComment(videoId, author, text) {
  const res = await fetch(`${API_BASE_URL}/comments`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ video_id: videoId, author, text }),
  });
  if (!res.ok) {
    throw new Error(`post comment failed: ${res.status} ${await res.text()}`);
  }
  return res.json();
}

export function streamUrl(videoId) {
  return `${API_BASE_URL}/stream?room=${encodeURIComponent(videoId)}`;
}
