"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

import { logout as logoutRequest, refresh as refreshRequest } from "@/lib/api/endpoints";

import { clearTokens, getAccessToken, getRefreshToken, setTokens, type TokenPair } from "./token-store";

export type SessionStatus = "loading" | "authed" | "anon";

export interface SessionUser {
  id: string;
  email: string;
}

interface SessionState {
  status: SessionStatus;
  user: SessionUser | null;
}

interface SessionContextValue extends SessionState {
  // Called by the login/register pages once they have a fresh token pair.
  applyTokens: (tokens: TokenPair) => void;
  logoutSession: () => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

function base64UrlDecode(input: string): string {
  const base64 = input.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=");
  return atob(padded);
}

// Decodes the access token's payload client-side for display purposes only
// (no signature verification — the backend is the source of truth for auth).
function decodeUser(accessToken: string): SessionUser | null {
  try {
    const payload = accessToken.split(".")[1];
    if (!payload) return null;
    const claims = JSON.parse(base64UrlDecode(payload)) as { sub?: string; email?: string };
    if (!claims.sub) return null;
    return { id: claims.sub, email: claims.email ?? "" };
  } catch {
    return null;
  }
}

function stateFromAccessToken(accessToken: string | null): SessionState {
  if (!accessToken) return { status: "anon", user: null };
  const decoded = decodeUser(accessToken);
  return decoded ? { status: "authed", user: decoded } : { status: "anon", user: null };
}

// Resolves the state that's knowable synchronously on mount, without any
// setState-in-effect: an in-memory access token (rare — only survives a
// client-side remount, since it's wiped on every full page load) settles
// immediately; otherwise stay "loading" iff a refresh token is persisted, so
// the effect below has something to rehydrate. No tokens at all -> "anon"
// immediately, nothing to wait for.
function computeInitialState(): SessionState {
  const existing = getAccessToken();
  if (existing) return stateFromAccessToken(existing);
  return getRefreshToken() ? { status: "loading", user: null } : { status: "anon", user: null };
}

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<SessionState>(computeInitialState);

  const applyAccessToken = useCallback((accessToken: string | null) => {
    setState(stateFromAccessToken(accessToken));
  }, []);

  const applyTokens = useCallback(
    (tokens: TokenPair) => {
      setTokens(tokens);
      applyAccessToken(tokens.accessToken);
    },
    [applyAccessToken],
  );

  const logoutSession = useCallback(async () => {
    const refreshToken = getRefreshToken();
    try {
      if (refreshToken) {
        await logoutRequest({ refreshToken });
      }
    } finally {
      clearTokens();
      applyAccessToken(null);
    }
  }, [applyAccessToken]);

  useEffect(() => {
    // Both branches below were already resolved synchronously by
    // computeInitialState — nothing to rehydrate on this mount.
    if (getAccessToken()) return;
    const refreshToken = getRefreshToken();
    if (!refreshToken) return;

    let cancelled = false;
    refreshRequest({ refreshToken })
      .then((tokens) => {
        if (cancelled) return;
        setTokens(tokens);
        applyAccessToken(tokens.accessToken);
      })
      .catch(() => {
        if (cancelled) return;
        clearTokens();
        applyAccessToken(null);
      });

    return () => {
      cancelled = true;
    };
  }, [applyAccessToken]);

  const value = useMemo(
    () => ({ status: state.status, user: state.user, applyTokens, logoutSession }),
    [state, applyTokens, logoutSession],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) {
    throw new Error("useSession must be used within a SessionProvider");
  }
  return ctx;
}
