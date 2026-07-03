import { useLayoutEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { clearCacheForUserChange } from "./cache";

export function useUserScopedQueryCache(userID: string | null): boolean {
  const client = useQueryClient();
  const previousUserIDRef = useRef<string | null>(userID);
  const [syncedUserID, setSyncedUserID] = useState<string | null>(userID);

  useLayoutEffect(() => {
    if (syncedUserID === userID) return;
    clearCacheForUserChange(client, previousUserIDRef.current, userID);
    previousUserIDRef.current = userID;
    setSyncedUserID(userID);
  }, [client, syncedUserID, userID]);

  return syncedUserID === userID;
}
