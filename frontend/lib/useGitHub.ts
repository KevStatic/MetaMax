"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { useSearchParams } from "next/navigation";

export type GithubRepo = {
  full_name: string;
  html_url: string;
  description: string;
  private: boolean;
  pushed_at: string;
};

const CLIENT_ID = process.env.NEXT_PUBLIC_GITHUB_CLIENT_ID ?? "";

export function useGitHub() {
  const [connected, setConnected] = useState(false);
  const [repos, setRepos] = useState<GithubRepo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Check connection status by fetching repos — 401 means not connected
  const checkConnection = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch("/api/github/repos");
      if (res.status === 401) {
        setConnected(false);
        setRepos([]);
      } else if (res.ok) {
        const data = await res.json();
        setConnected(true);
        setRepos(data);
      }
    } catch {
      // network error — treat as disconnected
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    checkConnection();
  }, [checkConnection]);

  // Debounced search
  const search = useCallback((q: string) => {
    setQuery(q);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      setLoading(true);
      try {
        const url = q.trim()
          ? `/api/github/repos?q=${encodeURIComponent(q)}`
          : "/api/github/repos";
        const res = await fetch(url);
        if (res.ok) setRepos(await res.json());
      } catch {
        // ignore
      } finally {
        setLoading(false);
      }
    }, 350);
  }, []);

  function connect() {
    if (!CLIENT_ID) {
      setError("GitHub OAuth not configured. Set NEXT_PUBLIC_GITHUB_CLIENT_ID.");
      return;
    }
    const params = new URLSearchParams({
      client_id: CLIENT_ID,
      scope: "repo read:user",
      state: "deploy",
      redirect_uri: `${window.location.origin}/api/github/callback`,
    });
    window.location.href = `https://github.com/login/oauth/authorize?${params}`;
  }

  async function disconnect() {
    await fetch("/api/github/repos", { method: "DELETE" });
    setConnected(false);
    setRepos([]);
    setQuery("");
  }

  return { connected, repos, loading, error, query, search, connect, disconnect, checkConnection };
}
