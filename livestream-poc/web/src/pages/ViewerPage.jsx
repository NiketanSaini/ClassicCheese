import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { Room, RoomEvent, Track } from "livekit-client";
import { fetchToken } from "../lib/api";
import CommentFeed from "../components/CommentFeed";

const LIVEKIT_URL = import.meta.env.VITE_LIVEKIT_URL || "ws://localhost:7880";

export default function ViewerPage() {
  const { videoId } = useParams();
  const videoRef = useRef(null);
  const audioContainerRef = useRef(null);
  const roomRef = useRef(null);
  const [status, setStatus] = useState("connecting"); // connecting | waiting | watching | error
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;

    async function watch() {
      setStatus("connecting");
      try {
        const token = await fetchToken(videoId, "subscriber");
        if (cancelled) return;

        const room = new Room();
        roomRef.current = room;

        room.on(RoomEvent.TrackSubscribed, (track) => {
          if (track.kind === Track.Kind.Video && videoRef.current) {
            track.attach(videoRef.current);
            setStatus("watching");
          } else if (track.kind === Track.Kind.Audio) {
            const el = track.attach();
            audioContainerRef.current?.appendChild(el);
          }
        });

        room.on(RoomEvent.TrackUnsubscribed, (track) => {
          track.detach();
        });

        await room.connect(LIVEKIT_URL, token);
        if (cancelled) {
          room.disconnect();
          return;
        }
        setStatus((s) => (s === "connecting" ? "waiting" : s));
      } catch (err) {
        if (cancelled) return;
        console.error(err);
        setError(err.message);
        setStatus("error");
      }
    }

    watch();

    return () => {
      cancelled = true;
      roomRef.current?.disconnect();
    };
  }, [videoId]);

  return (
    <div className="stream-page">
      <div className="stream-page__video-col">
        <h1>Watching: {videoId}</h1>
        <p className="stream-page__status">
          {status === "connecting" && "Connecting..."}
          {status === "waiting" && "Waiting for the host to go live..."}
          {status === "watching" && "Live"}
          {status === "error" && `Error: ${error}`}
        </p>
        <video ref={videoRef} autoPlay playsInline className="stream-page__video" />
        <div ref={audioContainerRef} hidden />
      </div>
      <div className="stream-page__chat-col">
        <CommentFeed videoId={videoId} />
      </div>
    </div>
  );
}
