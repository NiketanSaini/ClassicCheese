import { useEffect, useRef, useState } from "react";
import { useCommentStream } from "../hooks/useCommentStream";
import { useDisplayName } from "../hooks/useDisplayName";
import { postComment } from "../lib/api";

export default function CommentFeed({ videoId }) {
  const { comments, count } = useCommentStream(videoId);
  const [author, setAuthor] = useDisplayName();
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const listRef = useRef(null);

  useEffect(() => {
    const list = listRef.current;
    if (list) list.scrollTop = list.scrollHeight;
  }, [comments]);

  async function handleSubmit(event) {
    event.preventDefault();
    const trimmedAuthor = author.trim();
    const trimmedText = text.trim();
    if (!trimmedAuthor || !trimmedText) return;

    setSending(true);
    try {
      await postComment(videoId, trimmedAuthor, trimmedText);
      setText("");
    } catch (err) {
      console.error(err);
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="comment-feed">
      <div className="comment-feed__header">
        {count} comment{count === 1 ? "" : "s"}
      </div>

      <ul className="comment-feed__list" ref={listRef}>
        {comments.map((c) => (
          <li key={c.id}>
            <span className="comment-feed__author">{c.author}</span> {c.text}
          </li>
        ))}
      </ul>

      <form className="comment-feed__form" onSubmit={handleSubmit}>
        <input
          className="comment-feed__name-input"
          value={author}
          onChange={(e) => setAuthor(e.target.value)}
          placeholder="Your name"
          maxLength={64}
        />
        <input
          className="comment-feed__text-input"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Say something..."
          maxLength={500}
        />
        <button type="submit" disabled={sending}>
          Send
        </button>
      </form>
    </div>
  );
}
