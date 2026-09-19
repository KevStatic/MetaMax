"use client";

import { Sidebar } from "@/components/Sidebar";

const ACCENT = "#e2f0d9";

type EarningRow = { sessionId: string; user: string; duration: string; amount: string; date: string };

const EARNING_ROWS: EarningRow[] = [
  { sessionId: "sess-demo-001", user: "0x1a2b…3c4d", duration: "32m", amount: "0.0042", date: "Apr 30" },
  { sessionId: "sess-demo-002", user: "0x5e6f…7a8b", duration: "9m",  amount: "0.0011", date: "Apr 30" },
  { sessionId: "sess-demo-003", user: "0x9c0d…1e2f", duration: "30m", amount: "0.0031", date: "Apr 29" },
  { sessionId: "sess-demo-004", user: "0x7a8b…9c0d", duration: "18m", amount: "0.0024", date: "Apr 28" },
  { sessionId: "sess-demo-005", user: "0x2b3c…4d5e", duration: "45m", amount: "0.0058", date: "Apr 27" },
];

const WEEKLY_DATA = [
  { label: "Apr 24", amount: 0.0035 },
  { label: "Apr 25", amount: 0.0052 },
  { label: "Apr 26", amount: 0.0021 },
  { label: "Apr 27", amount: 0.0058 },
  { label: "Apr 28", amount: 0.0031 },
  { label: "Apr 29", amount: 0.0031 },
  { label: "Apr 30", amount: 0.0053 },
];

export default function ProviderEarningsPage() {
  const totalEarned = EARNING_ROWS.reduce((s, r) => s + parseFloat(r.amount), 0).toFixed(4);
  const maxBar = Math.max(...WEEKLY_DATA.map((d) => d.amount));
  const weekTotal = WEEKLY_DATA.reduce((s, d) => s + d.amount, 0).toFixed(4);

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="provider" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ marginBottom: 24 }}>
            <p style={{ fontSize: 28, fontWeight: 900, letterSpacing: -0.5, color: "#f9fafb" }}>Earnings</p>
            <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>USDC earned from compute sessions on your hardware</p>
          </header>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, marginBottom: 32 }}>
            {[
              { label: "Total Earned",    value: totalEarned + " USDC", accent: true },
              { label: "Sessions Hosted", value: String(EARNING_ROWS.length) },
              { label: "This Week",       value: weekTotal + " USDC" },
              { label: "Avg / Session",   value: (parseFloat(totalEarned) / EARNING_ROWS.length).toFixed(4) + " USDC" },
            ].map((c) => (
              <div key={c.label} style={{ borderRadius: 12, padding: 16, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
                <p style={{ fontSize: 13, color: "#9ca3af", marginBottom: 8 }}>{c.label}</p>
                <p style={{ fontSize: 20, fontWeight: 700, fontFamily: "monospace", color: c.accent ? ACCENT : "#f9fafb" }}>{c.value}</p>
              </div>
            ))}
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, marginBottom: 32 }}>
            {/* Bar chart */}
            <div style={{ borderRadius: 12, padding: 20, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
              <p style={{ fontSize: 13, fontWeight: 700, color: "#f9fafb", marginBottom: 20 }}>Last 7 Days</p>
              <div style={{ display: "flex", alignItems: "flex-end", gap: 10, height: 120 }}>
                {WEEKLY_DATA.map((d) => {
                  const pct = (d.amount / maxBar) * 100;
                  return (
                    <div key={d.label} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 6 }}>
                      <div
                        title={String(d.amount)}
                        style={{ width: "100%", borderRadius: "4px 4px 0 0", background: ACCENT, opacity: 0.8, minHeight: 4, height: `${pct}%` }}
                      />
                      <span style={{ fontSize: 9, fontFamily: "monospace", color: "#4b5563" }}>{d.label.slice(-5)}</span>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Withdraw card */}
            <div style={{ borderRadius: 12, padding: 20, display: "flex", flexDirection: "column", gap: 16, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
              <p style={{ fontSize: 13, fontWeight: 700, color: "#f9fafb" }}>Withdraw Earnings</p>
              <div style={{ borderRadius: 8, padding: 16, background: "#0a0a0a", border: "1px solid rgba(255,255,255,0.06)" }}>
                <p style={{ fontSize: 11, color: "#6b7280", marginBottom: 4 }}>Available balance</p>
                <p style={{ fontSize: 24, fontWeight: 900, fontFamily: "monospace", color: ACCENT }}>{totalEarned} <span style={{ fontSize: 14, color: "#6b7280" }}>USDC</span></p>
              </div>
              <button disabled style={{ height: 40, borderRadius: 8, fontSize: 13, fontWeight: 900, border: "none", cursor: "not-allowed", background: ACCENT, color: "#111111", opacity: 0.3 }}>
                Withdraw (coming soon)
              </button>
              <p style={{ fontSize: 11, color: "#4b5563" }}>Withdrawals enabled when protocol launches on mainnet.</p>
            </div>
          </div>

          <h2 style={{ fontSize: 17, fontWeight: 700, marginBottom: 16, color: "#f9fafb" }}>Session Breakdown</h2>
          <div style={{ borderRadius: 12, overflow: "hidden", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 130px 80px 100px 120px", gap: 16, padding: "12px 20px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", borderBottom: "1px solid rgba(255,255,255,0.06)", background: "#111111" }}>
              <span>Session</span><span>User</span><span>Duration</span><span>Date</span><span>Earned</span>
            </div>
            {EARNING_ROWS.map((r) => (
              <div
                key={r.sessionId}
                style={{ display: "grid", gridTemplateColumns: "1fr 130px 80px 100px 120px", gap: 16, padding: "14px 20px", alignItems: "center", borderBottom: "1px solid rgba(44,44,46,0.6)" }}
                onMouseEnter={(e) => (e.currentTarget.style.background = "rgba(255,255,255,0.02)")}
                onMouseLeave={(e) => (e.currentTarget.style.background = "transparent")}
              >
                <span style={{ fontSize: 11, fontFamily: "monospace", color: ACCENT }}>{r.sessionId}</span>
                <span style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af" }}>{r.user}</span>
                <span style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af" }}>{r.duration}</span>
                <span style={{ fontSize: 11, fontFamily: "monospace", color: "#6b7280" }}>{r.date}</span>
                <span style={{ fontSize: 13, fontWeight: 700, fontFamily: "monospace", color: "#f3f4f6" }}>{r.amount} <span style={{ fontSize: 10, color: "#6b7280" }}>USDC</span></span>
              </div>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
