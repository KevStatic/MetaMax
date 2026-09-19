"use client";

import { useState } from "react";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";

const ACCENT = "#e2f0d9";

type Rental = {
  id: string;
  user: string;
  sessionId: string;
  started: string;
  ended: string | null;
  status: "active" | "completed" | "failed";
  earnedUsdc: string;
  luks: "sealed" | "open";
};

const MOCK_RENTALS: Rental[] = [
  { id: "rnt_001", user: "0x1a2b…3c4d", sessionId: "sess-demo-001", started: "Apr 30 10:32", ended: "Apr 30 11:04", status: "completed", earnedUsdc: "0.0042", luks: "sealed" },
  { id: "rnt_002", user: "0x5e6f…7a8b", sessionId: "sess-demo-002", started: "Apr 30 12:00", ended: null,             status: "active",    earnedUsdc: "0.0011", luks: "open" },
  { id: "rnt_003", user: "0x9c0d…1e2f", sessionId: "sess-demo-003", started: "Apr 29 09:15", ended: "Apr 29 09:45", status: "completed", earnedUsdc: "0.0031", luks: "sealed" },
  { id: "rnt_004", user: "0x3a4b…5c6d", sessionId: "sess-demo-004", started: "Apr 28 16:40", ended: "Apr 28 16:52", status: "failed",    earnedUsdc: "0.0000", luks: "sealed" },
];

const STATUS_STYLE: Record<string, { text: string; bg: string }> = {
  active:    { text: ACCENT,    bg: "rgba(226,240,217,0.1)" },
  completed: { text: "#22c55e", bg: "rgba(34,197,94,0.10)"  },
  failed:    { text: "#ef4444", bg: "rgba(239,68,68,0.10)"  },
};
const LUKS_STYLE: Record<string, { text: string; bg: string }> = {
  sealed: { text: "#22c55e", bg: "rgba(34,197,94,0.10)"  },
  open:   { text: ACCENT,    bg: "rgba(226,240,217,0.1)" },
};

type Filter = "all" | "active" | "completed" | "failed";

export default function ProviderRentalsPage() {
  const [filter, setFilter] = useState<Filter>("all");
  const filtered = filter === "all" ? MOCK_RENTALS : MOCK_RENTALS.filter((r) => r.status === filter);

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="provider" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "flex-start", gap: 16, marginBottom: 24 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 900, letterSpacing: -0.5, color: "#f9fafb" }}>Rentals</p>
              <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>All compute sessions hosted on your hardware</p>
            </div>
            <div style={{ display: "flex", gap: 4, borderRadius: 8, padding: 4, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
              {(["all", "active", "completed", "failed"] as const).map((f) => (
                <button key={f} onClick={() => setFilter(f)} style={{ padding: "6px 12px", borderRadius: 6, fontSize: 12, fontWeight: 600, textTransform: "capitalize", cursor: "pointer", border: "none", background: filter === f ? ACCENT : "transparent", color: filter === f ? "#111111" : "#6b7280" }}>{f}</button>
              ))}
            </div>
          </header>

          <div style={{ borderRadius: 12, overflow: "hidden", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "80px 120px 1fr 140px 80px 90px 90px 70px", gap: 12, padding: "12px 20px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", borderBottom: "1px solid rgba(255,255,255,0.06)", background: "#111111" }}>
              <span>ID</span><span>User</span><span>Session</span><span>Started</span><span>Status</span><span>LUKS</span><span>Earned</span><span></span>
            </div>
            {filtered.map((r) => {
              const ss = STATUS_STYLE[r.status];
              const ls = LUKS_STYLE[r.luks];
              return (
                <div
                  key={r.id}
                  style={{ display: "grid", gridTemplateColumns: "80px 120px 1fr 140px 80px 90px 90px 70px", gap: 12, padding: "14px 20px", alignItems: "center", borderBottom: "1px solid rgba(44,44,46,0.6)" }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = "rgba(255,255,255,0.02)")}
                  onMouseLeave={(e) => (e.currentTarget.style.background = "transparent")}
                >
                  <span style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af" }}>{r.id}</span>
                  <span style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af" }}>{r.user}</span>
                  <Link href={`/sessions/${r.sessionId}`} style={{ fontSize: 11, fontFamily: "monospace", color: ACCENT, textDecoration: "none" }}>{r.sessionId}</Link>
                  <span style={{ fontSize: 11, fontFamily: "monospace", color: "#6b7280" }}>{r.started}</span>
                  <span style={{ fontSize: 11, padding: "2px 6px", borderRadius: 4, fontWeight: 600, color: ss.text, background: ss.bg }}>{r.status}</span>
                  <span style={{ fontSize: 11, padding: "2px 6px", borderRadius: 4, fontWeight: 600, color: ls.text, background: ls.bg }}>{r.luks}</span>
                  <span style={{ fontSize: 13, fontWeight: 700, fontFamily: "monospace", color: "#f3f4f6" }}>{r.earnedUsdc} <span style={{ fontSize: 10, color: "#6b7280" }}>USDC</span></span>
                  <Link href={`/sessions/${r.sessionId}`} style={{ fontSize: 11, color: ACCENT, textDecoration: "none" }}>verify →</Link>
                </div>
              );
            })}
            {filtered.length === 0 && <p style={{ padding: "32px 20px", textAlign: "center", fontSize: 13, color: "#4b5563" }}>No rentals found.</p>}
          </div>
        </div>
      </main>
    </div>
  );
}
