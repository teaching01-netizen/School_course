import { useEffect, useState } from "react";

export type ConnectivityState = {
  online: boolean;
  justRestored: boolean;
};

export function useConnectivity(): ConnectivityState {
  const [online, setOnline] = useState(() => (typeof navigator === "undefined" ? true : navigator.onLine));
  const [justRestored, setJustRestored] = useState(false);

  useEffect(() => {
    const handleOnline = () => {
      setOnline(true);
      setJustRestored(true);
    };
    const handleOffline = () => {
      setOnline(false);
      setJustRestored(false);
    };

    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOffline);
    return () => {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOffline);
    };
  }, []);

  useEffect(() => {
    if (!justRestored) return;
    const timer = window.setTimeout(() => setJustRestored(false), 2500);
    return () => window.clearTimeout(timer);
  }, [justRestored]);

  return { online, justRestored };
}
