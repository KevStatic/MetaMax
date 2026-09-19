"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ConnectButton } from "@rainbow-me/rainbowkit";
import { useAuth } from "@/lib/AuthContext";

export default function SignInPage() {
  const { isAuthenticated, isConnected, isAuthenticating, hydrated, authenticate } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (hydrated && isAuthenticated) {
      router.replace("/");
    }
  }, [hydrated, isAuthenticated, router]);

  if (!hydrated) return null;
  if (isAuthenticated) return null;

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 40,
        padding: "0 16px",
        background: "#111111",
        fontFamily: "Inter, var(--font-inter), sans-serif",
      }}
    >
      {/* Logo */}
      <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 16 }}>
        <h1 style={{ fontSize: 48, fontWeight: 300, letterSpacing: "-0.02em", color: "#fff" }}>MetaMax</h1>
        <p style={{ fontSize: 16, color: "#9ca3af", textAlign: "center", maxWidth: 280 }}>
          Trustless agentic cloud — every deployment is cryptographically proven.
        </p>
      </div>

      {/* Auth card */}
      <div
        style={{
          width: "100%",
          maxWidth: 380,
          borderRadius: 24,
          padding: 40,
          display: "flex",
          flexDirection: "column",
          gap: 28,
          background: "#1a1a1a",
          border: "1px solid rgba(255,255,255,0.07)",
        }}
      >
        <div>
          <h2 style={{ fontSize: 24, fontWeight: 600, color: "#fff", marginBottom: 8 }}>Sign in</h2>
          <p style={{ fontSize: 14, color: "#9ca3af" }}>
            Connect your wallet and sign the authentication message to access your dashboard.
          </p>
        </div>

        {/* Step 1 — connect wallet */}
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <span style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.1em", color: "#6b7280" }}>
            Step 1 — Connect wallet
          </span>
          <ConnectButton chainStatus="icon" showBalance={false} accountStatus="address" />
        </div>

        {/* Step 2 — sign message */}
        {isConnected && !isAuthenticated && (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <span style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.1em", color: "#6b7280" }}>
              Step 2 — Sign in
            </span>
            {isAuthenticating ? (
              <button
                disabled
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  gap: 8,
                  width: "100%",
                  borderRadius: 12,
                  height: 48,
                  fontSize: 16,
                  fontWeight: 500,
                  background: "rgba(226,240,217,0.12)",
                  color: "rgba(255,255,255,0.4)",
                  border: "none",
                  cursor: "default",
                }}
              >
                <span
                  style={{
                    display: "inline-block",
                    width: 16,
                    height: 16,
                    borderRadius: "50%",
                    border: "2px solid currentColor",
                    borderTopColor: "transparent",
                  }}
                  className="animate-spin"
                />
                Waiting for signature…
              </button>
            ) : (
              <button
                onClick={authenticate}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  gap: 8,
                  width: "100%",
                  borderRadius: 12,
                  height: 48,
                  fontSize: 16,
                  fontWeight: 700,
                  background: "#e2f0d9",
                  color: "#111111",
                  border: "none",
                  cursor: "pointer",
                }}
              >
                Sign in with wallet
              </button>
            )}
            <p style={{ fontSize: 12, color: "#6b7280", textAlign: "center" }}>
              A signature request will appear in your wallet. No gas is used.
            </p>
          </div>
        )}
      </div>

      <p style={{ fontSize: 12, color: "#4b5563", textAlign: "center", maxWidth: 280 }}>
        By signing in you agree to MetaMax&apos;s terms. Your key, your cloud.
      </p>
    </div>
  );
}
