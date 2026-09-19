"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import { timeSince } from "@/lib/utils";

type Session = {
  id: string;
  prompt: string;
  state: "running" | "completed" | "failed";
  created_at: string;
  updated_at: string;
};

const stateColor: Record<string, string> = {
  running: "#eab308",
  completed: "#22c55e",
  failed: "#ef4444",
};

export default function SessionsPage() {
  const { teamId } = useAuth();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!teamId) { setLoading(false); return; }
    apiFetch(`/teams/${teamId}/sessions`)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error("Failed to load"))))
      .then((data) => setSessions(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [teamId]);

  const running = sessions.filter((s) => s.state === "running").length;
  const completed = sessions.filter((s) => s.state === "completed").length;
  const failed = sessions.filter((s) => s.state === "failed").length;

  return (
    <div
      style={{
        display: "flex",
        height: "100vh",
        background: "#111111",
        fontFamily: "Inter, var(--font-inter), sans-serif",
        color: "#e5e7eb",
      }}
    >
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 16, marginBottom: 32 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Sessions</p>
              <p style={{ fontSize: 13, color: "#6b7280", marginTop: 4 }}>AI-agent deployment sessions</p>
            </div>
            <Link
              href="/deploy"
              style={{ display: "flex", alignItems: "center", height: 40, padding: "0 16px", borderRadius: 8, background: "#e2f0d9", color: "#111111", fontSize: 13, fontWeight: 700, textDecoration: "none" }}
            >
              + New Deployment
            </Link>
          </header>

          {/* Stats row */}
          {!loading && !error && (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, marginBottom: 32 }}>
              {[
                { label: "Total Sessions", value: sessions.length, color: "#f9fafb" },
                { label: "Completed", value: completed, color: "#22c55e" },
                { label: "Running", value: running, color: "#eab308" },
              ].map((s) => (
                <div key={s.label} style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.05)", borderRadius: 12, padding: "20px 24px" }}>
                  <p style={{ fontSize: 11, color: "#6b7280", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 8 }}>{s.label}</p>
                  <p style={{ fontSize: 32, fontWeight: 300, color: s.color }}>{s.value}</p>
                </div>
              ))}
            </div>
          )}

          {/* Session list */}
          <div style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.05)", borderRadius: 12, overflow: "hidden" }}>
            {loading && (
              <div style={{ padding: 40, textAlign: "center", color: "#6b7280", fontSize: 14 }}>Loading sessions…</div>
            )}
            {error && (
              <div style={{ padding: 40, textAlign: "center", color: "#ef4444", fontSize: 14 }}>{error}</div>
            )}
            {!loading && !error && sessions.length === 0 && (
              <div style={{ padding: 40, textAlign: "center", color: "#4b5563", fontSize: 14 }}>
                No sessions yet.{" "}
                <Link href="/deploy" style={{ color: "#e2f0d9", textDecoration: "underline" }}>Deploy your first app →</Link>
              </div>
            )}
            {!loading && sessions.map((s, i) => (
              <Link
                key={s.id}
                href={`/sessions/${s.id}`}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "16px 24px",
                  borderBottom: i < sessions.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none",
                  textDecoration: "none",
                  color: "inherit",
                }}
              >
                <div style={{ display: "flex", alignItems: "center", gap: 16, minWidth: 0 }}>
                  <div style={{ width: 8, height: 8, borderRadius: "50%", background: stateColor[s.state] ?? "#6b7280", flexShrink: 0 }} />
                  <div style={{ minWidth: 0 }}>
                    <p style={{ fontSize: 14, color: "#f3f4f6", fontWeight: 500, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                      {s.prompt.slice(0, 80)}{s.prompt.length > 80 ? "…" : ""}
                    </p>
                    <p style={{ fontSize: 11, color: "#4b5563", fontFamily: "var(--font-space-mono), monospace", marginTop: 2 }}>
                      {s.id.slice(0, 26)}…
                    </p>
                  </div>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 24, flexShrink: 0, marginLeft: 16 }}>
                  <span style={{ fontSize: 11, color: "#6b7280" }}>{timeSince(s.created_at)}</span>
                  <span style={{ fontSize: 12, fontWeight: 600, color: stateColor[s.state] ?? "#6b7280", textTransform: "capitalize", minWidth: 70, textAlign: "right" }}>
                    {s.state}
                  </span>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#4b5563" strokeWidth="2"><polyline points="9 18 15 12 9 6"/></svg>
                </div>
              </Link>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
