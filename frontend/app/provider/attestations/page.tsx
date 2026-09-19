"use client";

import { useEffect, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";

const ACCENT = "#e2f0d9";

type ProviderAttestation = {
  id?: number;
  uid?: string;
  session_id: string;
  tx_hash: string;
  merkle_root: string;
  created_at: string;
};

export default function ProviderAttestationsPage() {
  const { teamId } = useAuth();
  const [attestations, setAttestations] = useState<ProviderAttestation[]>([]);
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState<number | null>(null);

  useEffect(() => {
    if (!teamId) { setLoading(false); return; }
    apiFetch(`/teams/${teamId}/attestations`)
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => setAttestations(Array.isArray(data) ? data : []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [teamId]);

  function copyUID(uid: string, i: number) {
    navigator.clipboard.writeText(uid);
    setCopied(i);
    setTimeout(() => setCopied(null), 1500);
  }

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="provider" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "flex-start", gap: 16, marginBottom: 24 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 900, letterSpacing: -0.5, color: "#f9fafb" }}>Issued Attestations</p>
              <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>On-chain proofs you issued as compute provider via EAS</p>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 12px", borderRadius: 8, fontSize: 12, fontFamily: "monospace", background: "rgba(226,240,217,0.06)", border: "1px solid rgba(226,240,217,0.2)", color: ACCENT }}>
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
              EAS · Monad Testnet
            </div>
          </header>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, marginBottom: 32 }}>
            {[
              { label: "Attestations Issued", value: loading ? "…" : String(attestations.length) },
              { label: "Sessions Covered",    value: loading ? "…" : String(new Set(attestations.map((a) => a.session_id)).size) },
              { label: "Schema",              value: "EAS v1" },
            ].map((c) => (
              <div key={c.label} style={{ borderRadius: 12, padding: 16, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
                <p style={{ fontSize: 13, color: "#9ca3af", marginBottom: 8 }}>{c.label}</p>
                <p style={{ fontSize: 20, fontWeight: 700, fontFamily: "monospace", color: "#f9fafb" }}>{c.value}</p>
              </div>
            ))}
          </div>

          <div style={{ borderRadius: 12, overflow: "hidden", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 160px 1fr", gap: 16, padding: "12px 20px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", borderBottom: "1px solid rgba(255,255,255,0.06)", background: "#111111" }}>
              <span>Session</span><span>Merkle Root</span><span>Timestamp</span><span>Explorer</span>
            </div>

            {loading && <div style={{ padding: "32px 0", textAlign: "center", color: "#4b5563" }}>Loading…</div>}
            {!loading && attestations.length === 0 && <p style={{ padding: "32px 20px", textAlign: "center", fontSize: 13, color: "#4b5563" }}>No attestations issued yet.</p>}

            {attestations.map((a, i) => (
              <div
                key={i}
                style={{ display: "grid", gridTemplateColumns: "1fr 1fr 160px 1fr", gap: 16, padding: "14px 20px", alignItems: "center", borderBottom: "1px solid rgba(44,44,46,0.6)" }}
                onMouseEnter={(e) => (e.currentTarget.style.background = "rgba(255,255,255,0.02)")}
                onMouseLeave={(e) => (e.currentTarget.style.background = "transparent")}
              >
                <span style={{ fontSize: 11, fontFamily: "monospace", color: ACCENT, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{a.session_id.slice(0, 20)}…</span>
                <div style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                  <span style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{a.merkle_root.slice(0, 20)}…</span>
                  <button onClick={() => copyUID(a.merkle_root, i)} style={{ fontSize: 10, padding: "2px 6px", borderRadius: 4, background: "#2a2a2d", color: "#6b7280", border: "none", cursor: "pointer", flexShrink: 0 }}>
                    {copied === i ? "✓" : "copy"}
                  </button>
                </div>
                <span style={{ fontSize: 11, fontFamily: "monospace", color: "#6b7280" }}>{a.created_at ? new Date(a.created_at).toLocaleString() : "—"}</span>
                <a href={`https://testnet.monadscan.com/tx/${a.tx_hash}`} target="_blank" rel="noreferrer" style={{ fontSize: 11, fontFamily: "monospace", color: ACCENT, textDecoration: "none", display: "flex", alignItems: "center", gap: 4 }}>
                  {a.tx_hash.slice(0, 16)}…
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                </a>
              </div>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
