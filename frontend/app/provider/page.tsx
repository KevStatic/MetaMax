"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";

const ACCENT = "#e2f0d9";

type Session = {
  id: string;
  team_id: string;
  prompt: string;
  state: string;
  created_at: string;
};

type Provider = {
  Wallet: string;
  Endpoint: string;
  PricePerHour: string;
  StakedAmount: string;
  JobsCompleted: string;
  Active: boolean;
};

const STATUS_COLORS: Record<string, { text: string; bg: string }> = {
  running:   { text: "#e2f0d9", bg: "rgba(226,240,217,0.1)" },
  completed: { text: "#22c55e", bg: "rgba(34,197,94,0.10)" },
  failed:    { text: "#ef4444", bg: "rgba(239,68,68,0.10)" },
};

export default function ProviderOverviewPage() {
  const { teamId } = useAuth();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [provider, setProvider] = useState<Provider | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (teamId) {
      apiFetch(`/teams/${teamId}/sessions`)
        .then((r) => (r.ok ? r.json() : []))
        .then((data) => setSessions(Array.isArray(data) ? data : []))
        .catch(() => {})
        .finally(() => setLoading(false));
    } else {
      setLoading(false);
    }
    apiFetch("/providers/active")
      .then((r) => (r.ok ? r.json() : []))
      .then((data: Provider[]) => { if (Array.isArray(data) && data.length > 0) setProvider(data[0]); })
      .catch(() => {});
  }, [teamId]);

  const activeCount    = sessions.filter((s) => s.state === "running").length;
  const completedCount = sessions.filter((s) => s.state === "completed").length;

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="provider" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "flex-start", gap: 16, marginBottom: 24 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 300, letterSpacing: -0.5, color: "#f9fafb" }}>Provider Overview</p>
              <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>
                {provider ? `${provider.Endpoint} · ${provider.Active ? "active" : "inactive"}` : "Manage your hardware and earn USDC"}
              </p>
            </div>
            <Link href="/provider/register" style={{ display: "flex", alignItems: "center", gap: 8, borderRadius: 8, height: 40, padding: "0 16px", fontSize: 13, fontWeight: 900, background: ACCENT, color: "#111111", textDecoration: "none" }}>
              + Register Hardware
            </Link>
          </header>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, marginBottom: 32 }}>
            {[
              { label: "Staked Amount",       value: provider ? (parseInt(provider.StakedAmount) / 1e18).toFixed(4) + " MON" : "—", accent: true },
              { label: "Active Sessions",      value: loading ? "…" : String(activeCount) },
              { label: "Completed Sessions",   value: loading ? "…" : String(completedCount) },
              { label: "Jobs Completed",       value: provider ? provider.JobsCompleted : "—" },
            ].map((c) => (
              <div key={c.label} style={{ borderRadius: 12, padding: 16, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
                <p style={{ fontSize: 13, color: "#9ca3af", marginBottom: 8 }}>{c.label}</p>
                <p style={{ fontSize: 22, fontWeight: 700, fontFamily: "monospace", color: c.accent ? ACCENT : "#f9fafb" }}>{c.value}</p>
              </div>
            ))}
          </div>

          <h2 style={{ fontSize: 17, fontWeight: 700, marginBottom: 16, color: "#f9fafb" }}>Recent Sessions</h2>
          <div style={{ borderRadius: 12, overflow: "hidden", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 160px 90px", gap: 16, padding: "12px 20px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", borderBottom: "1px solid rgba(255,255,255,0.06)", background: "#111111" }}>
              <span>Session</span><span>Prompt</span><span>Started</span><span>Status</span>
            </div>

            {loading && <div style={{ padding: "32px 0", textAlign: "center", color: "#4b5563" }}>Loading…</div>}
            {!loading && sessions.length === 0 && <p style={{ padding: "32px 20px", textAlign: "center", fontSize: 13, color: "#4b5563" }}>No sessions yet.</p>}

            {sessions.map((s) => {
              const sc = STATUS_COLORS[s.state] ?? STATUS_COLORS.failed;
              return (
                <div
                  key={s.id}
                  style={{ display: "grid", gridTemplateColumns: "1fr 1fr 160px 90px", gap: 16, padding: "14px 20px", alignItems: "center", borderBottom: "1px solid rgba(44,44,46,0.6)" }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = "rgba(255,255,255,0.02)")}
                  onMouseLeave={(e) => (e.currentTarget.style.background = "transparent")}
                >
                  <Link href={`/sessions/${s.id}`} style={{ fontSize: 12, fontFamily: "monospace", color: ACCENT, textDecoration: "none" }}>{s.id.slice(0, 16)}…</Link>
                  <span style={{ fontSize: 12, fontFamily: "monospace", color: "#9ca3af", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{s.prompt.slice(0, 40)}{s.prompt.length > 40 ? "…" : ""}</span>
                  <span style={{ fontSize: 12, fontFamily: "monospace", color: "#6b7280" }}>{s.created_at ? new Date(s.created_at).toLocaleString() : "—"}</span>
                  <span style={{ fontSize: 11, padding: "2px 8px", borderRadius: 4, fontWeight: 600, color: sc.text, background: sc.bg }}>{s.state}</span>
                </div>
              );
            })}
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, marginTop: 32 }}>
            {[
              { label: "Register Hardware", href: "/provider/register", desc: "Add your machine" },
              { label: "Rentals",           href: "/provider/rentals",  desc: "Sessions on your hardware" },
              { label: "Earnings",          href: "/provider/earnings", desc: "USDC revenue" },
              { label: "Attestations",      href: "/provider/attestations", desc: "Issued proofs" },
            ].map((l) => (
              <Link
                key={l.label}
                href={l.href}
                style={{ borderRadius: 12, padding: 16, display: "flex", flexDirection: "column", gap: 4, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)", textDecoration: "none" }}
                onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.borderColor = ACCENT)}
                onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.borderColor = "rgba(255,255,255,0.07)")}
              >
                <p style={{ fontSize: 13, fontWeight: 700, color: "#f3f4f6" }}>{l.label} →</p>
                <p style={{ fontSize: 12, color: "#6b7280" }}>{l.desc}</p>
              </Link>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
