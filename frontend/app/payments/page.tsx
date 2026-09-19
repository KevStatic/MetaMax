"use client";

import { useEffect, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { useAccount } from "wagmi";
import { monadTestnet, monadTxUrl } from "@/lib/monad";
import { useBalance } from "wagmi";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";

const ACCENT = "#e2f0d9";

type Payment = {
  id: number;
  wallet: string;
  session_id?: string;
  amount_usdc: string;
  tx_hash?: string;
  nonce?: string;
  status: string;
  created_at: string;
};

const STATUS_COLORS: Record<string, { text: string; bg: string }> = {
  confirmed: { text: "#22c55e", bg: "rgba(34,197,94,0.1)"  },
  pending:   { text: "#eab308", bg: "rgba(234,179,8,0.1)"  },
  failed:    { text: "#ef4444", bg: "rgba(239,68,68,0.1)"  },
};

export default function PaymentsPage() {
  const { address } = useAccount();
  const { data: balance } = useBalance({ address, chainId: monadTestnet.id });
  const { address: authAddress } = useAuth();
  const [payments, setPayments] = useState<Payment[]>([]);
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState(false);

  function copyAddress() {
    if (authAddress) { navigator.clipboard.writeText(authAddress); setCopied(true); setTimeout(() => setCopied(false), 1500); }
  }

  useEffect(() => {
    apiFetch("/payments")
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => setPayments(Array.isArray(data) ? data : []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const totalSpent = payments
    .filter((p) => p.status === "confirmed")
    .reduce((sum, p) => sum + parseFloat(p.amount_usdc ?? "0") / 1e6, 0)
    .toFixed(4);

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ marginBottom: 24 }}>
            <p style={{ fontSize: 28, fontWeight: 300, letterSpacing: -0.5, color: "#f9fafb" }}>Payments</p>
            <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>x402 micro-payments on Monad Testnet</p>
          </header>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, marginBottom: 32 }}>
            {[
              { label: "Total Spent",   value: totalSpent + " monad-native" },
              { label: "Transactions",  value: loading ? "…" : String(payments.length) },
              { label: "Protocol",      value: "x402" },
            ].map((c) => (
              <div key={c.label} style={{ borderRadius: 12, padding: 16, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
                <p style={{ fontSize: 13, color: "#9ca3af", marginBottom: 8 }}>{c.label}</p>
                <p style={{ fontSize: 20, fontWeight: 700, fontFamily: "monospace", color: "#f9fafb" }}>{c.value}</p>
              </div>
            ))}
          </div>

          <div style={{ borderRadius: 12, overflow: "hidden", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
            {/* Fund wallet section */}
            <div style={{ padding: 20, borderBottom: "1px solid rgba(255,255,255,0.06)" }}>
              <p style={{ fontSize: 14, fontWeight: 600, color: "#f9fafb", marginBottom: 4 }}>Fund Your Wallet</p>
              <p style={{ fontSize: 12, color: "#6b7280", marginBottom: 16 }}>Send monad-native or MON on Monad Testnet to your connected wallet to pay for compute sessions via x402.</p>
              <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
                <div style={{ flex: 1, minWidth: 0, display: "flex", alignItems: "center", padding: "8px 12px", borderRadius: 8, fontSize: 12, fontFamily: "monospace", background: "#111111", border: "1px solid rgba(255,255,255,0.07)", color: "#9ca3af" }}>
                  <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{authAddress ?? "Connect wallet"}</span>
                </div>
                <button onClick={copyAddress} disabled={!authAddress} style={{ flexShrink: 0, fontSize: 12, padding: "8px 14px", borderRadius: 8, fontWeight: 600, border: "none", cursor: authAddress ? "pointer" : "default", background: ACCENT, color: "#111111", opacity: authAddress ? 1 : 0.4 }}>
                  {copied ? "Copied!" : "Copy Address"}
                </button>
                <a href="https://faucet.monad.xyz" target="_blank" rel="noreferrer" style={{ flexShrink: 0, fontSize: 12, padding: "8px 14px", borderRadius: 8, fontWeight: 600, background: "#1f2937", color: ACCENT, textDecoration: "none", border: "1px solid rgba(226,240,217,0.15)" }}>Get test MON ↗</a>
              </div>
              <div style={{ marginTop: 12, display: "flex", alignItems: "center", gap: 16 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  <span style={{ width: 6, height: 6, borderRadius: "50%", background: "#22c55e", display: "inline-block" }} />
                  <span style={{ fontSize: 12, color: "#6b7280" }}>Balance: <span style={{ fontFamily: "monospace", color: "#f9fafb" }}>{balance ? parseFloat(balance.formatted).toFixed(4) + " " + balance.symbol : "—"}</span></span>
                </div>
                <span style={{ fontSize: 12, color: "#374151" }}>Network: Monad Testnet</span>
              </div>
            </div>

            {/* Table header */}
            <div style={{ display: "grid", gridTemplateColumns: "1fr 110px 80px 160px 1fr", gap: 16, padding: "12px 20px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", borderBottom: "1px solid rgba(255,255,255,0.06)", background: "#111111" }}>
              <span>Session / Nonce</span><span>Amount</span><span>Status</span><span>Time</span><span>Tx Hash</span>
            </div>

            {loading && <div style={{ padding: "40px 0", textAlign: "center", color: "#4b5563" }}>Loading…</div>}
            {!loading && payments.length === 0 && <p style={{ padding: "40px 20px", textAlign: "center", fontSize: 13, color: "#4b5563" }}>No payments yet. Payments appear after x402 sessions are created.</p>}

            {payments.map((p) => {
              const sc = STATUS_COLORS[p.status] ?? STATUS_COLORS.pending;
              const displayAmount = (parseFloat(p.amount_usdc ?? "0") / 1e6).toFixed(6);
              return (
                <div
                  key={p.id}
                  style={{ display: "grid", gridTemplateColumns: "1fr 110px 80px 160px 1fr", gap: 16, padding: "14px 20px", alignItems: "center", borderBottom: "1px solid rgba(44,44,46,0.6)" }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = "rgba(255,255,255,0.02)")}
                  onMouseLeave={(e) => (e.currentTarget.style.background = "transparent")}
                >
                  <span style={{ fontSize: 12, fontFamily: "monospace", color: "#9ca3af", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{p.session_id || p.nonce?.slice(0, 18) || "—"}</span>
                  <span style={{ fontSize: 13, fontWeight: 700, fontFamily: "monospace", color: "#f3f4f6" }}>{displayAmount} <span style={{ color: "#6b7280", fontSize: 10 }}>monad-native</span></span>
                  <span style={{ fontSize: 11, padding: "2px 6px", borderRadius: 4, fontWeight: 600, color: sc.text, background: sc.bg }}>{p.status}</span>
                  <span style={{ fontSize: 11, fontFamily: "monospace", color: "#4b5563" }}>{p.created_at ? new Date(p.created_at).toLocaleString() : "—"}</span>
                  {p.tx_hash ? (
                    <a href={monadTxUrl(p.tx_hash)} target="_blank" rel="noreferrer" style={{ fontSize: 11, fontFamily: "monospace", color: ACCENT, textDecoration: "none", display: "flex", alignItems: "center", gap: 4, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {p.tx_hash.slice(0, 20)}…
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                    </a>
                  ) : (
                    <span style={{ fontSize: 11, fontFamily: "monospace", color: "#4b5563" }}>pending…</span>
                  )}
                </div>
              );
            })}
          </div>
          <p style={{ marginTop: 16, fontSize: 12, color: "#4b5563" }}>Payments use the x402 protocol — native MON escrow payments streamed per second on Monad Testnet.</p>
        </div>
      </main>
    </div>
  );
}
