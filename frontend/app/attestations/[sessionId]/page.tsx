"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { formatTime } from "@/lib/utils";

const BG = "#111111";
const CARD = "#1a1a1a";
const BORDER = "rgba(255,255,255,0.06)";
const ACCENT = "#e2f0d9";

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

type AuditEntry = {
  index: number;
  tool: string;
  hash: string;
  timestamp: string;
  error?: string;
  proof?: string[];
};

export default function AttestationDetailPage() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const [attestation, setAttestation] = useState<Attestation | null>(null);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!sessionId) return;
    Promise.all([
      apiFetch(`/attestations/${sessionId}`).then((r) => (r.ok ? r.json() : null)),
      apiFetch(`/sessions/${sessionId}/audit`).then((r) => (r.ok ? r.json() : null)),
    ])
      .then(([att, aud]) => {
        setAttestation(att);
        if (aud?.actions) setAudit(aud.actions);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [sessionId]);

  const field = (label: string, value: string, mono = false, href?: string) => (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <span style={{ fontSize: 11, color: "#6b7280", textTransform: "uppercase", letterSpacing: "0.05em" }}>{label}</span>
      {href ? (
        <a
          href={href}
          target="_blank"
          rel="noreferrer"
          style={{ fontSize: 13, fontFamily: mono ? "monospace" : "inherit", color: ACCENT, wordBreak: "break-all", textDecoration: "none" }}
        >
          {value}
        </a>
      ) : (
        <span style={{ fontSize: 13, fontFamily: mono ? "monospace" : "inherit", color: "#e5e7eb", wordBreak: "break-all" }}>{value || "—"}</span>
      )}
    </div>
  );

  return (
    <div style={{ display: "flex", height: "100vh", background: BG, fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32, maxWidth: 900 }}>
          <header style={{ marginBottom: 28 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 6 }}>
              <Link href="/attestations" style={{ fontSize: 12, color: "#6b7280", textDecoration: "none" }}>← Attestations</Link>
            </div>
            <p style={{ fontSize: 26, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Attestation Detail</p>
            <p style={{ fontSize: 13, color: "#6b7280", marginTop: 4, fontFamily: "monospace" }}>
              Session: {sessionId}
            </p>
          </header>

          {loading && (
            <div style={{ display: "flex", justifyContent: "center", padding: 48 }}>
              <span className="animate-spin" style={{ display: "inline-block", width: 32, height: 32, borderRadius: "50%", border: "2px solid rgba(255,255,255,0.1)", borderTopColor: "#fff" }} />
            </div>
          )}

          {error && <p style={{ color: "#ef4444", fontSize: 14 }}>{error}</p>}

          {!loading && attestation && (
            <>
              {/* EAS status banner */}
              <div style={{
                display: "flex", alignItems: "center", gap: 12, padding: "12px 20px", borderRadius: 10, marginBottom: 24,
                background: attestation.attestation_uid ? "rgba(34,197,94,0.08)" : "rgba(234,179,8,0.08)",
                border: `1px solid ${attestation.attestation_uid ? "rgba(34,197,94,0.25)" : "rgba(234,179,8,0.25)"}`,
              }}>
                <div style={{ width: 10, height: 10, borderRadius: "50%", background: attestation.attestation_uid ? "#22c55e" : "#eab308", flexShrink: 0 }} />
                <span style={{ fontSize: 13, fontWeight: 700, color: attestation.attestation_uid ? "#22c55e" : "#eab308" }}>
                  {attestation.attestation_uid ? "On-chain attestation confirmed" : "Attestation pending — UID not yet captured"}
                </span>
                {attestation.eas_scan_url && (
                  <a
                    href={attestation.eas_scan_url}
                    target="_blank"
                    rel="noreferrer"
                    style={{ marginLeft: "auto", fontSize: 12, padding: "4px 14px", borderRadius: 6, background: "rgba(34,197,94,0.15)", color: "#22c55e", textDecoration: "none", border: "1px solid rgba(34,197,94,0.3)", fontWeight: 700 }}
                  >
                    View on EAS Scan ↗
                  </a>
                )}
              </div>

              {/* Attestation fields */}
              <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, padding: 24, marginBottom: 24, display: "flex", flexDirection: "column", gap: 20 }}>
                <p style={{ fontSize: 14, fontWeight: 700, color: "#f9fafb", margin: 0 }}>Attestation Record</p>
                {field("Attestation UID", attestation.attestation_uid ?? "", true,
                  attestation.attestation_uid ? `https://testnet.monadscan.com/attestation/view/${attestation.attestation_uid}` : undefined)}
                {field("Transaction Hash", attestation.tx_hash, true,
                  attestation.tx_hash ? `https://testnet.monadscan.com/tx/${attestation.tx_hash}` : undefined)}
                {field("Merkle Root", attestation.merkle_root, true)}
                {field("Schema UID", attestation.schema_uid ?? "", true,
                  attestation.schema_uid ? `https://testnet.monadscan.com/schema/view/${attestation.schema_uid}` : undefined)}
                {field("Attested At", formatTime(attestation.created_at))}
              </div>

              {/* Action log */}
              {audit.length > 0 && (
                <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, overflow: "hidden" }}>
                  <div style={{ padding: "16px 24px", borderBottom: `1px solid ${BORDER}` }}>
                    <p style={{ fontSize: 14, fontWeight: 700, color: "#f9fafb", margin: 0 }}>
                      Action Log — {audit.length} step(s) · Merkle root verified
                    </p>
                  </div>
                  {audit.map((action, i) => (
                    <div
                      key={i}
                      style={{
                        padding: "14px 24px",
                        borderBottom: i < audit.length - 1 ? `1px solid ${BORDER}` : "none",
                        display: "flex", flexDirection: "column", gap: 6,
                      }}
                    >
                      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                        <span style={{ fontSize: 11, color: "#4b5563", minWidth: 24 }}>#{action.index}</span>
                        <span style={{ fontSize: 13, fontWeight: 700, color: action.error ? "#ef4444" : ACCENT }}>{action.tool}</span>
                        <span style={{ fontSize: 11, color: "#6b7280", marginLeft: "auto" }}>{formatTime(action.timestamp)}</span>
                      </div>
                      <div style={{ fontSize: 11, fontFamily: "monospace", color: "#4b5563", paddingLeft: 34, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                        hash: {action.hash}
                      </div>
                      {action.proof && action.proof.length > 0 && (
                        <div style={{ paddingLeft: 34 }}>
                          <span style={{ fontSize: 10, color: "#374151" }}>Merkle proof: </span>
                          {action.proof.map((p, pi) => (
                            <span key={pi} style={{ fontSize: 10, fontFamily: "monospace", color: "#374151", marginRight: 6 }}>
                              {p.slice(0, 12)}…
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </>
          )}

          {!loading && !attestation && !error && (
            <div style={{ padding: 48, textAlign: "center", color: "#4b5563" }}>
              No attestation found for this session. It may still be processing.
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
