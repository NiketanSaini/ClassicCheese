import { useEffect, useState } from "react";
import { streamUrl } from "../lib/api";

const MAX_COMMENTS = 200;

// Opens one EventSource per videoId and keeps the last MAX_COMMENTS comments
// plus the live count in state. EventSource reconnects on its own after a
// dropped connection (e.g. a full server-side buffer), so no manual retry
// logic is needed here.
export function useCommentStream(videoId) {
  const [comments, setComments] = useState([]);
  const [count, setCount] = useState(0);

  useEffect(() => {
    if (!videoId) return;

    setComments([]);
    setCount(0);

    const source = new EventSource(streamUrl(videoId));

    source.addEventListener("comment", (event) => {
      const comment = JSON.parse(event.data);
      setComments((prev) => [...prev.slice(-(MAX_COMMENTS - 1)), comment]);
    });

    source.addEventListener("count", (event) => {
      setCount(Number(event.data));
    });

    return () => source.close();
  }, [videoId]);

  return { comments, count };
}
