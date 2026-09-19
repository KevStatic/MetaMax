"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth, STORAGE } from "@/lib/AuthContext";

export default function OnboardingPage() {
  const { address } = useAuth();
  const router = useRouter();
  const [teamName, setTeamName] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!teamName.trim()) return;
    setSaving(true);
    setError("");
    try {
      const r = await apiFetch("/account", {
        method: "POST",
        body: JSON.stringify({ name: teamName.trim() }),
      });
      if (!r.ok) throw new Error(await r.text());
      const account = await r.json();
      if (account.team_name) localStorage.setItem(STORAGE.TEAM_NAME, account.team_name);
      if (account.team_id) localStorage.setItem(STORAGE.TEAM_ID, account.team_id);
      router.push("/");
    } catch (e) {
      setError(String(e));
      setSaving(false);
    }
  }

  return (
    <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <div style={{ width: "100%", maxWidth: 400, padding: "0 24px" }}>
        <div style={{ textAlign: "center", marginBottom: 32 }}>
          <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", letterSpacing: -1 }}>MetaMax</p>
          <p style={{ fontSize: 22, fontWeight: 700, color: "#f3f4f6", marginTop: 24, marginBottom: 4 }}>Almost there</p>
          <p style={{ fontSize: 14, color: "#6b7280" }}>Give your workspace a name to get started</p>
        </div>

        {error && (
          <div style={{ marginBottom: 16, padding: 12, borderRadius: 8, background: "rgba(239,68,68,0.1)", border: "1px solid rgba(239,68,68,0.3)", color: "#fca5a5", fontSize: 13 }}>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <input
            type="text"
            placeholder="My Team"
            value={teamName}
            onChange={(e) => setTeamName(e.target.value)}
            autoFocus
            required
            style={{ padding: "12px 14px", borderRadius: 8, border: "1px solid rgba(255,255,255,0.08)", background: "#1a1a1a", color: "#e5e7eb", fontSize: 14, outline: "none" }}
          />
          {address && (
            <p style={{ fontSize: 11, color: "#4b5563", fontFamily: "monospace" }}>Wallet: {address}</p>
          )}
          <button
            type="submit"
            disabled={saving || !teamName.trim()}
            style={{ padding: "12px 24px", borderRadius: 8, background: "#e2f0d9", color: "#111111", fontSize: 14, fontWeight: 700, border: "none", cursor: saving || !teamName.trim() ? "default" : "pointer", opacity: saving || !teamName.trim() ? 0.5 : 1, marginTop: 4 }}
          >
            {saving ? "Saving…" : "Get Started →"}
          </button>
        </form>
      </div>
    </div>
  );
}
