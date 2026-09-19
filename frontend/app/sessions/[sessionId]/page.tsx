"use client";

import { use, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch, getSessionAudit, sessionStreamURL, type ActionItem } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import { formatTime, formatDuration } from "@/lib/utils";

type Session = {
  id: string;
  team_id: string;
  prompt: string;
  state: "running" | "completed" | "failed";
  merkle_root?: string;
  attestation_tx?: string;
  created_at: string;
  updated_at: string;
};

type Plan = {
  summary: string;
  containers?: { image?: string; ports?: string[] }[];
  estimated_cost_usd?: number;
};

type LiveEvent = {
  type: string;
  ts: number;
  message?: string;
  action?: unknown;
  plan?: Plan;
  deployed_url?: string;
  error?: string;
  data?: unknown;
};

type WsStatus = "connecting" | "connected" | "reconnecting" | "disconnected";

const toolColors: Record<string, string> = {
  bash: "#f59e0b",
  write_file: "#60a5fa",
  read_file: "#a78bfa",
  clone_repo: "#34d399",
  install_packages: "#fb923c",
  run_command: "#f59e0b",
  start_process: "#22c55e",
  setup_ide: "#818cf8",
  setup_database: "#2dd4bf",
  health_check: "#4ade80",
  get_logs: "#94a3b8",
  destroy: "#ef4444",
};

export default function SessionPage({ params }: { params: Promise<{ sessionId: string }> }) {
  const { sessionId } = use(params);
  const { isAuthenticated, isConnected, hydrated } = useAuth();

  const [session, setSession] = useState<Session | null>(null);
  const [actions, setActions] = useState<ActionItem[]>([]);
  const [liveEvents, setLiveEvents] = useState<LiveEvent[]>([]);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [appURL, setAppURL] = useState("");
  const [wsStatus, setWsStatus] = useState<WsStatus>("disconnected");
  const [confirming, setConfirming] = useState(false);
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const [copied, setCopied] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const wsRef = useRef<WebSocket | null>(null);
  const liveEndRef = useRef<HTMLDivElement | null>(null);
  const intentionalCloseRef = useRef(false);
  const reconnectAttemptsRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      intentionalCloseRef.current = true;
      wsRef.current?.close();
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
    };
  }, []);

  useEffect(() => {
    if (!hydrated || !isAuthenticated) return;
    apiFetch(`/sessions/${sessionId}`)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error("Session not found"))))
      .then((s: Session) => {
        setSession(s);
        if (s.state === "running") {
          connectWS(sessionId);
        } else {
          loadAudit();
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [hydrated, isAuthenticated, sessionId]); // eslint-disable-line react-hooks/exhaustive-deps

  async function loadAudit() {
    try {
      const log = await getSessionAudit(sessionId);
      setActions(log.actions ?? []);
    } catch {
      // non-fatal
    }
  }

  function scheduleReconnect(sid: string) {
    if (reconnectAttemptsRef.current >= 5) { setWsStatus("disconnected"); return; }
    setWsStatus("reconnecting");
    const delay = Math.min(1000 * 2 ** reconnectAttemptsRef.current, 16000);
    reconnectTimerRef.current = setTimeout(() => {
      reconnectAttemptsRef.current += 1;
      connectWS(sid);
    }, delay);
  }

  function connectWS(sid: string) {
    intentionalCloseRef.current = false;
    setWsStatus("connecting");
    const url = sessionStreamURL(sid);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => { setWsStatus("connected"); reconnectAttemptsRef.current = 0; };

    ws.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data) as LiveEvent & Record<string, unknown>;
        setLiveEvents((prev) => [...prev, { ...evt, ts: evt.ts ?? Date.now() }]);

        if (evt.type === "plan") {
          const p = (evt.plan ?? evt.data) as Plan | undefined;
          if (p) setPlan(p);
        } else if (evt.type === "done") {
          const url = evt.deployed_url ?? (evt.data as { url?: string })?.url ?? "";
          if (url) setAppURL(url);
          setSession((prev) => prev ? { ...prev, state: "completed" } : prev);
          intentionalCloseRef.current = true;
          ws.close();
          setWsStatus("disconnected");
          loadAudit();
        } else if (evt.type === "error") {
          setSession((prev) => prev ? { ...prev, state: "failed" } : prev);
          intentionalCloseRef.current = true;
          ws.close();
          setWsStatus("disconnected");
        }

        liveEndRef.current?.scrollIntoView({ behavior: "smooth" });
      } catch {
        // ignore
      }
    };

    ws.onclose = () => { if (!intentionalCloseRef.current) scheduleReconnect(sid); };
    ws.onerror = () => ws.close();
  }

  async function handleConfirm(approved: boolean) {
    setConfirming(true);
    try {
      await apiFetch(`/sessions/${sessionId}/confirm`, {
        method: "POST",
        body: JSON.stringify({ approved }),
      });
      setPlan(null);
      if (!approved) {
        setSession((prev) => prev ? { ...prev, state: "failed" } : prev);
        intentionalCloseRef.current = true;
        wsRef.current?.close();
      }
    } finally {
      setConfirming(false);
    }
  }

  function copyHash(hash: string, idx: number) {
    navigator.clipboard.writeText(hash);
    setCopied(idx);
    setTimeout(() => setCopied(null), 1500);
  }

  if (loading) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#111111", color: "#6b7280" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <span className="animate-spin" style={{ display: "inline-block", width: 16, height: 16, borderRadius: "50%", border: "2px solid currentColor", borderTopColor: "transparent" }} />
          {isConnected && !isAuthenticated ? "Authenticating…" : "Loading session…"}
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#111111", color: "#6b7280" }}>
        <div style={{ textAlign: "center" }}>
          <p style={{ fontWeight: 700, color: "#f3f4f6", marginBottom: 8 }}>Wallet not connected</p>
          <p style={{ fontSize: 14, opacity: 0.6, marginBottom: 16 }}>Connect your wallet to view session details.</p>
          <Link href="/" style={{ fontSize: 14, textDecoration: "underline", color: "#e2f0d9" }}>← Back to dashboard</Link>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#111111", color: "#ef4444" }}>
        <div style={{ textAlign: "center" }}>
          <p style={{ fontWeight: 700, marginBottom: 8 }}>Session not found</p>
          <p style={{ fontSize: 14, opacity: 0.6, marginBottom: 16 }}>{error}</p>
          <Link href="/" style={{ fontSize: 14, textDecoration: "underline", color: "#e2f0d9" }}>← Back to dashboard</Link>
        </div>
      </div>
    );
  }

  const stateColor = session?.state === "completed" ? "#22c55e" : session?.state === "failed" ? "#ef4444" : "#eab308";

  return (
    <div style={{ minHeight: "100vh", display: "flex", background: "#111111", color: "#d1d5db", fontFamily: "var(--font-inter), sans-serif" }}>
      <Sidebar mode="user" />
      <div style={{ flex: 1 }}>
        {/* Nav */}
        <header style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "16px 24px", borderBottom: "1px solid #1f2937" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <Link href="/" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 14, color: "#6b7280", textDecoration: "none" }}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="m15 18-6-6 6-6"/></svg>
              Dashboard
            </Link>
            <span style={{ color: "#374151" }}>/</span>
            <Link href="/sessions" style={{ fontSize: 14, color: "#6b7280", textDecoration: "none" }}>Sessions</Link>
            <span style={{ color: "#374151" }}>/</span>
            <span style={{ fontSize: 14, fontWeight: 600, color: "#f3f4f6", fontFamily: "var(--font-space-mono), monospace" }}>
              {sessionId.slice(0, 20)}…
            </span>
          </div>
          <Link href="/deploy" style={{ fontSize: 12, padding: "6px 16px", borderRadius: 4, fontWeight: 600, background: "#e2f0d9", color: "#111111", textDecoration: "none" }}>
            New Deploy
          </Link>
        </header>

        <div style={{ maxWidth: 900, margin: "0 auto", padding: "40px 24px" }}>

          {/* Session header */}
          <div style={{ borderRadius: 4, padding: 24, marginBottom: 24, background: "#181818", border: "1px solid #1f2937" }}>
            <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 16, marginBottom: 16 }}>
              <div style={{ flex: 1 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 8 }}>
                  <span style={{ width: 10, height: 10, borderRadius: 2, background: stateColor, boxShadow: `0 0 6px ${stateColor}`, display: "inline-block" }} />
                  <span style={{ fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.1em", color: stateColor }}>
                    {session?.state}
                  </span>
                </div>
                <p style={{ fontSize: 15, fontWeight: 500, color: "#f3f4f6" }}>{session?.prompt}</p>
              </div>
              <div style={{ textAlign: "right", flexShrink: 0 }}>
                <div style={{ fontSize: 11, color: "#4b5563", marginBottom: 4 }}>Started</div>
                <div style={{ fontSize: 11, fontFamily: "monospace", color: "#6b7280" }}>
                  {session ? formatTime(session.created_at) : "—"}
                </div>
              </div>
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, paddingTop: 16, borderTop: "1px solid #1f2937" }}>
              {[
                { label: "Session ID", value: session?.id ?? "—", mono: true },
                { label: "Team ID", value: session?.team_id ?? "—", mono: true },
                { label: "Actions", value: String(actions.length) },
                { label: "Duration", value: session ? formatDuration(session.created_at, session.updated_at) : "—" },
              ].map((f) => (
                <div key={f.label}>
                  <div style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 4, color: "#4b5563" }}>{f.label}</div>
                  <div style={{ fontSize: f.mono ? 11 : 14, fontWeight: f.mono ? 400 : 700, fontFamily: f.mono ? "monospace" : "inherit", color: f.mono ? "#6b7280" : "#f3f4f6", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {f.value}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Merkle / attestation banner */}
          {session?.merkle_root && (
            <div style={{ borderRadius: 4, padding: 16, marginBottom: 24, display: "flex", alignItems: "center", gap: 12, background: "#181818", border: "1px solid #1f2937" }}>
              <span style={{ fontSize: 14 }}>🔏</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 11, fontWeight: 600, marginBottom: 2, color: "#5c6e8c" }}>On-Chain Audit Root</div>
                <div style={{ fontSize: 11, fontFamily: "monospace", color: "#4b5563", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {session.merkle_root}
                </div>
              </div>
              {session.attestation_tx && (
                <a
                  href={`https://testnet.monadscan.com/`}
                  target="_blank"
                  rel="noreferrer"
                  style={{ fontSize: 11, padding: "4px 12px", borderRadius: 4, background: "#1f2937", color: "#5c6e8c", textDecoration: "none", flexShrink: 0 }}
                >
                  Verify on EAS →
                </a>
              )}
            </div>
          )}

          {/* App URL banner */}
          {appURL && (
            <div style={{ borderRadius: 4, padding: 16, marginBottom: 24, display: "flex", alignItems: "center", gap: 12, background: "#0a1a0a", border: "1px solid #14532d" }}>
              <span style={{ fontSize: 16 }}>🚀</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 11, fontWeight: 600, marginBottom: 2, color: "#22c55e" }}>App Deployed</div>
                <a href={appURL} target="_blank" rel="noreferrer" style={{ fontSize: 11, fontFamily: "monospace", color: "#4ade80" }}>{appURL}</a>
              </div>
              <a href={appURL} target="_blank" rel="noreferrer" style={{ fontSize: 11, padding: "4px 12px", borderRadius: 4, background: "#14532d", color: "#4ade80", textDecoration: "none", flexShrink: 0 }}>
                Open App ↗
              </a>
            </div>
          )}

          {/* Plan confirmation panel */}
          {plan && (
            <div style={{ borderRadius: 4, padding: 20, marginBottom: 24, background: "#0c0f1a", border: "1px solid #1e3a5f" }}>
              <p style={{ fontSize: 14, fontWeight: 700, marginBottom: 12, color: "#93c5fd" }}>📋 Agent Deployment Plan — Awaiting Approval</p>
              <p style={{ fontSize: 13, marginBottom: 16, color: "#6b7280" }}>{plan.summary}</p>
              {plan.containers?.map((c, i) => (
                <div key={i} style={{ borderRadius: 4, padding: 12, marginBottom: 8, background: "#111827", border: "1px solid #1f2937" }}>
                  <p style={{ fontSize: 12, fontWeight: 600, fontFamily: "monospace", color: "#93c5fd" }}>{c.image}</p>
                  {c.ports && c.ports.length > 0 && <p style={{ fontSize: 11, color: "#4b5563", marginTop: 4 }}>Ports: {c.ports.join(", ")}</p>}
                </div>
              ))}
              {plan.estimated_cost_usd != null && (
                <p style={{ fontSize: 12, color: "#6b7280", marginBottom: 16 }}>
                  Estimated: <span style={{ color: "#f3f4f6", fontWeight: 700 }}>${plan.estimated_cost_usd.toFixed(4)} USDC</span>
                </p>
              )}
              <div style={{ display: "flex", gap: 12 }}>
                <button
                  disabled={confirming}
                  onClick={() => handleConfirm(true)}
                  style={{ fontSize: 12, padding: "8px 16px", borderRadius: 4, fontWeight: 600, background: "#e2f0d9", color: "#111111", border: "none", cursor: confirming ? "default" : "pointer", opacity: confirming ? 0.5 : 1 }}
                >
                  {confirming ? "Approving…" : "✓ Approve & Deploy"}
                </button>
                <button
                  disabled={confirming}
                  onClick={() => handleConfirm(false)}
                  style={{ fontSize: 12, padding: "8px 16px", borderRadius: 4, fontWeight: 600, background: "#1f2937", color: "#9ca3af", border: "none", cursor: confirming ? "default" : "pointer", opacity: confirming ? 0.5 : 1 }}
                >
                  ✗ Cancel
                </button>
              </div>
            </div>
          )}

          {/* Live event stream */}
          {(liveEvents.length > 0 || session?.state === "running") && (
            <div style={{ marginBottom: 24 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12 }}>
                <h2 style={{ fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.08em", color: "#4b5563" }}>
                  Live Stream — {liveEvents.length} events
                </h2>
                {session?.state === "running" && (
                  <span style={{
                    fontSize: 11, padding: "2px 8px", borderRadius: 4, fontWeight: 600,
                    background: wsStatus === "connected" ? "#052e16" : wsStatus === "reconnecting" ? "#1c0a00" : "#1c1917",
                    color: wsStatus === "connected" ? "#4ade80" : wsStatus === "reconnecting" ? "#fb923c" : "#a8a29e",
                    border: `1px solid ${wsStatus === "connected" ? "#14532d" : wsStatus === "reconnecting" ? "#7c2d12" : "#292524"}`,
                  }}>
                    {wsStatus === "connected" ? "● live" : wsStatus === "reconnecting" ? `↻ reconnecting (${reconnectAttemptsRef.current})` : "◌ connecting…"}
                  </span>
                )}
              </div>
              <div style={{ borderRadius: 4, overflowY: "auto", background: "#0a0a0a", border: "1px solid #1f2937", maxHeight: 256, padding: 12 }}>
                {liveEvents.map((evt, i) => (
                  <div key={i} style={{ display: "flex", gap: 8, marginBottom: 4 }}>
                    <span style={{
                      fontSize: 11, fontWeight: 700, flexShrink: 0, width: 56,
                      color: evt.type === "error" ? "#ef4444" : evt.type === "done" ? "#22c55e" : evt.type === "plan" ? "#93c5fd" : "#6b7280",
                    }}>
                      [{evt.type}]
                    </span>
                    <span style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af" }}>
                      {evt.type === "message" && (typeof evt.message === "string" ? evt.message : JSON.stringify(evt.message))}
                      {evt.type === "action" && JSON.stringify(evt.action).slice(0, 120)}
                      {evt.type === "plan" && (evt.plan?.summary ?? JSON.stringify(evt.plan).slice(0, 120))}
                      {evt.type === "done" && (evt.deployed_url ?? "done")}
                      {evt.type === "error" && (typeof evt.error === "string" ? evt.error : "error")}
                    </span>
                  </div>
                ))}
                <div ref={liveEndRef} />
              </div>
            </div>
          )}

          {/* Action log */}
          <div>
            <h2 style={{ fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.08em", color: "#4b5563", marginBottom: 16 }}>
              Action Log — {actions.length} tool calls
            </h2>

            {actions.length === 0 && (
              <div style={{ borderRadius: 4, padding: 32, textAlign: "center", background: "#181818", border: "1px solid #1f2937" }}>
                <p style={{ fontSize: 14, color: "#4b5563" }}>
                  {session?.state === "running" ? "Session is still running…" : "No actions recorded."}
                </p>
              </div>
            )}

            <div style={{ position: "relative" }}>
              {actions.length > 0 && (
                <div style={{ position: "absolute", left: 20, top: 0, bottom: 0, width: 1, background: "linear-gradient(to bottom, #1f2937, transparent)" }} />
              )}
              <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                {actions.map((action, i) => {
                  const color = toolColors[action.tool] ?? "#6b7280";
                  const hasError = !!action.error;
                  const isExpanded = expanded[i];

                  return (
                    <div key={i} style={{ position: "relative", paddingLeft: 48 }}>
                      <div style={{
                        position: "absolute", left: 14, top: 16, width: 12, height: 12, borderRadius: 2,
                        background: hasError ? "#7f1d1d" : "#0e0e0e",
                        border: `2px solid ${hasError ? "#ef4444" : color}`,
                        boxShadow: `0 0 6px ${hasError ? "#ef444444" : color + "44"}`,
                      }} />

                      <div style={{ borderRadius: 4, overflow: "hidden", background: "#181818", border: `1px solid ${hasError ? "#7f1d1d" : "#1f2937"}` }}>
                        <button
                          onClick={() => setExpanded((e) => ({ ...e, [i]: !e[i] }))}
                          style={{ width: "100%", display: "flex", alignItems: "center", gap: 12, padding: "12px 16px", textAlign: "left", background: "transparent", border: "none", cursor: "pointer", color: "inherit" }}
                        >
                          <span style={{ fontSize: 12, fontWeight: 600, fontFamily: "var(--font-space-mono), monospace", color }}>{action.tool}</span>
                          <span style={{ fontSize: 11, color: "#4b5563" }}>#{action.index}</span>
                          <span style={{ marginLeft: "auto", fontSize: 11, color: "#374151" }}>{formatTime(action.timestamp)}</span>
                          {hasError
                            ? <span style={{ fontSize: 11, fontWeight: 600, color: "#ef4444" }}>✗ error</span>
                            : <span style={{ fontSize: 11, fontWeight: 600, color: "#22c55e" }}>✓</span>
                          }
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#4b5563" strokeWidth="2" style={{ transform: isExpanded ? "rotate(180deg)" : "none", transition: "transform 0.15s" }}>
                            <path d="m6 9 6 6 6-6"/>
                          </svg>
                        </button>

                        {isExpanded && (
                          <div style={{ padding: "0 16px 16px", borderTop: "1px solid #1f2937" }}>
                            <div style={{ paddingTop: 12 }}>
                              <div style={{ fontSize: 10, textTransform: "uppercase", letterSpacing: "0.08em", color: "#4b5563", marginBottom: 6 }}>Input</div>
                              <pre style={{ fontSize: 11, padding: 12, borderRadius: 8, background: "#0a0a0a", color: "#9ca3af", fontFamily: "var(--font-space-mono), monospace", lineHeight: 1.6, overflowX: "auto", margin: 0 }}>
                                {JSON.stringify(action.input, null, 2)}
                              </pre>
                            </div>
                            {(action.result !== undefined || action.error) && (
                              <div style={{ marginTop: 12 }}>
                                <div style={{ fontSize: 10, textTransform: "uppercase", letterSpacing: "0.08em", color: hasError ? "#7f1d1d" : "#4b5563", marginBottom: 6 }}>
                                  {hasError ? "Error" : "Result"}
                                </div>
                                <pre style={{ fontSize: 11, padding: 12, borderRadius: 8, background: hasError ? "#1a0a0a" : "#0a0a0a", color: hasError ? "#fca5a5" : "#6b7280", fontFamily: "var(--font-space-mono), monospace", lineHeight: 1.6, overflowX: "auto", margin: 0 }}>
                                  {action.error ?? JSON.stringify(action.result, null, 2)}
                                </pre>
                              </div>
                            )}
                            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginTop: 12 }}>
                              <div>
                                <div style={{ fontSize: 10, textTransform: "uppercase", letterSpacing: "0.08em", color: "#374151", marginBottom: 4 }}>Action Hash</div>
                                <code style={{ fontSize: 11, color: "#4b5563", fontFamily: "var(--font-space-mono), monospace" }}>{action.hash}</code>
                              </div>
                              <button
                                onClick={() => copyHash(action.hash, i)}
                                style={{ fontSize: 11, padding: "4px 8px", borderRadius: 8, background: "#1f2937", color: "#6b7280", border: "none", cursor: "pointer" }}
                              >
                                {copied === i ? "Copied!" : "Copy"}
                              </button>
                            </div>
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>

          {/* Footer */}
          <div style={{ marginTop: 40, paddingTop: 24, display: "flex", alignItems: "center", justifyContent: "space-between", borderTop: "1px solid #1f2937" }}>
            <Link href="/" style={{ fontSize: 12, color: "#4b5563", textDecoration: "none" }}>← Dashboard</Link>
            <Link href="/deploy" style={{ fontSize: 12, color: "#e2f0d9", textDecoration: "none" }}>Deploy Again →</Link>
          </div>
        </div>
      </div>
    </div>
  );
}
