import { STORAGE } from "@/lib/AuthContext";

export const API =
  typeof window !== "undefined"
    ? "/api/backend"
    : (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080");

export const WS_API = (process.env.NEXT_PUBLIC_WS_URL || (typeof window !== "undefined" ? `${window.location.origin}/api/backend` : (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"))).replace(/^http/, "ws");

export function getToken(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem(STORAGE.JWT) ?? "";
}

export function getWallet(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem(STORAGE.WALLET) ?? "";
}

/**
 * URL for a session's live event stream.
 *
 * A browser WebSocket cannot send an Authorization header, so the JWT goes in
 * the query string; the backend accepts it there for this endpoint only.
 */
export function sessionStreamURL(sessionId: string): string {
  const params = new URLSearchParams();
  const wallet = getWallet();
  if (wallet) params.set("wallet", wallet);
  const token = getToken();
  if (token) params.set("token", token);
  const qs = params.toString();
  return `${WS_API}/sessions/${sessionId}/stream${qs ? `?${qs}` : ""}`;
}

export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const wallet = getWallet();
  const token = getToken();
  const sep = path.includes("?") ? "&" : "?";
  const url = wallet ? `${API}${path}${sep}wallet=${encodeURIComponent(wallet)}` : `${API}${path}`;
  return fetch(url, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers as Record<string, string>),
    },
  });
}

// ── Typed API helpers ──────────────────────────────────────────────────────

export type Session = {
  id: string;
  team_id: string;
  prompt: string;
  state: "running" | "completed" | "failed";
  merkle_root?: string;
  attestation_tx?: string;
  created_at: string;
  updated_at: string;
};

export type ActionLog = {
  id: number;
  session_id: string;
  team_id: string;
  actions: ActionItem[];
  created_at: string;
};

export type ActionItem = {
  index: number;
  tool: string;
  input: Record<string, unknown>;
  result: unknown;
  error?: string;
  timestamp: string;
  hash: string;
};

export type Team = {
  id: string;
  name: string;
  owner: string;
};

export async function createSession(teamId: string, prompt: string, repoUrl?: string): Promise<Session> {
  const r = await apiFetch("/sessions", {
    method: "POST",
    body: JSON.stringify({ team_id: teamId, prompt, repo_url: repoUrl }),
  });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export async function confirmSession(sessionId: string, approved: boolean): Promise<void> {
  const r = await apiFetch(`/sessions/${sessionId}/confirm`, {
    method: "POST",
    body: JSON.stringify({ approved }),
  });
  if (!r.ok) throw new Error(await r.text());
}

export async function getSession(sessionId: string): Promise<Session> {
  const r = await apiFetch(`/sessions/${sessionId}`);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export async function getSessionAudit(sessionId: string): Promise<ActionLog> {
  const r = await apiFetch(`/sessions/${sessionId}/audit`);
  if (!r.ok) throw new Error(await r.text());
  const raw = await r.json();
  return {
    ...raw,
    actions: typeof raw.actions === "string" ? JSON.parse(raw.actions) : raw.actions ?? [],
  };
}

export async function listSessions(teamId: string): Promise<Session[]> {
  const r = await apiFetch(`/teams/${teamId}/sessions`);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}
