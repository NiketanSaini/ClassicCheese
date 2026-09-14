import { Routes, Route } from "react-router-dom";
import HomePage from "./pages/HomePage";
import HostPage from "./pages/HostPage";
import ViewerPage from "./pages/ViewerPage";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/host/:videoId" element={<HostPage />} />
      <Route path="/watch/:videoId" element={<ViewerPage />} />
    </Routes>
  );
}
