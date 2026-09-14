import { useState } from "react";
import { useNavigate } from "react-router-dom";

export default function HomePage() {
  const [videoId, setVideoId] = useState("demo");
  const navigate = useNavigate();
  const trimmed = videoId.trim();

  return (
    <div className="home">
      <h1>Live Stream + Comments</h1>
      <p>Enter a room name, then go live or watch.</p>
      <input
        value={videoId}
        onChange={(e) => setVideoId(e.target.value)}
        placeholder="room name"
      />
      <div className="home__actions">
        <button disabled={!trimmed} onClick={() => navigate(`/host/${encodeURIComponent(trimmed)}`)}>
          Go live (host)
        </button>
        <button disabled={!trimmed} onClick={() => navigate(`/watch/${encodeURIComponent(trimmed)}`)}>
          Watch
        </button>
      </div>
    </div>
  );
}
