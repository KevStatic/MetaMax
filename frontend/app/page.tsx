"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { WalletButton } from "@/components/WalletButton";
import { useBalance } from "wagmi";
import { monadTestnet } from "@/lib/monad";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";

type Container = {
  ID: string;
  Name: string;
  Image?: string;
  Status: string;
  Ports: Record<string, string> | null;
  Created?: string;
};

type SessionSummary = {
  id: string;
  prompt: string;
  state: string;
  created_at: string;
  updated_at: string;
};

function useClock() {
  const [time, setTime] = useState("");
  useEffect(() => {
    const fmt = () =>
      new Date().toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hour12: true });
    setTime(fmt());
    const t = setInterval(() => setTime(fmt()), 1000);
    return () => clearInterval(t);
  }, []);
  return time;
}

export default function Home() {
  const { address, isAuthenticated, teamId, teamName, isNewAccount, hydrated } = useAuth();
  const router = useRouter();
  const { data: balance } = useBalance({
    address: address as `0x${string}` | undefined,
    chainId: monadTestnet.id,
  });
  const clock = useClock();
  const [containers, setContainers] = useState<Container[]>([]);
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [loadingContainers, setLoadingContainers] = useState(false);

  useEffect(() => {
    if (hydrated && !isAuthenticated) {
      router.replace("/signin");
    }
  }, [hydrated, isAuthenticated, router]);

  useEffect(() => {
    if (!teamId) return;
    setLoadingContainers(true);
    apiFetch(`/teams/${teamId}/workspaces`)
      .then((r) => (r.ok ? r.json() : []))
      .then((c) => { if (Array.isArray(c)) setContainers(c); })
      .catch(() => {})
      .finally(() => setLoadingContainers(false));
    apiFetch(`/teams/${teamId}/sessions`)
      .then((r) => (r.ok ? r.json() : []))
      .then((s) => { if (Array.isArray(s)) setSessions(s); })
      .catch(() => {});
  }, [teamId]);

  const running = containers.filter((c) => c.Status?.toLowerCase().includes("running")).length;
  const completed = sessions.filter((s) => s.state === "completed").length;
  const successRate = sessions.length > 0 ? Math.round((completed / sessions.length) * 100) : 0;

  if (!hydrated || !isAuthenticated) {
    return (
      <div className="min-h-screen flex items-center justify-center" style={{ background: "#111111" }}>
        <span className="inline-block h-8 w-8 rounded-full border-2 border-white/20 border-t-white animate-spin" />
      </div>
    );
  }

  return (
    <div
      style={{
        display: "flex",
        height: "100vh",
        background: "#111111",
        fontFamily: "Inter, var(--font-inter), sans-serif",
        color: "#f9fafb",
      }}
    >
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: "32px", display: "flex", flexDirection: "column", gap: 24 }}>

          {/* ── Header ── */}
          <header style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <h1 style={{ fontSize: 30, fontWeight: 300, letterSpacing: "-0.02em" }}>Overview</h1>
            <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
              <div style={{ fontSize: 20, fontWeight: 300 }}>
                {clock}{" "}
                <span style={{ fontSize: 11, color: "#6b7280", marginLeft: 4, textTransform: "uppercase", letterSpacing: "0.06em" }}>
                  Time
                </span>
              </div>
              <WalletButton />
              <Link
                href="/launch"
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 8,
                  borderRadius: 999,
                  padding: "8px 20px",
                  fontSize: 14,
                  fontWeight: 500,
                  background: "#e2f0d9",
                  color: "#111111",
                  textDecoration: "none",
                }}
              >
                Deploy my AI workload
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                  <line x1="5" y1="12" x2="19" y2="12"/>
                  <polyline points="12 5 19 12 12 19"/>
                </svg>
              </Link>
            </div>
          </header>

          {/* ── Onboarding banner ── */}
          {isNewAccount && (
            <Link
              href="/onboarding"
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                borderRadius: 16,
                padding: "16px 20px",
                background: "rgba(226,240,217,0.06)",
                border: "1px solid rgba(226,240,217,0.18)",
                textDecoration: "none",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <div
                  style={{
                    width: 28,
                    height: 28,
                    borderRadius: "50%",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    background: "rgba(226,240,217,0.15)",
                  }}
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#e2f0d9" strokeWidth="2.5">
                    <circle cx="12" cy="12" r="10"/>
                    <line x1="12" y1="8" x2="12" y2="12"/>
                    <line x1="12" y1="16" x2="12.01" y2="16"/>
                  </svg>
                </div>
                <div>
                  <p style={{ fontSize: 14, fontWeight: 500, color: "#fff" }}>
                    {teamName && !teamName.startsWith("account-") ? `Welcome, ${teamName}` : "Set up your account"}
                  </p>
                  <p style={{ fontSize: 12, color: "#6b7280" }}>Add a display name to personalize your workspace.</p>
                </div>
              </div>
              <span style={{ fontSize: 12, fontWeight: 500, color: "#e2f0d9" }}>Set up →</span>
            </Link>
          )}

          {/* ── Stat strip ── */}
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16 }}>
            {/* Containers */}
            <div
              style={{
                borderRadius: 16,
                padding: 24,
                border: "1px solid rgba(255,255,255,0.05)",
                background: "#1a1a1a",
                display: "flex",
                flexDirection: "column",
                gap: 16,
              }}
            >
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <span style={{ fontSize: 10, color: "#6b7280", textTransform: "uppercase", letterSpacing: "0.08em" }}>Containers</span>
                <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  <div style={{ width: 6, height: 6, borderRadius: "50%", background: running > 0 ? "#4ade80" : "#374151" }} />
                  <span style={{ fontSize: 10, color: "#6b7280" }}>{running > 0 ? "Live" : "Idle"}</span>
                </div>
              </div>
              <div>
                <div style={{ fontSize: 40, fontWeight: 300, letterSpacing: "-0.02em" }}>
                  {running}
                  <span style={{ fontSize: 24, color: "#4b5563" }}>/{containers.length}</span>
                </div>
                <div style={{ fontSize: 10, color: "#4b5563", marginTop: 4 }}>running / total</div>
              </div>
            </div>

            {/* Sessions */}
            <div
              style={{
                borderRadius: 16,
                padding: 24,
                border: "1px solid rgba(255,255,255,0.05)",
                background: "#1a1a1a",
                display: "flex",
                flexDirection: "column",
                gap: 16,
              }}
            >
              <span style={{ fontSize: 10, color: "#6b7280", textTransform: "uppercase", letterSpacing: "0.08em" }}>Sessions</span>
              <div>
                <div style={{ fontSize: 40, fontWeight: 300, letterSpacing: "-0.02em" }}>{sessions.length}</div>
                <div style={{ fontSize: 10, color: "#4b5563", marginTop: 4 }}>total deployments</div>
              </div>
            </div>

            {/* Success Rate */}
            <div
              style={{
                borderRadius: 16,
                padding: 24,
                border: "1px solid rgba(255,255,255,0.05)",
                background: "#1a1a1a",
                display: "flex",
                flexDirection: "column",
                gap: 16,
              }}
            >
              <span style={{ fontSize: 10, color: "#6b7280", textTransform: "uppercase", letterSpacing: "0.08em" }}>Success Rate</span>
              <div>
                <div style={{ fontSize: 40, fontWeight: 300, letterSpacing: "-0.02em" }}>{successRate}%</div>
                <div style={{ fontSize: 10, color: "#4b5563", marginTop: 4 }}>{completed} completed</div>
              </div>
            </div>

            {/* Balance */}
            <div
              style={{
                borderRadius: 16,
                padding: 24,
                border: "1px solid rgba(255,255,255,0.05)",
                background: "#1a1a1a",
                display: "flex",
                flexDirection: "column",
                gap: 16,
              }}
            >
              <span style={{ fontSize: 10, color: "#6b7280", textTransform: "uppercase", letterSpacing: "0.08em" }}>Balance</span>
              <div>
                <div style={{ fontSize: 40, fontWeight: 300, letterSpacing: "-0.02em" }}>
                  {balance ? parseFloat(balance.formatted).toFixed(4) : "—"}
                </div>
                <div style={{ fontSize: 10, color: "#4b5563", marginTop: 4 }}>MON (Monad Testnet)</div>
              </div>
            </div>
          </div>

          {/* ── Recent Sessions ── */}
          <div
            style={{
              borderRadius: 16,
              border: "1px solid rgba(255,255,255,0.05)",
              background: "#1a1a1a",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                padding: "16px 24px",
                borderBottom: "1px solid rgba(255,255,255,0.05)",
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
              }}
            >
              <span style={{ fontSize: 14, fontWeight: 600, color: "#f9fafb" }}>Recent Sessions</span>
              <Link href="/sessions" style={{ fontSize: 12, color: "#e2f0d9", textDecoration: "none" }}>
                View all →
              </Link>
            </div>
            {sessions.length === 0 ? (
              <div style={{ padding: "40px 24px", textAlign: "center", color: "#4b5563", fontSize: 14 }}>
                {loadingContainers ? "Loading…" : "No sessions yet. Deploy your first app →"}
              </div>
            ) : (
              sessions.slice(0, 5).map((s) => {
                const stateColor =
                  s.state === "completed" ? "#22c55e" :
                  s.state === "failed" ? "#ef4444" : "#eab308";
                return (
                  <Link
                    key={s.id}
                    href={`/sessions/${s.id}`}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      padding: "14px 24px",
                      borderBottom: "1px solid rgba(255,255,255,0.03)",
                      textDecoration: "none",
                      color: "inherit",
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                      <div style={{ width: 8, height: 8, borderRadius: "50%", background: stateColor, flexShrink: 0 }} />
                      <div>
                        <p style={{ fontSize: 13, color: "#e5e7eb", fontWeight: 500 }}>{s.prompt.slice(0, 60)}{s.prompt.length > 60 ? "…" : ""}</p>
                        <p style={{ fontSize: 11, color: "#4b5563", fontFamily: "var(--font-space-mono), monospace" }}>
                          {s.id.slice(0, 20)}…
                        </p>
                      </div>
                    </div>
                    <div style={{ textAlign: "right", flexShrink: 0 }}>
                      <span style={{ fontSize: 11, fontWeight: 600, color: stateColor, textTransform: "capitalize" }}>{s.state}</span>
                    </div>
                  </Link>
                );
              })
            )}
          </div>

        </div>
      </main>
    </div>
  );
}
