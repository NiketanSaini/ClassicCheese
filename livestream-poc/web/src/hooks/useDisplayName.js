import { useState } from "react";

const STORAGE_KEY = "livestream-poc:display-name";

// No real auth for v1 — just a display name the viewer types once,
// remembered locally so it doesn't need retyping between the host and
// viewer pages.
export function useDisplayName() {
  const [name, setName] = useState(() => localStorage.getItem(STORAGE_KEY) || "");

  function updateName(next) {
    setName(next);
    localStorage.setItem(STORAGE_KEY, next);
  }

  return [name, updateName];
}
