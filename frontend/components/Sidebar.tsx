"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAuth } from "@/lib/AuthContext";

type SidebarMode = "user" | "provider";

function IconDashboard() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <rect x="3" y="3" width="7" height="7" rx="1"/>
      <rect x="14" y="3" width="7" height="7" rx="1"/>
      <rect x="3" y="14" width="7" height="7" rx="1"/>
      <rect x="14" y="14" width="7" height="7" rx="1"/>
    </svg>
  );
}
function IconDeploy() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <circle cx="12" cy="12" r="3"/>
      <path d="M12 2v3M12 19v3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M2 12h3M19 12h3M4.22 19.78l2.12-2.12M17.66 6.34l2.12-2.12"/>
    </svg>
  );
}
function IconSessions() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <polyline points="12 8 12 12 14 14"/>
      <path d="M3.05 11a9 9 0 1 1 .5 4M3 16v-5h5"/>
    </svg>
  );
}
function IconAttestation() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
    </svg>
  );
}
function IconKey() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <circle cx="7.5" cy="15.5" r="4.5"/>
      <path d="M21 2l-9.6 9.6M15.5 7.5l2 2"/>
    </svg>
  );
}
function IconServer() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <rect x="2" y="2" width="20" height="8" rx="2" ry="2"/>
      <rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>
      <line x1="6" y1="6" x2="6.01" y2="6"/>
      <line x1="6" y1="18" x2="6.01" y2="18"/>
    </svg>
  );
}
function IconSettings() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  );
}
function IconPayment() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <rect x="1" y="4" width="22" height="16" rx="2" ry="2"/>
      <line x1="1" y1="10" x2="23" y2="10"/>
    </svg>
  );
}
function IconHistory() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <polyline points="12 8 12 12 14 14"/>
      <path d="M3.05 11a9 9 0 1 1 .5 4M3 16v-5h5"/>
    </svg>
  );
}
function IconRentals() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <rect x="2" y="3" width="20" height="14" rx="2"/>
      <path d="M8 21h8M12 17v4"/>
    </svg>
  );
}
function IconEarnings() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <line x1="12" y1="1" x2="12" y2="23"/>
      <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>
    </svg>
  );
}

const USER_NAV = [
  { href: "/",             label: "Overview",     icon: <IconDashboard /> },
  { href: "/launch",       label: "Autopilot",    icon: <IconDeploy /> },
  { href: "/deploy",       label: "Deploy",       icon: <IconDeploy /> },
  { href: "/projects",     label: "Projects",     icon: <IconServer /> },
  { href: "/sessions",     label: "Sessions",     icon: <IconSessions /> },
  { href: "/attestations", label: "Attestations", icon: <IconAttestation /> },
  { href: "/vault",        label: "Vault",        icon: <IconKey /> },
  { href: "/secrets",      label: "Secrets",      icon: <IconKey /> },
  { href: "/payments",     label: "Payments",     icon: <IconPayment /> },
  { href: "/audit",        label: "Audit Trail",  icon: <IconHistory /> },
];

const PROVIDER_NAV = [
  { href: "/provider",              label: "Dashboard",    icon: <IconDashboard /> },
  { href: "/provider/register",     label: "Register",     icon: <IconServer /> },
  { href: "/provider/settings",     label: "Settings",     icon: <IconSettings /> },
  { href: "/provider/rentals",      label: "Rentals",      icon: <IconRentals /> },
  { href: "/provider/earnings",     label: "Earnings",     icon: <IconEarnings /> },
  { href: "/provider/attestations", label: "Attestations", icon: <IconAttestation /> },
];

interface SidebarProps {
  mode?: SidebarMode;
}

export function Sidebar({ mode = "user" }: SidebarProps) {
  const pathname = usePathname();
  const { address, teamName, logout } = useAuth();
  const nav = mode === "provider" ? PROVIDER_NAV : USER_NAV;

  return (
    <aside
      style={{
        width: 220,
        minHeight: "100vh",
        background: "#0e0e0e",
        borderRight: "1px solid rgba(255,255,255,0.05)",
        display: "flex",
        flexDirection: "column",
        flexShrink: 0,
      }}
    >
      {/* Logo */}
      <div
        style={{
          padding: "24px 20px 20px",
          borderBottom: "1px solid rgba(255,255,255,0.05)",
        }}
      >
        <span
          style={{
            fontFamily: "var(--font-space-mono), monospace",
            fontSize: 15,
            fontWeight: 700,
            color: "#f9fafb",
            letterSpacing: "0.05em",
          }}
        >
          MetaMax
        </span>
        <div style={{ fontSize: 10, color: "#4b5563", marginTop: 2, letterSpacing: "0.08em" }}>
          TRUSTLESS AGENTIC CLOUD
        </div>
      </div>

      {/* Nav */}
      <nav style={{ flex: 1, padding: "12px 8px", display: "flex", flexDirection: "column", gap: 2 }}>
        {nav.map((item) => {
          const active = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
          return (
            <Link
              key={item.href}
              href={item.href}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "9px 12px",
                borderRadius: 8,
                fontSize: 13,
                fontWeight: active ? 600 : 400,
                color: active ? "#f9fafb" : "#6b7280",
                background: active ? "rgba(255,255,255,0.07)" : "transparent",
                textDecoration: "none",
                transition: "all 0.15s",
              }}
            >
              <span style={{ color: active ? "#e2f0d9" : "#4b5563" }}>{item.icon}</span>
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Footer */}
      <div
        style={{
          padding: "12px 16px 20px",
          borderTop: "1px solid rgba(255,255,255,0.05)",
          display: "flex",
          flexDirection: "column",
          gap: 8,
        }}
      >
        {address && (
          <div>
            {teamName && !teamName.startsWith("account-") && (
              <p style={{ fontSize: 12, color: "#9ca3af", fontWeight: 600 }}>{teamName}</p>
            )}
            <p
              style={{
                fontSize: 11,
                color: "#4b5563",
                fontFamily: "var(--font-space-mono), monospace",
              }}
            >
              {address.slice(0, 6)}…{address.slice(-4)}
            </p>
          </div>
        )}
        <Link
          href="/settings"
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            fontSize: 12,
            color: "#4b5563",
            textDecoration: "none",
          }}
        >
          <IconSettings />
          Settings
        </Link>
        {address && (
          <button
            onClick={logout}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              fontSize: 12,
              color: "#4b5563",
              background: "transparent",
              border: "none",
              cursor: "pointer",
              padding: 0,
            }}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
              <polyline points="16 17 21 12 16 7"/>
              <line x1="21" y1="12" x2="9" y2="12"/>
            </svg>
            Sign out
          </button>
        )}
      </div>
    </aside>
  );
}
