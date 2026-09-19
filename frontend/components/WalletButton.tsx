"use client";

import { ConnectButton } from "@rainbow-me/rainbowkit";
import { useAuth } from "@/lib/AuthContext";

export function WalletButton() {
  const { isConnected, isAuthenticated, isAuthenticating, authenticate } = useAuth();

  return (
    <div className="flex items-center gap-2">
      <ConnectButton chainStatus="icon" showBalance={false} accountStatus="address" />
      {isConnected && !isAuthenticated && (
        <button
          onClick={authenticate}
          disabled={isAuthenticating}
          className="flex items-center gap-2 rounded-full px-4 py-2 text-sm font-medium transition-opacity hover:opacity-90 disabled:opacity-50"
          style={{ background: "#e2f0d9", color: "#111111" }}
        >
          {isAuthenticating ? (
            <>
              <span className="inline-block h-3.5 w-3.5 rounded-full border-2 border-current border-t-transparent animate-spin" />
              Signing…
            </>
          ) : (
            "Sign in"
          )}
        </button>
      )}
    </div>
  );
}
