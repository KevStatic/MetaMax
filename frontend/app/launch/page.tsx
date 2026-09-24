"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch, sessionStreamURL } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import {
  PIPELINE,
  MONAD_EXPLORER,
  MONAD_FAUCET,
  shortHash,
  type ChainSummary,
  type ScoredProvider,
  type StageId,
  type StageStatus,
} from "@/lib/monad";

const BG = "#111111";
const CARD = "#1a1a1a";
const BORDER = "rgba(255,255,255,0.06)";
const ACCENT = "#e2f0d9";
const ACCENT_FG = "#111111";
const GREEN = "#22c55e";
const AMBER = "#eab308";
const RED = "#ef4444";

const DEFAULT_REPO = "https://github.com/vercel/next.js";
const DEFAULT_PROMPT =
  "Deploy this repository as a production web service on port 3000. Pick the best provider on Monad, run it in an isolated container, and attest the execution proof on-chain.";

type StageState = {
  status: StageStatus;
  detail?: string;
  txHash?: string;
  explorerUrl?: string;
};

type LogLine = { ts: number; kind: string; text: string; url?: string };

type WalletCard = {
  wallet: { address: string; balance_mon: string; explorer_url: string; funded: boolean };
  contracts: Record<string, { address: string; explorer_url: string }>;
  escrow_deposit_mon: string;
};

const emptyStages = (): Record<string, StageState> =>
  Object.fromEntries(PIPELINE.map((s) => [s.id, { status: "pending" as StageStatus }]));

export default function LaunchPage() {
  const { isAuthenticated, hydrated, teamId } = useAuth();
  const router = useRouter();

  const [stages, setStages] = useState<Record<string, StageState>>(emptyStages);
  const [log, setLog] = useState<LogLine[]>([]);
  const [running, setRunning] = useState(false);
  const [finished, setFinished] = useState(false);
  const [sessionId, setSessionId] = useState("");
  const [errMsg, setErrMsg] = useState("");
  const [appURL, setAppURL] = useState("");
  const [ranking, setRanking] = useState<ScoredProvider[]>([]);
  const [summary, setSummary] = useState<ChainSummary | null>(null);
  const [agent, setAgent] = useState<WalletCard | null>(null);

  const [showAdvanced, setShowAdvanced] = useState(false);
  const [repoURL, setRepoURL] = useState(DEFAULT_REPO);
  const [prompt, setPrompt] = useState(DEFAULT_PROMPT);

  const wsRef = useRef<WebSocket | null>(null);
  const logEndRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (hydrated && !isAuthenticated) router.replace("/signin");
  }, [hydrated, isAuthenticated, router]);

  // The agent wallet is the thing paying for all of this — show it up front.
  useEffect(() => {
    apiFetch("/agent/wallet")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => d && setAgent(d))
      .catch(() => {});
  }, []);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [log]);

  useEffect(() => () => wsRef.current?.close(), []);

  const pushLog = useCallback((kind: string, text: string, url?: string) => {
    setLog((prev) => [...prev, { ts: Date.now(), kind, text, url }]);
  }, []);

  function applyStage(stage: StageId, next: StageState) {
    setStages((prev) => {
      const current = prev[stage];
      // A "done" stage never regresses to "active" on a later sub-step event.
      if (current?.status === "done" && next.status === "active") {
        return { ...prev, [stage]: { ...current, detail: next.detail ?? current.detail } };
      }
      return { ...prev, [stage]: { ...current, ...next } };
    });
  }

  function connect(sid: string) {
    const url = sessionStreamURL(sid);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onmessage = (e) => {
      let evt: Record<string, unknown>;
      try {
        evt = JSON.parse(e.data);
      } catch {
        return;
      }
      const type = String(evt.type ?? "");

      if (type === "stage") {
        const stage = evt.stage as StageId;
        const status = evt.status as StageStatus;
        const detail = evt.detail as string | undefined;
        const txHash = evt.tx_hash as string | undefined;
        const explorerUrl = evt.explorer_url as string | undefined;

        applyStage(stage, { status, detail, txHash, explorerUrl });

        const def = PIPELINE.find((p) => p.id === stage);
        if (detail) pushLog(status, `${def?.label ?? stage} — ${detail}`, explorerUrl);

        if (stage === "comparing" && Array.isArray(evt.data)) {
          setRanking(evt.data as ScoredProvider[]);
        }
        return;
      }

      if (type === "message") {
        pushLog("message", String(evt.message ?? ""));
        return;
      }
      if (type === "action") {
        const action = evt.action as { tool?: string } | undefined;
        if (action?.tool) pushLog("action", action.tool);
        return;
      }
      if (type === "done") {
        const deployed = (evt.deployed_url as string) ?? "";
        if (deployed) setAppURL(deployed);
        if (evt.data) setSummary(evt.data as ChainSummary);
        pushLog("done", "Deployment complete.");
        setRunning(false);
        setFinished(true);
        ws.close();
        return;
      }
      if (type === "error") {
        const msg = String(evt.error ?? evt.message ?? "Unknown error");
        setErrMsg(msg);
        pushLog("failed", msg);
        setRunning(false);
        setFinished(true);
        ws.close();
      }
    };

    ws.onerror = () => ws.close();
  }

  async function launch() {
    if (!teamId) return;
    setStages(emptyStages());
    setLog([]);
    setRanking([]);
    setSummary(null);
    setErrMsg("");
    setAppURL("");
    setFinished(false);
    setRunning(true);

    try {
      const res = await apiFetch("/sessions", {
        method: "POST",
        body: JSON.stringify({
          team_id: teamId,
          prompt: prompt.trim() || DEFAULT_PROMPT,
          repo_url: repoURL.trim() || undefined,
          autopilot: true,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      const session = await res.json();
      const sid: string = session.id ?? session.session_id;
      setSessionId(sid);
      pushLog("message", `Session ${sid} created — agent is on autopilot.`);
      connect(sid);
    } catch (e) {
      setErrMsg(String(e));
      setRunning(false);
      setFinished(true);
    }
  }

  if (!hydrated || !isAuthenticated) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: BG }}>
        <span className="animate-spin" style={{ display: "inline-block", width: 32, height: 32, borderRadius: "50%", border: "2px solid rgba(255,255,255,0.1)", borderTopColor: "#fff" }} />
      </div>
    );
  }

  const doneCount = PIPELINE.filter((p) => stages[p.id]?.status === "done").length;
  const txLinks = [
    { label: "Escrow created", url: summary?.escrow_url, hash: summary?.escrow_tx },
    { label: "Proof attested", url: summary?.attest_url, hash: summary?.attest_tx },
    { label: "Reputation recorded", url: summary?.reputation_url, hash: summary?.reputation_tx },
    { label: "Payment released", url: summary?.settle_url, hash: summary?.settle_tx },
  ].filter((t) => t.hash);

  return (
    <div style={{ display: "flex", height: "100vh", background: BG, fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32, maxWidth: 1240, margin: "0 auto" }}>

          {/* Header */}
          <header style={{ marginBottom: 24 }}>
            <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Autonomous Deployment</p>
            <p style={{ fontSize: 13, fontFamily: "monospace", color: "#6b7280", marginTop: 4 }}>
              One button. The agent picks a provider, pays it, runs your workload, and proves it on Monad.
            </p>
          </header>

          {/* Agent wallet strip */}
          {agent?.wallet?.address && (
            <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 16, padding: "12px 16px", marginBottom: 20, background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12 }}>
              <div>
                <p style={{ fontSize: 11, color: "#6b7280" }}>Agent wallet</p>
                <a href={agent.wallet.explorer_url} target="_blank" rel="noreferrer" style={{ fontSize: 13, fontFamily: "monospace", color: ACCENT, textDecoration: "none" }}>
                  {shortHash(agent.wallet.address, 8, 6)} ↗
                </a>
              </div>
              <div>
                <p style={{ fontSize: 11, color: "#6b7280" }}>Balance</p>
                <p style={{ fontSize: 13, fontWeight: 700, color: agent.wallet.funded ? GREEN : RED }}>
                  {agent.wallet.balance_mon} MON
                </p>
              </div>
              <div>
                <p style={{ fontSize: 11, color: "#6b7280" }}>Escrow per job</p>
                <p style={{ fontSize: 13, fontWeight: 700, color: "#f9fafb" }}>{agent.escrow_deposit_mon} MON</p>
              </div>
              <div style={{ marginLeft: "auto", display: "flex", gap: 8, flexWrap: "wrap" }}>
                {Object.entries(agent.contracts ?? {})
                  .filter(([, c]) => c.address)
                  .map(([name, c]) => (
                    <a
                      key={name}
                      href={c.explorer_url}
                      target="_blank"
                      rel="noreferrer"
                      style={{ fontSize: 11, fontFamily: "monospace", padding: "4px 10px", borderRadius: 999, background: "#161618", border: `1px solid ${BORDER}`, color: "#9ca3af", textDecoration: "none" }}
                    >
                      {name.replace(/_/g, " ")} ↗
                    </a>
                  ))}
              </div>
              {!agent.wallet.funded && (
                <p style={{ fontSize: 11, color: AMBER, width: "100%" }}>
                  The agent wallet holds no MON — on-chain steps will be skipped. Fund it at{" "}
                  <a href={MONAD_FAUCET} target="_blank" rel="noreferrer" style={{ color: AMBER }}>{MONAD_FAUCET}</a>.
                </p>
              )}
            </div>
          )}

          {/* The button */}
          <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 14, padding: 24, marginBottom: 24 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 20, flexWrap: "wrap" }}>
              <button
                onClick={launch}
                disabled={running}
                style={{
                  padding: "18px 34px", borderRadius: 12, border: "none",
                  background: running ? "rgba(226,240,217,0.25)" : ACCENT,
                  color: ACCENT_FG, fontSize: 17, fontWeight: 900,
                  cursor: running ? "default" : "pointer",
                  boxShadow: running ? "none" : "0 8px 28px rgba(226,240,217,0.16)",
                }}
              >
                {running ? `Deploying… ${doneCount}/${PIPELINE.length}` : finished ? "↻ Deploy again" : "🚀 Deploy my AI workload"}
              </button>
              <div style={{ flex: 1, minWidth: 220 }}>
                <p style={{ fontSize: 13, color: "#9ca3af", lineHeight: 1.5 }}>
                  Every on-chain step below links straight to MonadScan, so nothing here has to be taken on trust.
                </p>
                <button
                  onClick={() => setShowAdvanced((v) => !v)}
                  style={{ marginTop: 6, fontSize: 12, fontWeight: 600, color: ACCENT, background: "transparent", border: "none", cursor: "pointer", padding: 0 }}
                >
                  {showAdvanced ? "Hide workload settings ↑" : "Change the workload ↓"}
                </button>
              </div>
              {sessionId && (
                <Link href={`/sessions/${sessionId}`} style={{ fontSize: 12, color: "#6b7280", textDecoration: "none" }}>
                  Session {shortHash(sessionId, 8, 4)} →
                </Link>
              )}
            </div>

            {showAdvanced && (
              <div style={{ marginTop: 18, paddingTop: 18, borderTop: `1px solid ${BORDER}`, display: "flex", flexDirection: "column", gap: 10 }}>
                <input
                  value={repoURL}
                  onChange={(e) => setRepoURL(e.target.value)}
                  placeholder="https://github.com/owner/repo"
                  style={{ padding: "10px 12px", borderRadius: 8, border: `1px solid ${BORDER}`, background: BG, color: "#e5e7eb", fontSize: 13, outline: "none" }}
                />
                <textarea
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  rows={3}
                  style={{ padding: 12, borderRadius: 8, border: `1px solid ${BORDER}`, background: BG, color: "#e5e7eb", fontSize: 13, resize: "vertical", outline: "none", fontFamily: "inherit" }}
                />
              </div>
            )}
          </div>

          {errMsg && (
            <div style={{ padding: 14, marginBottom: 20, borderRadius: 10, background: "rgba(239,68,68,0.08)", border: "1px solid rgba(239,68,68,0.3)" }}>
              <p style={{ fontSize: 13, color: RED, fontFamily: "monospace", whiteSpace: "pre-wrap" }}>{errMsg}</p>
            </div>
          )}

          <div style={{ display: "grid", gridTemplateColumns: "minmax(0,1fr) minmax(0,1fr)", gap: 24 }}>

            {/* Pipeline */}
            <section>
              <h2 style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb", marginBottom: 14 }}>Pipeline</h2>
              {PIPELINE.map((def, i) => {
                const st = stages[def.id] ?? { status: "pending" as StageStatus };
                const color =
                  st.status === "done" ? GREEN :
                  st.status === "active" ? ACCENT :
                  st.status === "failed" ? RED :
                  st.status === "skipped" ? "#6b7280" : "#4b5563";
                return (
                  <div key={def.id} style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "0 14px" }}>
                    <div style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
                      <div style={{
                        width: 26, height: 26, borderRadius: "50%", display: "flex", alignItems: "center", justifyContent: "center",
                        background: st.status === "done" ? "rgba(34,197,94,0.15)" : st.status === "active" ? "rgba(226,240,217,0.15)" : st.status === "failed" ? "rgba(239,68,68,0.12)" : "#1c1c1e",
                        border: st.status === "active" ? `1px solid rgba(226,240,217,0.4)` : "1px solid transparent",
                      }}>
                        {st.status === "done" ? (
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={GREEN} strokeWidth="3"><polyline points="20 6 9 17 4 12" /></svg>
                        ) : st.status === "active" ? (
                          <span className="animate-pulse" style={{ width: 8, height: 8, borderRadius: "50%", background: ACCENT }} />
                        ) : st.status === "failed" ? (
                          <span style={{ color: RED, fontSize: 13, fontWeight: 800, lineHeight: 1 }}>!</span>
                        ) : (
                          <span style={{ width: 6, height: 6, borderRadius: "50%", background: "#4b5563" }} />
                        )}
                      </div>
                      {i < PIPELINE.length - 1 && <div style={{ width: 1, flex: 1, minHeight: 22, background: st.status === "done" ? "rgba(34,197,94,0.3)" : "#2c2c2e" }} />}
                    </div>

                    <div style={{ paddingBottom: 18 }}>
                      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                        <p style={{ fontSize: 13, fontWeight: 700, color }}>{def.label}</p>
                        {def.onchain && (
                          <span style={{ fontSize: 9, fontWeight: 700, letterSpacing: 0.4, padding: "2px 6px", borderRadius: 4, background: "rgba(139,92,246,0.14)", color: "#a78bfa" }}>
                            ON-CHAIN
                          </span>
                        )}
                        {st.status === "skipped" && (
                          <span style={{ fontSize: 9, fontWeight: 700, padding: "2px 6px", borderRadius: 4, background: "rgba(255,255,255,0.06)", color: "#6b7280" }}>SKIPPED</span>
                        )}
                      </div>
                      <p style={{ fontSize: 11, color: "#6b7280", marginTop: 2, lineHeight: 1.5 }}>{st.detail || def.blurb}</p>
                      {st.explorerUrl && st.txHash && (
                        <a
                          href={st.explorerUrl}
                          target="_blank"
                          rel="noreferrer"
                          style={{ display: "inline-block", marginTop: 6, fontSize: 11, fontFamily: "monospace", color: "#a78bfa", textDecoration: "none", padding: "3px 8px", borderRadius: 6, background: "rgba(139,92,246,0.08)", border: "1px solid rgba(139,92,246,0.2)" }}
                        >
                          {shortHash(st.txHash)} ↗ MonadScan
                        </a>
                      )}
                    </div>
                  </div>
                );
              })}
            </section>

            {/* Right column */}
            <section style={{ display: "flex", flexDirection: "column", gap: 20 }}>

              {/* Provider comparison */}
              {ranking.length > 0 && (
                <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, overflow: "hidden" }}>
                  <div style={{ padding: "12px 16px", borderBottom: `1px solid ${BORDER}` }}>
                    <p style={{ fontSize: 13, fontWeight: 700, color: "#f9fafb" }}>Provider comparison</p>
                    <p style={{ fontSize: 11, color: "#6b7280", marginTop: 2 }}>
                      {ranking.length} node(s) scored on price (45%), reputation (30%) and stake (25%), minus slash penalties.
                    </p>
                  </div>
                  <div style={{ overflowX: "auto" }}>
                    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 11 }}>
                      <thead>
                        <tr style={{ color: "#6b7280", textAlign: "left" }}>
                          {["#", "Provider", "MON/hr", "Jobs", "Latency", "Score"].map((h) => (
                            <th key={h} style={{ padding: "8px 12px", fontWeight: 600, whiteSpace: "nowrap" }}>{h}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {ranking.map((p) => {
                          const winner = p.rank === 1 && p.latency_ms >= 0;
                          return (
                            <tr key={p.wallet} style={{ borderTop: `1px solid ${BORDER}`, background: winner ? "rgba(226,240,217,0.06)" : "transparent" }}>
                              <td style={{ padding: "8px 12px", color: winner ? ACCENT : "#6b7280", fontWeight: 700 }}>{p.rank}</td>
                              <td style={{ padding: "8px 12px", fontFamily: "monospace" }}>
                                <a href={p.explorer_url} target="_blank" rel="noreferrer" style={{ color: winner ? ACCENT : "#9ca3af", textDecoration: "none" }}>
                                  {shortHash(p.wallet, 6, 4)}
                                </a>
                              </td>
                              <td style={{ padding: "8px 12px", color: "#d1d5db", whiteSpace: "nowrap" }}>{p.price_per_hour_mon}</td>
                              <td style={{ padding: "8px 12px", color: "#d1d5db" }}>{p.jobs_completed}</td>
                              <td style={{ padding: "8px 12px", color: p.latency_ms < 0 ? RED : "#d1d5db", whiteSpace: "nowrap" }}>
                                {p.latency_ms < 0 ? "offline" : `${p.latency_ms} ms`}
                              </td>
                              <td style={{ padding: "8px 12px", color: winner ? ACCENT : "#d1d5db", fontWeight: 700 }}>{p.score.toFixed(1)}</td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* Monad receipt */}
              {txLinks.length > 0 && (
                <div style={{ background: CARD, border: "1px solid rgba(139,92,246,0.25)", borderRadius: 12, padding: 16 }}>
                  <p style={{ fontSize: 13, fontWeight: 700, color: "#a78bfa", marginBottom: 10 }}>Monad receipt</p>
                  <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                    {txLinks.map((t) => (
                      <div key={t.label} style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center" }}>
                        <span style={{ fontSize: 12, color: "#9ca3af" }}>{t.label}</span>
                        <a href={t.url} target="_blank" rel="noreferrer" style={{ fontSize: 11, fontFamily: "monospace", color: "#a78bfa", textDecoration: "none" }}>
                          {shortHash(t.hash!)} ↗
                        </a>
                      </div>
                    ))}
                    {summary?.merkle_root && (
                      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center", paddingTop: 8, borderTop: `1px solid ${BORDER}` }}>
                        <span style={{ fontSize: 12, color: "#9ca3af" }}>Merkle root</span>
                        <span style={{ fontSize: 11, fontFamily: "monospace", color: "#d1d5db" }}>{shortHash(summary.merkle_root)}</span>
                      </div>
                    )}
                    {summary?.provider_explorer_url && (
                      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center" }}>
                        <span style={{ fontSize: 12, color: "#9ca3af" }}>Paid provider</span>
                        <a href={summary.provider_explorer_url} target="_blank" rel="noreferrer" style={{ fontSize: 11, fontFamily: "monospace", color: "#a78bfa", textDecoration: "none" }}>
                          {shortHash(summary.provider!, 8, 6)} ↗
                        </a>
                      </div>
                    )}
                  </div>
                  <div style={{ display: "flex", gap: 10, marginTop: 14 }}>
                    {appURL && (
                      <a href={appURL} target="_blank" rel="noreferrer" style={{ flex: 1, textAlign: "center", padding: "9px 14px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 12, fontWeight: 700, textDecoration: "none" }}>
                        Open App ↗
                      </a>
                    )}
                    <Link href={`/audit?session=${sessionId}`} style={{ flex: 1, textAlign: "center", padding: "9px 14px", borderRadius: 8, background: "rgba(255,255,255,0.07)", color: "#e5e7eb", fontSize: 12, fontWeight: 700, textDecoration: "none" }}>
                      Verify proof
                    </Link>
                  </div>
                </div>
              )}

              {/* Live log */}
              <div style={{ background: "#0a0a0a", border: `1px solid ${BORDER}`, borderRadius: 12, overflow: "hidden", display: "flex", flexDirection: "column", minHeight: 240, maxHeight: 460 }}>
                <div style={{ padding: "10px 14px", borderBottom: `1px solid ${BORDER}`, display: "flex", alignItems: "center", gap: 8 }}>
                  <span className={running ? "animate-pulse" : undefined} style={{ width: 7, height: 7, borderRadius: "50%", background: running ? ACCENT : "#4b5563" }} />
                  <span style={{ fontSize: 12, fontWeight: 700, color: "#f9fafb" }}>Agent log</span>
                  <span style={{ marginLeft: "auto", fontSize: 11, color: "#4b5563", fontFamily: "monospace" }}>{MONAD_EXPLORER.replace("https://", "")}</span>
                </div>
                <div style={{ flex: 1, overflowY: "auto", padding: 12, fontFamily: "monospace", fontSize: 11, display: "flex", flexDirection: "column", gap: 3 }}>
                  {log.length === 0 && <p style={{ color: "#4b5563" }}>Press the button to start.</p>}
                  {log.map((l, i) => {
                    const color =
                      l.kind === "done" ? GREEN :
                      l.kind === "failed" ? RED :
                      l.kind === "skipped" ? "#6b7280" :
                      l.kind === "active" ? ACCENT : "#9ca3af";
                    return (
                      <div key={i} style={{ lineHeight: 1.5 }}>
                        <span style={{ color: "#374151" }}>{new Date(l.ts).toLocaleTimeString()} </span>
                        <span style={{ color }}>{l.text}</span>
                        {l.url && (
                          <a href={l.url} target="_blank" rel="noreferrer" style={{ color: "#a78bfa", marginLeft: 6, textDecoration: "none" }}>↗</a>
                        )}
                      </div>
                    );
                  })}
                  <div ref={logEndRef} />
                </div>
              </div>
            </section>
          </div>
        </div>
      </main>
    </div>
  );
}
