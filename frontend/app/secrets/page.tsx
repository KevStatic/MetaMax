"use client";

import { useEffect, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";

const ACCENT = "#e2f0d9";

type Secret = { id: number; name: string; created_at: string };

export default function SecretsPage() {
  const [secrets, setSecrets] = useState<Secret[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState("");
  const [newValue, setNewValue] = useState("");
  const [saving, setSaving] = useState(false);

  function loadSecrets() {
    apiFetch("/secrets")
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => setSecrets(Array.isArray(data) ? data : []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }

  useEffect(() => { loadSecrets(); }, []);

  async function handleAdd() {
    if (!newName.trim() || !newValue.trim()) return;
    setSaving(true);
    try {
      const res = await apiFetch("/secrets", {
        method: "POST",
        body: JSON.stringify({ name: newName.trim().toUpperCase().replace(/\s+/g, "_"), value: newValue.trim() }),
      });
      if (res.ok) { setNewName(""); setNewValue(""); setAdding(false); loadSecrets(); }
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    await apiFetch(`/secrets/${id}`, { method: "DELETE" });
    setSecrets((prev) => prev.filter((s) => s.id !== id));
  }

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", alignItems: "flex-start", gap: 16, marginBottom: 24 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 300, letterSpacing: -0.5, color: "#f9fafb" }}>Secrets</p>
              <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>Encrypted key-value secrets injected into your containers</p>
            </div>
            <button
              onClick={() => setAdding(true)}
              style={{ display: "flex", alignItems: "center", gap: 8, borderRadius: 8, height: 40, padding: "0 16px", fontSize: 13, fontWeight: 900, background: ACCENT, color: "#111111", border: "none", cursor: "pointer" }}
            >
              + Add Secret
            </button>
          </header>

          {adding && (
            <div style={{ marginBottom: 24, borderRadius: 12, padding: 20, display: "flex", flexDirection: "column", gap: 16, background: "#1a1a1a", border: `1px solid rgba(226,240,217,0.25)` }}>
              <p style={{ fontSize: 13, fontWeight: 700, color: ACCENT }}>New Secret</p>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
                <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  <label style={{ fontSize: 11, fontWeight: 600, color: "#9ca3af" }}>Key name</label>
                  <input
                    type="text" placeholder="DB_PASSWORD" value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    style={{ fontSize: 13, padding: "10px 12px", borderRadius: 8, outline: "none", fontFamily: "monospace", background: "#111111", border: "1px solid rgba(255,255,255,0.07)", color: "#e5e7eb" }}
                    onFocus={(e) => (e.currentTarget.style.borderColor = ACCENT)}
                    onBlur={(e) => (e.currentTarget.style.borderColor = "rgba(255,255,255,0.07)")}
                  />
                </div>
                <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  <label style={{ fontSize: 11, fontWeight: 600, color: "#9ca3af" }}>Value</label>
                  <input
                    type="password" placeholder="secret value" value={newValue}
                    onChange={(e) => setNewValue(e.target.value)}
                    style={{ fontSize: 13, padding: "10px 12px", borderRadius: 8, outline: "none", fontFamily: "monospace", background: "#111111", border: "1px solid rgba(255,255,255,0.07)", color: "#e5e7eb" }}
                    onFocus={(e) => (e.currentTarget.style.borderColor = ACCENT)}
                    onBlur={(e) => (e.currentTarget.style.borderColor = "rgba(255,255,255,0.07)")}
                  />
                </div>
              </div>
              <div style={{ display: "flex", gap: 12 }}>
                <button onClick={handleAdd} disabled={saving} style={{ padding: "8px 16px", borderRadius: 8, fontSize: 13, fontWeight: 700, border: "none", cursor: saving ? "default" : "pointer", background: ACCENT, color: "#111111", opacity: saving ? 0.5 : 1 }}>
                  {saving ? "Saving…" : "Save Secret"}
                </button>
                <button onClick={() => { setAdding(false); setNewName(""); setNewValue(""); }} style={{ padding: "8px 16px", borderRadius: 8, fontSize: 13, fontWeight: 600, border: "none", cursor: "pointer", background: "#2a2a2d", color: "#9ca3af" }}>
                  Cancel
                </button>
              </div>
            </div>
          )}

          <div style={{ borderRadius: 12, overflow: "hidden", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.07)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 140px 80px", gap: 16, padding: "12px 20px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em", color: "#4b5563", borderBottom: "1px solid rgba(255,255,255,0.06)", background: "#111111" }}>
              <span>Key</span><span>Created</span><span></span>
            </div>

            {loading && <div style={{ padding: "32px 0", textAlign: "center", color: "#4b5563" }}>Loading…</div>}
            {!loading && secrets.length === 0 && <p style={{ padding: "32px 20px", textAlign: "center", fontSize: 13, color: "#4b5563" }}>No secrets yet.</p>}

            {secrets.map((s) => (
              <div
                key={s.id}
                style={{ display: "grid", gridTemplateColumns: "1fr 140px 80px", gap: 16, padding: "14px 20px", alignItems: "center", borderBottom: "1px solid rgba(44,44,46,0.6)" }}
                onMouseEnter={(e) => (e.currentTarget.style.background = "rgba(255,255,255,0.02)")}
                onMouseLeave={(e) => (e.currentTarget.style.background = "transparent")}
              >
                <span style={{ fontSize: 14, fontFamily: "monospace", fontWeight: 700, color: "#f3f4f6" }}>{s.name}</span>
                <span style={{ fontSize: 11, fontFamily: "monospace", color: "#4b5563" }}>{s.created_at ? new Date(s.created_at).toLocaleDateString() : "—"}</span>
                <button onClick={() => handleDelete(s.id)} style={{ fontSize: 11, padding: "4px 8px", borderRadius: 4, fontWeight: 600, border: "none", cursor: "pointer", color: "#ef4444", background: "rgba(239,68,68,0.1)", justifySelf: "end" }}>Delete</button>
              </div>
            ))}
          </div>

          <p style={{ marginTop: 16, fontSize: 12, color: "#4b5563" }}>
            Secrets are LUKS2-encrypted at rest and injected as environment variables inside the secure enclave.
          </p>
        </div>
      </main>
    </div>
  );
}
