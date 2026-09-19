"use client";

import { useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";

export default function VaultPage() {
  const { address } = useAuth();
  const [nonce, setNonce] = useState("");
  const [vaultKey, setVaultKey] = useState("");
  const [containerId, setContainerId] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function fetchNonce() {
    setLoading(true);
    setError("");
    try {
      const r = await apiFetch("/vault/nonce");
      if (!r.ok) throw new Error(await r.text());
      const { nonce: n } = await r.json();
      setNonce(n);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  async function fetchKey() {
    if (!containerId.trim()) return;
    setLoading(true);
    setError("");
    try {
      const r = await apiFetch("/vault/key", {
        method: "POST",
        body: JSON.stringify({ container_id: containerId.trim(), nonce }),
      });
      if (!r.ok) throw new Error(await r.text());
      const { key } = await r.json();
      setVaultKey(key);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ marginBottom: 32 }}>
            <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Vault</p>
            <p style={{ fontSize: 13, color: "#6b7280", marginTop: 4 }}>Retrieve LUKS2 container encryption keys</p>
          </header>

          <div style={{ maxWidth: 480, display: "flex", flexDirection: "column", gap: 16 }}>
            {error && (
              <div style={{ padding: 12, borderRadius: 8, background: "rgba(239,68,68,0.1)", border: "1px solid rgba(239,68,68,0.3)", color: "#fca5a5", fontSize: 13 }}>
                {error}
              </div>
            )}

            <div style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.06)", borderRadius: 12, padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
              <div>
                <p style={{ fontSize: 14, fontWeight: 600, color: "#f9fafb", marginBottom: 4 }}>Step 1 — Get Vault Nonce</p>
                <p style={{ fontSize: 12, color: "#6b7280" }}>A single-use nonce is required to authorize key retrieval.</p>
              </div>
              <button
                onClick={fetchNonce}
                disabled={loading}
                style={{ padding: "10px 20px", borderRadius: 8, background: "#e2f0d9", color: "#111111", fontSize: 13, fontWeight: 700, border: "none", cursor: loading ? "default" : "pointer", opacity: loading ? 0.6 : 1, alignSelf: "flex-start" }}
              >
                {loading ? "Loading…" : "Get Nonce"}
              </button>
              {nonce && (
                <div style={{ padding: "8px 12px", borderRadius: 8, background: "#0a0a0a", border: "1px solid rgba(255,255,255,0.05)" }}>
                  <p style={{ fontSize: 11, color: "#4b5563", marginBottom: 4 }}>Nonce</p>
                  <code style={{ fontSize: 11, fontFamily: "monospace", color: "#9ca3af" }}>{nonce}</code>
                </div>
              )}
            </div>

            {nonce && (
              <div style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.06)", borderRadius: 12, padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
                <div>
                  <p style={{ fontSize: 14, fontWeight: 600, color: "#f9fafb", marginBottom: 4 }}>Step 2 — Retrieve Key</p>
                  <p style={{ fontSize: 12, color: "#6b7280" }}>Enter the container ID to retrieve its encryption key.</p>
                </div>
                <input
                  type="text"
                  placeholder="Container ID"
                  value={containerId}
                  onChange={(e) => setContainerId(e.target.value)}
                  style={{ padding: "10px 12px", borderRadius: 8, border: "1px solid rgba(255,255,255,0.06)", background: "#111111", color: "#e5e7eb", fontSize: 13, outline: "none" }}
                />
                <button
                  onClick={fetchKey}
                  disabled={loading || !containerId.trim()}
                  style={{ padding: "10px 20px", borderRadius: 8, background: "#e2f0d9", color: "#111111", fontSize: 13, fontWeight: 700, border: "none", cursor: !containerId.trim() || loading ? "default" : "pointer", opacity: !containerId.trim() || loading ? 0.4 : 1, alignSelf: "flex-start" }}
                >
                  {loading ? "Loading…" : "Get Key"}
                </button>
                {vaultKey && (
                  <div style={{ padding: "8px 12px", borderRadius: 8, background: "#0a0a0a", border: "1px solid rgba(255,255,255,0.05)" }}>
                    <p style={{ fontSize: 11, color: "#4b5563", marginBottom: 4 }}>Encryption Key (keep secret)</p>
                    <code style={{ fontSize: 11, fontFamily: "monospace", color: "#e2f0d9", wordBreak: "break-all" }}>{vaultKey}</code>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
