import React, { createContext, useContext, useEffect, useMemo, useState } from "react";
import { apiJson } from "../api/client";

type AuthUser = {
  id: string;
  username: string;
  role: "Admin" | "Teacher";
};

type AuthContextValue = {
  user: AuthUser | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<AuthUser>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = async () => {
    try {
      const me = await apiJson<AuthUser>("/api/v1/me", { method: "GET" });
      setUser(me);
    } catch {
      setUser(null);
    }
  };

  useEffect(() => {
    (async () => {
      setLoading(true);
      await refresh();
      setLoading(false);
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      loading,
      refresh,
      login: async (username: string, password: string) => {
        const me = await apiJson<AuthUser>("/api/v1/login", {
          method: "POST",
          body: JSON.stringify({ username, password }),
        });
        setUser(me);
        return me;
      },
      logout: async () => {
        await apiJson<{ ok: boolean }>("/api/v1/logout", { method: "POST", body: JSON.stringify({}) });
        setUser(null);
      },
    }),
    [loading, user]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
