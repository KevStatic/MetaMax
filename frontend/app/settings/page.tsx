"use client";

import { useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { useAuth } from "@/lib/AuthContext";

export default function SettingsPage() {
  const { address, teamName, teamId } = useAuth();
  const [copied, setCopied] = useState<string | null>(null);

  function copy(label: string, value: string) {
    navigator.clipboard.writeText(value);
    setCopied(label);
    setTimeout(() => setCopied(null), 1500);
  }

  const fields = [
    { label: "Wallet Address", value: address ?? "—" },
    { label: "Team Name", value: teamName ?? "—" },
    { label: "Team ID", value: teamId ?? "—" },
  ];

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ marginBottom: 32 }}>
            <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Settings</p>
            <p style={{ fontSize: 13, color: "#6b7280", marginTop: 4 }}>Account and workspace information</p>
          </header>

          <div style={{ maxWidth: 560, display: "flex", flexDirection: "column", gap: 16 }}>
            <div style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.06)", borderRadius: 12, overflow: "hidden" }}>
              {fields.map((f, i) => (
                <div
                  key={f.label}
                  style={{
                    padding: "16px 24px",
                    borderBottom: i < fields.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 16,
                  }}
                >
                  <div>
                    <p style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", marginBottom: 4 }}>{f.label}</p>
                    <p style={{ fontSize: 13, fontFamily: f.label !== "Team Name" ? "monospace" : "inherit", color: "#9ca3af", wordBreak: "break-all" }}>{f.value}</p>
                  </div>
                  {f.value !== "—" && (
                    <button
                      onClick={() => copy(f.label, f.value)}
                      style={{ fontSize: 11, padding: "4px 12px", borderRadius: 4, background: "#1f2937", color: "#6b7280", border: "none", cursor: "pointer", flexShrink: 0 }}
                    >
                      {copied === f.label ? "Copied!" : "Copy"}
                    </button>
                  )}
                </div>
              ))}
            </div>

            <div style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.06)", borderRadius: 12, padding: 24 }}>
              <p style={{ fontSize: 14, fontWeight: 600, color: "#f9fafb", marginBottom: 8 }}>Network</p>
              <p style={{ fontSize: 12, color: "#6b7280" }}>Monad Testnet (chain ID 10143)</p>
              <a
                href="https://testnet.monadscan.com"
                target="_blank"
                rel="noreferrer"
                style={{ fontSize: 12, color: "#e2f0d9", marginTop: 8, display: "inline-block", textDecoration: "none" }}
              >
                EAS Explorer ↗
              </a>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
