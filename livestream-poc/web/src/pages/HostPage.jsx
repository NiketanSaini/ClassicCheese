import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { Room } from "livekit-client";
import { fetchToken } from "../lib/api";
import CommentFeed from "../components/CommentFeed";

const LIVEKIT_URL = import.meta.env.VITE_LIVEKIT_URL || "ws://localhost:7880";

export default function HostPage() {
  const { videoId } = useParams();
  const videoRef = useRef(null);
  const roomRef = useRef(null);
  const [status, setStatus] = useState("idle"); // idle | connecting | live | error
  const [error, setError] = useState(null);

  useEffect(() => {
    let stream;
    let cancelled = false;

    async function goLive() {
      setStatus("connecting");
      try {
        const token = await fetchToken(videoId, "publisher");
        if (cancelled) return;

        const room = new Room();
        roomRef.current = room;
        await room.connect(LIVEKIT_URL, token);
        if (cancelled) return;

        stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: true });
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }

        if (videoRef.current) {
          videoRef.current.srcObject = stream;
        }
        await Promise.all(
          stream.getTracks().map((track) => room.localParticipant.publishTrack(track))
        );

        setStatus("live");
      } catch (err) {
        if (cancelled) return;
        console.error(err);
        setError(err.message);
        setStatus("error");
      }
    }

    goLive();

    return () => {
      cancelled = true;
      stream?.getTracks().forEach((t) => t.stop());
      roomRef.current?.disconnect();
    };
  }, [videoId]);

  return (
    <div className="stream-page">
      <div className="stream-page__video-col">
        <h1>Live: {videoId}</h1>
        <p className="stream-page__status">
          {status === "connecting" && "Connecting..."}
          {status === "live" && "You are live"}
          {status === "error" && `Error: ${error}`}
        </p>
        <video ref={videoRef} autoPlay muted playsInline className="stream-page__video" />
      </div>
      <div className="stream-page__chat-col">
        <CommentFeed videoId={videoId} />
      </div>
    </div>
  );
}
