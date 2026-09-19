"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import { formatTime } from "@/lib/utils";

const BG = "#111111";
const CARD = "#1a1a1a";
const BORDER = "rgba(255,255,255,0.06)";
const ACCENT = "#e2f0d9";
const ACCENT_FG = "#1a2e1a";

type Project = {
  id: string;
  team_id: string;
  name: string;
  repo_url: string;
  branch: string;
  auto_deploy: boolean;
  last_deployed_at?: string;
  created_at: string;
};

type EnvVarMeta = {
  id: number;
  key: string;
  created_at: string;
};

type CreatedProject = Project & { webhook_secret?: string };

export default function ProjectsPage() {
  const { teamId } = useAuth();
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // Create form
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [newRepoURL, setNewRepoURL] = useState("");
  const [newBranch, setNewBranch] = useState("main");
  const [newAutoDeploy, setNewAutoDeploy] = useState(true);
  const [creating, setCreating] = useState(false);
  const [createdWebhookSecret, setCreatedWebhookSecret] = useState<{ projectId: string; secret: string } | null>(null);

  // Expanded project for env vars
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [envVars, setEnvVars] = useState<EnvVarMeta[]>([]);
  const [envLoading, setEnvLoading] = useState(false);
  const [envKeyInput, setEnvKeyInput] = useState("");
  const [envValInput, setEnvValInput] = useState("");

  // Delete confirm
  const [deleteId, setDeleteId] = useState<string | null>(null);

  function loadProjects() {
    if (!teamId) return;
    setLoading(true);
    apiFetch(`/projects`)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error("Failed"))))
      .then((data) => setProjects(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => { loadProjects(); }, [teamId]); // eslint-disable-line react-hooks/exhaustive-deps

  async function handleCreate() {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const res = await apiFetch("/projects", {
        method: "POST",
        body: JSON.stringify({
          name: newName.trim(),
          repo_url: newRepoURL.trim() || undefined,
          branch: newBranch.trim() || "main",
          auto_deploy: newAutoDeploy,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      const p: CreatedProject = await res.json();
      if (p.webhook_secret) {
        setCreatedWebhookSecret({ projectId: p.id, secret: p.webhook_secret });
      }
      setProjects((prev) => [p, ...prev]);
      setNewName(""); setNewRepoURL(""); setNewBranch("main"); setNewAutoDeploy(true);
      setShowCreate(false);
    } catch (e) {
      setError(String(e));
    } finally {
      setCreating(false);
    }
  }

  async function handleDelete(id: string) {
    await apiFetch(`/projects/${id}`, { method: "DELETE" });
    setProjects((prev) => prev.filter((p) => p.id !== id));
    setDeleteId(null);
    if (expandedId === id) setExpandedId(null);
  }

  async function expandProject(id: string) {
    if (expandedId === id) { setExpandedId(null); return; }
    setExpandedId(id);
    setEnvLoading(true);
    setEnvVars([]);
    try {
      const res = await apiFetch(`/projects/${id}/env`);
      if (res.ok) setEnvVars(await res.json() ?? []);
    } finally {
      setEnvLoading(false);
    }
  }

  async function addEnvVar(projectId: string) {
    const k = envKeyInput.trim().toUpperCase().replace(/\s+/g, "_");
    if (!k) return;
    const res = await apiFetch(`/projects/${projectId}/env`, {
      method: "POST",
      body: JSON.stringify({ key: k, value: envValInput }),
    });
    if (res.ok) {
      const created: EnvVarMeta = await res.json();
      setEnvVars((prev) => {
        const filtered = prev.filter((e) => e.key !== k);
        return [...filtered, created];
      });
      setEnvKeyInput(""); setEnvValInput("");
    }
  }

  async function deleteEnvVar(projectId: string, envId: number) {
    await apiFetch(`/projects/${projectId}/env/${envId}`, { method: "DELETE" });
    setEnvVars((prev) => prev.filter((e) => e.id !== envId));
  }

  const inputStyle: React.CSSProperties = {
    padding: "8px 12px", borderRadius: 8, border: `1px solid ${BORDER}`,
    background: BG, color: "#e5e7eb", fontSize: 13, outline: "none", width: "100%",
  };

  return (
    <div style={{ display: "flex", height: "100vh", background: BG, fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32 }}>
          <header style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 28, flexWrap: "wrap", gap: 12 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>Projects</p>
              <p style={{ fontSize: 13, color: "#6b7280", marginTop: 4 }}>Manage deployments, env vars, and CI/CD webhooks per project.</p>
            </div>
            <div style={{ display: "flex", gap: 10 }}>
              <Link href="/deploy" style={{ display: "flex", alignItems: "center", height: 40, padding: "0 16px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, textDecoration: "none" }}>+ New Deployment</Link>
              <button
                onClick={() => setShowCreate((v) => !v)}
                style={{ height: 40, padding: "0 16px", borderRadius: 8, background: "rgba(255,255,255,0.07)", color: "#e5e7eb", fontSize: 13, fontWeight: 700, border: `1px solid ${BORDER}`, cursor: "pointer" }}
              >
                + Create Project
              </button>
            </div>
          </header>

          {/* Webhook secret one-time display */}
          {createdWebhookSecret && (
            <div style={{ background: "rgba(226,240,217,0.08)", border: "1px solid rgba(226,240,217,0.3)", borderRadius: 10, padding: 20, marginBottom: 24 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
                <div>
                  <p style={{ fontSize: 13, fontWeight: 700, color: ACCENT, margin: 0 }}>Project created — save your webhook secret now!</p>
                  <p style={{ fontSize: 11, color: "#9ca3af", margin: "4px 0 12px" }}>This is shown only once and cannot be retrieved again.</p>
                </div>
                <button onClick={() => setCreatedWebhookSecret(null)} style={{ fontSize: 16, color: "#6b7280", background: "none", border: "none", cursor: "pointer" }}>×</button>
              </div>
              <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                <code style={{ flex: 1, fontSize: 12, fontFamily: "monospace", color: "#e5e7eb", background: BG, padding: "8px 12px", borderRadius: 8, border: `1px solid ${BORDER}`, wordBreak: "break-all" }}>
                  {createdWebhookSecret.secret}
                </code>
                <button
                  onClick={() => navigator.clipboard.writeText(createdWebhookSecret.secret)}
                  style={{ padding: "8px 14px", fontSize: 12, fontWeight: 600, borderRadius: 8, background: "rgba(255,255,255,0.07)", color: "#e5e7eb", border: `1px solid ${BORDER}`, cursor: "pointer", flexShrink: 0 }}
                >
                  Copy
                </button>
              </div>
              <p style={{ fontSize: 11, color: "#6b7280", marginTop: 10, marginBottom: 0 }}>
                Webhook URL: <code style={{ fontFamily: "monospace" }}>
                  {typeof window !== "undefined" ? window.location.origin : ""}/api/backend/webhooks/github/{createdWebhookSecret.projectId}
                </code>
              </p>
            </div>
          )}

          {/* Create project form */}
          {showCreate && (
            <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, padding: 24, marginBottom: 24, display: "flex", flexDirection: "column", gap: 16 }}>
              <p style={{ fontSize: 14, fontWeight: 700, color: "#f9fafb", margin: 0 }}>New Project</p>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 2fr 1fr", gap: 12 }}>
                <div>
                  <label style={{ fontSize: 11, color: "#6b7280", display: "block", marginBottom: 4 }}>Project Name *</label>
                  <input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="my-app" style={inputStyle} />
                </div>
                <div>
                  <label style={{ fontSize: 11, color: "#6b7280", display: "block", marginBottom: 4 }}>Repository URL</label>
                  <input value={newRepoURL} onChange={(e) => setNewRepoURL(e.target.value)} placeholder="https://github.com/org/repo" style={inputStyle} />
                </div>
                <div>
                  <label style={{ fontSize: 11, color: "#6b7280", display: "block", marginBottom: 4 }}>Branch</label>
                  <input value={newBranch} onChange={(e) => setNewBranch(e.target.value)} placeholder="main" style={inputStyle} />
                </div>
              </div>
              <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
                <input
                  type="checkbox"
                  checked={newAutoDeploy}
                  onChange={(e) => setNewAutoDeploy(e.target.checked)}
                  style={{ width: 16, height: 16, accentColor: ACCENT }}
                />
                <span style={{ fontSize: 13, color: "#e5e7eb" }}>Enable auto-deploy on push (CI/CD webhook)</span>
              </label>
              <div style={{ display: "flex", gap: 10, justifyContent: "flex-end" }}>
                <button onClick={() => setShowCreate(false)} style={{ padding: "8px 16px", borderRadius: 8, background: "rgba(255,255,255,0.05)", color: "#9ca3af", fontSize: 13, fontWeight: 600, border: `1px solid ${BORDER}`, cursor: "pointer" }}>Cancel</button>
                <button
                  onClick={handleCreate}
                  disabled={creating || !newName.trim()}
                  style={{ padding: "8px 20px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, border: "none", cursor: creating || !newName.trim() ? "default" : "pointer", opacity: creating || !newName.trim() ? 0.5 : 1 }}
                >
                  {creating ? "Creating…" : "Create Project"}
                </button>
              </div>
            </div>
          )}

          {error && <p style={{ color: "#ef4444", fontSize: 13, marginBottom: 16 }}>{error}</p>}

          {/* Projects list */}
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {loading && <div style={{ padding: 40, textAlign: "center", color: "#6b7280" }}>Loading…</div>}
            {!loading && projects.length === 0 && (
              <div style={{ padding: 40, textAlign: "center", color: "#4b5563", background: CARD, borderRadius: 12, border: `1px solid ${BORDER}` }}>
                No projects yet. Create one above or deploy from the{" "}
                <Link href="/deploy" style={{ color: ACCENT }}>Deploy page</Link>.
              </div>
            )}
            {projects.map((p) => (
              <div key={p.id} style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, overflow: "hidden" }}>
                {/* Project row */}
                <div style={{ padding: "16px 20px", display: "flex", alignItems: "center", gap: 16 }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 4 }}>
                      <span style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb" }}>{p.name}</span>
                      {p.auto_deploy && (
                        <span style={{ fontSize: 10, fontWeight: 700, padding: "2px 8px", borderRadius: 999, background: "rgba(34,197,94,0.12)", color: "#22c55e", border: "1px solid rgba(34,197,94,0.25)" }}>
                          CI/CD on
                        </span>
                      )}
                    </div>
                    <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
                      {p.repo_url && (
                        <span style={{ fontSize: 12, fontFamily: "monospace", color: "#6b7280", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", maxWidth: 320 }}>
                          {p.repo_url}
                        </span>
                      )}
                      <span style={{ fontSize: 12, color: "#4b5563" }}>branch: <code style={{ fontFamily: "monospace" }}>{p.branch}</code></span>
                      {p.last_deployed_at && (
                        <span style={{ fontSize: 12, color: "#4b5563" }}>last deploy: {formatTime(p.last_deployed_at)}</span>
                      )}
                    </div>
                  </div>
                  <div style={{ display: "flex", gap: 8, flexShrink: 0 }}>
                    <Link
                      href={`/deploy?project_id=${p.id}`}
                      style={{ padding: "6px 14px", fontSize: 12, fontWeight: 700, borderRadius: 8, background: ACCENT, color: ACCENT_FG, textDecoration: "none" }}
                    >
                      Deploy
                    </Link>
                    <button
                      onClick={() => expandProject(p.id)}
                      style={{ padding: "6px 14px", fontSize: 12, fontWeight: 600, borderRadius: 8, background: expandedId === p.id ? "rgba(255,255,255,0.1)" : "rgba(255,255,255,0.05)", color: "#e5e7eb", border: `1px solid ${BORDER}`, cursor: "pointer" }}
                    >
                      Env Vars {expandedId === p.id ? "↑" : "↓"}
                    </button>
                    <button
                      onClick={() => setDeleteId(p.id)}
                      style={{ padding: "6px 12px", fontSize: 12, fontWeight: 600, borderRadius: 8, background: "rgba(220,53,69,0.08)", color: "#ef4444", border: "1px solid rgba(220,53,69,0.2)", cursor: "pointer" }}
                    >
                      Delete
                    </button>
                  </div>
                </div>

                {/* Env vars panel */}
                {expandedId === p.id && (
                  <div style={{ borderTop: `1px solid ${BORDER}`, padding: "16px 20px", background: "#0f0f0f", display: "flex", flexDirection: "column", gap: 12 }}>
                    <p style={{ fontSize: 13, fontWeight: 700, color: "#9ca3af", margin: 0 }}>
                      Environment Variables — encrypted at rest with AES-256-GCM
                    </p>

                    {envLoading && <span style={{ fontSize: 12, color: "#6b7280" }}>Loading…</span>}

                    {!envLoading && envVars.length === 0 && (
                      <span style={{ fontSize: 12, color: "#4b5563" }}>No env vars set yet.</span>
                    )}

                    {!envLoading && envVars.map((ev) => (
                      <div key={ev.id} style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 12px", borderRadius: 8, background: BG, border: `1px solid ${BORDER}` }}>
                        <span style={{ fontSize: 12, fontFamily: "monospace", color: ACCENT, minWidth: 160, flexShrink: 0 }}>{ev.key}</span>
                        <span style={{ flex: 1, fontSize: 12, color: "#4b5563" }}>{"•".repeat(16)} (encrypted)</span>
                        <span style={{ fontSize: 11, color: "#4b5563", flexShrink: 0 }}>{formatTime(ev.created_at)}</span>
                        <button
                          onClick={() => deleteEnvVar(p.id, ev.id)}
                          style={{ padding: "3px 10px", fontSize: 11, borderRadius: 4, border: "none", background: "rgba(220,53,69,0.12)", color: "#dc3545", cursor: "pointer", flexShrink: 0 }}
                        >
                          Remove
                        </button>
                      </div>
                    ))}

                    {/* Add new env var */}
                    <div style={{ display: "flex", gap: 8 }}>
                      <input
                        type="text"
                        placeholder="KEY_NAME"
                        value={envKeyInput}
                        onChange={(e) => setEnvKeyInput(e.target.value.toUpperCase().replace(/\s+/g, "_"))}
                        onKeyDown={(e) => e.key === "Enter" && addEnvVar(p.id)}
                        style={{ width: 180, flexShrink: 0, padding: "7px 10px", borderRadius: 8, border: `1px solid ${BORDER}`, background: BG, color: "#e5e7eb", fontSize: 12, fontFamily: "monospace", outline: "none" }}
                      />
                      <input
                        type="password"
                        placeholder="value"
                        value={envValInput}
                        onChange={(e) => setEnvValInput(e.target.value)}
                        onKeyDown={(e) => e.key === "Enter" && addEnvVar(p.id)}
                        style={{ flex: 1, padding: "7px 10px", borderRadius: 8, border: `1px solid ${BORDER}`, background: BG, color: "#e5e7eb", fontSize: 12, outline: "none" }}
                      />
                      <button
                        onClick={() => addEnvVar(p.id)}
                        disabled={!envKeyInput.trim()}
                        style={{ padding: "7px 14px", borderRadius: 8, background: "rgba(255,255,255,0.08)", color: "#e5e7eb", fontSize: 12, fontWeight: 700, border: `1px solid ${BORDER}`, cursor: !envKeyInput.trim() ? "default" : "pointer", opacity: !envKeyInput.trim() ? 0.4 : 1, whiteSpace: "nowrap" }}
                      >
                        + Set
                      </button>
                    </div>

                    <div style={{ marginTop: 4, padding: "10px 14px", borderRadius: 8, background: "rgba(226,240,217,0.04)", border: "1px solid rgba(226,240,217,0.1)" }}>
                      <p style={{ fontSize: 11, color: "#6b7280", margin: 0 }}>
                        Webhook URL for CI/CD: <code style={{ fontFamily: "monospace", color: ACCENT }}>
                          {typeof window !== "undefined" ? window.location.origin : ""}/api/backend/webhooks/github/{p.id}
                        </code>
                      </p>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      </main>

      {/* Delete confirmation modal */}
      {deleteId && (
        <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 50 }}>
          <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, padding: 28, maxWidth: 380, width: "90%" }}>
            <p style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb", marginBottom: 8 }}>Delete project?</p>
            <p style={{ fontSize: 13, color: "#9ca3af", marginBottom: 24 }}>This will permanently delete the project and all its environment variables. Sessions are not deleted.</p>
            <div style={{ display: "flex", gap: 10, justifyContent: "flex-end" }}>
              <button onClick={() => setDeleteId(null)} style={{ padding: "8px 16px", borderRadius: 8, background: "rgba(255,255,255,0.06)", color: "#e5e7eb", fontSize: 13, fontWeight: 600, border: "none", cursor: "pointer" }}>Cancel</button>
              <button onClick={() => handleDelete(deleteId)} style={{ padding: "8px 16px", borderRadius: 8, background: "rgba(220,53,69,0.2)", color: "#ef4444", fontSize: 13, fontWeight: 700, border: "1px solid rgba(220,53,69,0.3)", cursor: "pointer" }}>Delete</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
