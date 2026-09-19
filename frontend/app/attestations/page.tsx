"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import { formatTime } from "@/lib/utils";

type Attestation = {
  id: number;
  session_id: string;
  tx_hash: string;
  attestation_uid?: string;
  merkle_root: string;
  schema_uid?: string;
  eas_scan_url?: string;
  created_at: string;
};

export default function AttestationsPage() {
  const { teamId } = useAuth();
  const [attestations, setAttestations] = useState<Attestation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!teamId) { setLoading(false); return; }
    apiFetch(`/teams/${teamId}/attestations`)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error("Failed to load"))))
      .then((data) => setAttestations(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [teamId]);

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ marginBottom: 32 }}>
            <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Attestations</p>
            <p style={{ fontSize: 13, color: "#6b7280", marginTop: 4 }}>On-chain EAS attestations for your deployment sessions</p>
          </header>

          <div style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.05)", borderRadius: 12, overflow: "hidden" }}>
            {loading && <div style={{ padding: 40, textAlign: "center", color: "#6b7280" }}>Loading…</div>}
            {error && <div style={{ padding: 40, textAlign: "center", color: "#ef4444" }}>{error}</div>}
            {!loading && !error && attestations.length === 0 && (
              <div style={{ padding: 40, textAlign: "center", color: "#4b5563", fontSize: 14 }}>
                No attestations yet. Complete a deployment session to generate one.
              </div>
            )}
            {!loading && attestations.map((a, i) => (
              <div
                key={a.id}
                style={{
                  padding: "16px 24px",
                  borderBottom: i < attestations.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 16,
                }}
              >
                <div style={{ display: "flex", alignItems: "center", gap: 16, minWidth: 0 }}>
                  <div style={{ width: 8, height: 8, borderRadius: "50%", background: "#22c55e", flexShrink: 0 }} />
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontSize: 12, fontFamily: "monospace", color: "#9ca3af", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      Session: {a.session_id.slice(0, 28)}…
                    </div>
                    <div style={{ fontSize: 11, fontFamily: "monospace", color: "#4b5563", marginTop: 2, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      Root: {a.merkle_root.slice(0, 40)}…
                    </div>
                  </div>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 16, flexShrink: 0 }}>
                  <span style={{ fontSize: 11, color: "#6b7280" }}>{formatTime(a.created_at)}</span>
                  {a.eas_scan_url ? (
                    <a
                      href={a.eas_scan_url}
                      target="_blank"
                      rel="noreferrer"
                      style={{ fontSize: 11, padding: "4px 12px", borderRadius: 4, background: "rgba(34,197,94,0.1)", color: "#22c55e", textDecoration: "none", border: "1px solid rgba(34,197,94,0.2)", flexShrink: 0 }}
                    >
                      View on EAS →
                    </a>
                  ) : (
                    <a
                      href={`https://testnet.monadscan.com/`}
                      target="_blank"
                      rel="noreferrer"
                      style={{ fontSize: 11, padding: "4px 12px", borderRadius: 4, background: "#1f2937", color: "#6b7280", textDecoration: "none", flexShrink: 0 }}
                    >
                      EAS Scan →
                    </a>
                  )}
                  <Link
                    href={`/attestations/${a.session_id}`}
                    style={{ fontSize: 11, padding: "4px 12px", borderRadius: 4, background: "#1f2937", color: "#9ca3af", textDecoration: "none", flexShrink: 0 }}
                  >
                    Detail →
                  </Link>
                </div>
              </div>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
