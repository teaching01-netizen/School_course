if (import.meta.env.DEV) {
  import("react-grab");
  import("react-scan");
}

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import { initUserInvalidSync } from "./lib/userInvalidSync";
import { ErrorBoundary } from "./components/ErrorBoundary";
import App from "./App";

initUserInvalidSync();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>
);
