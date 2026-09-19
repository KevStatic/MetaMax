"use client";

import { useEffect, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";

const ACCENT = "#e2f0d9";

const TOOL_COLORS: Record<string, { text: string; bg: string }> = {
  bash:             { text: "#f59e0b", bg: "rgba(245,158,11,0.1)" },
  write_file:       { text: "#60a5fa", bg: "rgba(96,165,250,0.1)" },
  read_file:        { text: "#a78bfa", bg: "rgba(167,139,250,0.1)" },
  clone_repo:       { text: "#34d399", bg: "rgba(52,211,153,0.1)" },
  install_packages: { text: "#fb923c", bg: "rgba(251,146,60,0.1)" },
  run_command:      { text: "#f59e0b", bg: "rgba(245,158,11,0.1)" },
  start_process:    { text: "#22c55e", bg: "rgba(34,197,94,0.1)"  },
  create_container: { text: "#22c55e", bg: "rgba(34,197,94,0.1)"  },
  setup_database:   { text: "#eab308", bg: "rgba(234,179,8,0.1)"  },
  health_check:     { text: "#4ade80", bg: "rgba(74,222,128,0.1)" },
};

function getTC(tool: string) {
  return TOOL_COLORS[tool] ?? { text: "#6b7280", bg: "rgba(107,114,128,0.1)" };
}

type SessionSummary = { id: string; prompt: string; state: string };
type Action = { tool: string; hash: string; proof?: string[]; input?: unknown; result?: unknown; error?: string };
type ActionLog = { session_id: string; actions: Action[]; merkle_root?: string };

export default function AuditPage() {
  const { teamId } = useAuth();
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [sessionFilter, setSessionFilter] = useState("all");
  const [log, setLog] = useState<ActionLog | null>(null);
  const [loadingLog, setLoadingLog] = useState(false);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  useEffect(() => {
    if (!teamId) return;
    apiFetch(`/teams/${teamId}/sessions`)
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => {
        if (Array.isArray(data) && data.length > 0) {
          setSessions(data);
          setSessionFilter(data[0].id);
        }
      })
      .catch(() => {});
  }, [teamId]);

  useEffect(() => {
    if (!sessionFilter || sessionFilter === "all") { setLog(null); return; }
    setLoadingLog(true);
    apiFetch(`/sessions/${sessionFilter}/audit`)
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => setLog(data))
      .catch(() => setLog(null))
      .finally(() => setLoadingLog(false));
  }, [sessionFilter]);

  function toggle(i: number) {
    setExpanded((prev) => { const next = new Set(prev); if (next.has(i)) next.delete(i); else next.add(i); return next; });
  }

  const actions: Action[] = log?.actions ?? [];

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "flex-start", gap: 16, marginBottom: 24 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 300, letterSpacing: -0.5, color: "#f9fafb" }}>Audit Trail</p>
              <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>Immutable action log — every agent action hashed and stored on-chain</p>
            </div>
            <select
              value={sessionFilter}
              onChange={(e) => setSessionFilter(e.target.value)}
              style={{ borderRadius: 8, padding: "8px 12px", fontSize: 13, outline: "none", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)", color: "#e5e7eb", cursor: "pointer" }}
            >
              <option value="all">All Sessions</option>
              {sessions.map((s) => (
                <option key={s.id} value={s.id}>{s.id.slice(0, 16)}…</option>
              ))}
            </select>
          </header>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, marginBottom: 32 }}>
            {[
              { label: "Total Actions", value: loadingLog ? "…" : String(actions.length) },
              { label: "Merkle Root",   value: log?.merkle_root ? log.merkle_root.slice(0, 18) + "…" : "—" },
              { label: "Verified",      value: actions.length > 0 ? "On-chain" : "—" },
            ].map((c) => (
              <div key={c.label} style={{ borderRadius: 12, padding: 16, background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
                <p style={{ fontSize: 13, color: "#9ca3af", marginBottom: 8 }}>{c.label}</p>
                <p style={{ fontSize: 14, fontWeight: 700, fontFamily: "monospace", color: c.label === "Verified" && c.value === "On-chain" ? "#22c55e" : "#f9fafb", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{c.value}</p>
              </div>
            ))}
          </div>

          {loadingLog && <div style={{ padding: "40px 0", textAlign: "center", color: "#4b5563" }}>Loading…</div>}
          {!loadingLog && actions.length === 0 && (
            <p style={{ padding: "40px 0", textAlign: "center", fontSize: 13, color: "#4b5563" }}>
              {sessions.length === 0 ? "No sessions yet." : "No action log for this session."}
            </p>
          )}

          {/* Timeline */}
          <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "0 16px" }}>
            {actions.map((action, i) => {
              const tc = getTC(action.tool);
              const isExpanded = expanded.has(i);
              const isLast = i === actions.length - 1;
              const hasError = !!action.error;
              return (
                <>
                  <div key={`dot-${i}`} style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
                    <div style={{ width: 28, height: 28, borderRadius: "50%", flexShrink: 0, display: "flex", alignItems: "center", justifyContent: "center", background: hasError ? "rgba(239,68,68,0.12)" : tc.bg, border: `1px solid ${hasError ? "#ef4444" : tc.text}` }}>
                      {hasError
                        ? <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#ef4444" strokeWidth="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                        : <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={tc.text} strokeWidth="2"><polyline points="20 6 9 17 4 12"/></svg>
                      }
                    </div>
                    {!isLast && <div style={{ width: 1, flex: 1, marginTop: 4, background: "rgba(255,255,255,0.06)", minHeight: 24 }} />}
                  </div>

                  <div
                    key={`card-${i}`}
                    onClick={() => toggle(i)}
                    style={{ marginBottom: 12, borderRadius: 12, overflow: "hidden", cursor: "pointer", border: `1px solid ${hasError ? "rgba(239,68,68,0.3)" : "rgba(255,255,255,0.07)"}`, background: "#1a1a1a" }}
                  >
                    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px" }}>
                      <div style={{ display: "flex", alignItems: "center", gap: 12, minWidth: 0 }}>
                        <span style={{ fontSize: 11, padding: "2px 8px", borderRadius: 4, fontFamily: "monospace", fontWeight: 600, flexShrink: 0, color: hasError ? "#ef4444" : tc.text, background: hasError ? "rgba(239,68,68,0.1)" : tc.bg }}>
                          {action.tool}
                        </span>
                        <span style={{ fontSize: 11, fontFamily: "monospace", color: "#6b7280", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{action.hash.slice(0, 32)}…</span>
                      </div>
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#4b5563" strokeWidth="2" style={{ transform: isExpanded ? "rotate(180deg)" : "rotate(0deg)", transition: "transform 0.15s", flexShrink: 0 }}>
                        <polyline points="6 9 12 15 18 9"/>
                      </svg>
                    </div>

                    {isExpanded && (
                      <div style={{ padding: "0 16px 16px", borderTop: "1px solid rgba(255,255,255,0.06)", display: "flex", flexDirection: "column", gap: 12 }}>
                        {!!action.input && (
                          <div style={{ paddingTop: 12 }}>
                            <p style={{ fontSize: 10, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 6, color: "#6b7280" }}>INPUT</p>
                            <pre style={{ fontSize: 11, borderRadius: 8, padding: 12, overflowX: "auto", margin: 0, background: "#0a0a0a", color: "#9ca3af", fontFamily: "var(--font-space-mono), monospace", lineHeight: 1.6 }}>
                              {JSON.stringify(action.input, null, 2)}
                            </pre>
                          </div>
                        )}
                        {(!!action.result || !!action.error) && (
                          <div>
                            <p style={{ fontSize: 10, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 6, color: hasError ? "#7f1d1d" : "#6b7280" }}>{hasError ? "ERROR" : "RESULT"}</p>
                            <pre style={{ fontSize: 11, borderRadius: 8, padding: 12, overflowX: "auto", margin: 0, background: hasError ? "#1a0a0a" : "#0a0a0a", color: hasError ? "#fca5a5" : "#6b7280", fontFamily: "var(--font-space-mono), monospace", lineHeight: 1.6 }}>
                              {action.error ?? JSON.stringify(action.result, null, 2)}
                            </pre>
                          </div>
                        )}
                        <div style={{ display: "flex", alignItems: "center", gap: 8, paddingTop: 4 }}>
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={ACCENT} strokeWidth="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
                          <span style={{ fontSize: 11, fontFamily: "monospace", wordBreak: "break-all", color: "#4b5563" }}>SHA256: {action.hash}</span>
                        </div>

                        {action.proof && action.proof.length > 0 && (
                          <div style={{ paddingTop: 4 }}>
                            <p style={{ fontSize: 10, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 6, color: "#6b7280" }}>MERKLE PROOF</p>
                            <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                              {action.proof.map((node, pi) => {
                                const [side, hash] = node.startsWith("left:") ? ["L", node.slice(5)] : ["R", node.slice(6)];
                                return (
                                  <div key={pi} style={{ display: "flex", alignItems: "center", gap: 8, fontFamily: "monospace", fontSize: 10 }}>
                                    <span style={{ padding: "1px 5px", borderRadius: 3, fontWeight: 700, color: side === "L" ? "#60a5fa" : "#f472b6", background: side === "L" ? "rgba(96,165,250,0.1)" : "rgba(244,114,182,0.1)" }}>{side}</span>
                                    <span style={{ color: "#6b7280", wordBreak: "break-all" }}>{hash}</span>
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </>
              );
            })}
          </div>
        </div>
      </main>
    </div>
  );
}
