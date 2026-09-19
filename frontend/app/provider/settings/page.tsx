"use client";

import { useEffect, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { WalletButton } from "@/components/WalletButton";
import { useAccount, useReadContract, useWriteContract, useWaitForTransactionReceipt } from "wagmi";
import { formatEther, parseEther } from "viem";
import { monadTestnet } from "@/lib/monad";
import { ProviderRegistryABI, deployments } from "@/lib/contracts/typechain";

const ACCENT = "#e2f0d9";
const REGISTRY_ADDRESS = deployments.monadTestnet.ProviderRegistry;

type SettingsForm = {
  endpoint: string;
  pricePerHour: string;
};

export default function ProviderSettingsPage() {
  const { isConnected } = useAccount();
  const [form, setForm] = useState<SettingsForm>({ endpoint: "", pricePerHour: "0.08" });
  const [error, setError] = useState("");

  const { writeContract, data: txHash, isPending } = useWriteContract();
  const { isSuccess, isLoading: isConfirming } = useWaitForTransactionReceipt({ hash: txHash, chainId: monadTestnet.id });

  function handleChange(key: keyof SettingsForm, value: string) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function handleSubmit() {
    if (!isConnected) return;
    setError("");
    try {
      const pricePerHourWei = parseEther(form.pricePerHour);
      writeContract({
        address: REGISTRY_ADDRESS,
        abi: ProviderRegistryABI.abi,
        functionName: "update",
        args: [form.endpoint, pricePerHourWei],
        chainId: monadTestnet.id,
      });
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Transaction failed");
    }
  }

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="provider" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32, maxWidth: 560 }}>
          <header style={{ marginBottom: 24 }}>
            <p style={{ fontSize: 28, fontWeight: 900, letterSpacing: -0.5, color: "#f9fafb" }}>Provider Settings</p>
            <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>
              Update your endpoint URL and pricing on-chain
            </p>
          </header>

          {!isConnected ? (
            <div style={{ borderRadius: 12, padding: 24, display: "flex", flexDirection: "column", alignItems: "center", gap: 16, background: "#1a1a1a", border: `1px solid ${ACCENT}` }}>
              <p style={{ fontSize: 13, color: "#9ca3af" }}>Connect your wallet to update provider settings</p>
              <WalletButton />
            </div>
          ) : isSuccess ? (
            <div style={{ borderRadius: 12, padding: 24, display: "flex", flexDirection: "column", gap: 12, background: "rgba(34,197,94,0.06)", border: "1px solid rgba(34,197,94,0.25)" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#22c55e" strokeWidth="2.5"><polyline points="20 6 9 17 4 12"/></svg>
                <p style={{ fontSize: 13, fontWeight: 700, color: "#22c55e" }}>Provider settings updated on-chain!</p>
              </div>
              <p style={{ fontSize: 11, fontFamily: "monospace", wordBreak: "break-all", color: "#6b7280" }}>Tx: {txHash}</p>
              <a href={`https://testnet.monadscan.com/tx/${txHash}`} target="_blank" rel="noreferrer" style={{ fontSize: 12, color: ACCENT }}>View on MonadScan →</a>
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
              {error && (
                <div style={{ borderRadius: 8, padding: 12, fontSize: 12, fontFamily: "monospace", background: "rgba(239,68,68,0.08)", border: "1px solid rgba(239,68,68,0.25)", color: "#ef4444" }}>{error}</div>
              )}

              {/* Endpoint */}
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <label style={{ fontSize: 13, fontWeight: 600, color: "#9ca3af" }}>New Endpoint URL</label>
                <input
                  type="text"
                  placeholder="https://my-provider.example.com"
                  value={form.endpoint}
                  onChange={(e) => handleChange("endpoint", e.target.value)}
                  style={{ fontSize: 13, padding: "10px 12px", borderRadius: 8, outline: "none", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.08)", color: "#e5e7eb" }}
                  onFocus={(e) => (e.currentTarget.style.borderColor = ACCENT)}
                  onBlur={(e) => (e.currentTarget.style.borderColor = "rgba(255,255,255,0.08)")}
                />
              </div>

              {/* Price */}
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <label style={{ fontSize: 13, fontWeight: 600, color: "#9ca3af" }}>New Price / hour (MON)</label>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <input
                    type="number" step="0.01" min="0.01"
                    value={form.pricePerHour}
                    onChange={(e) => handleChange("pricePerHour", e.target.value)}
                    style={{ width: 160, fontSize: 13, padding: "10px 12px", borderRadius: 8, outline: "none", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.08)", color: "#e5e7eb" }}
                    onFocus={(e) => (e.currentTarget.style.borderColor = ACCENT)}
                    onBlur={(e) => (e.currentTarget.style.borderColor = "rgba(255,255,255,0.08)")}
                  />
                  <span style={{ fontSize: 13, fontFamily: "monospace", color: "#6b7280" }}>MON/hr</span>
                </div>
              </div>

              <p style={{ fontSize: 12, color: "#6b7280" }}>Updating settings calls <code style={{ fontFamily: "monospace", color: ACCENT }}>update()</code> on the ProviderRegistry contract. No additional stake required.</p>

              <button
                onClick={handleSubmit}
                disabled={!form.endpoint.trim() || isPending || isConfirming}
                style={{ height: 44, borderRadius: 8, fontSize: 13, fontWeight: 900, border: "none", cursor: !form.endpoint.trim() || isPending || isConfirming ? "default" : "pointer", background: ACCENT, color: "#111111", opacity: !form.endpoint.trim() || isPending || isConfirming ? 0.3 : 1, display: "flex", alignItems: "center", justifyContent: "center", gap: 8 }}
              >
                {(isPending || isConfirming) && (
                  <span className="animate-spin" style={{ display: "inline-block", width: 14, height: 14, borderRadius: "50%", border: "2px solid currentColor", borderTopColor: "transparent" }} />
                )}
                {isPending ? "Confirm in wallet…" : isConfirming ? "Confirming…" : "Update Provider Settings"}
              </button>
            </div>
          )}

          {isConnected && <StakeAndAvailability />}
        </div>
      </main>
    </div>
  );
}

/**
 * Stake top-ups, partial withdrawals and the availability switch — the three
 * levers a provider pulls between jobs. Stake is collateral: drop below
 * MIN_STAKE and the registry deactivates you automatically, and a proved
 * failure lets the dispute resolver slash half of it.
 */
function StakeAndAvailability() {
  const { address } = useAccount();
  const [amount, setAmount] = useState("0.01");
  const [err, setErr] = useState("");

  const { writeContract, data: txHash, isPending } = useWriteContract();
  const { isSuccess, isLoading: isConfirming } = useWaitForTransactionReceipt({ hash: txHash, chainId: monadTestnet.id });

  const { data: record, refetch } = useReadContract({
    address: REGISTRY_ADDRESS,
    abi: ProviderRegistryABI.abi,
    functionName: "providers",
    args: address ? [address] : undefined,
    chainId: monadTestnet.id,
    query: { enabled: Boolean(address && REGISTRY_ADDRESS) },
  });

  useEffect(() => {
    if (isSuccess) refetch();
  }, [isSuccess, refetch]);

  // providers() returns the full struct: [wallet, endpoint, price, staked, slashes, jobs, active]
  const staked = record ? (record as readonly unknown[])[3] as bigint : undefined;
  const slashes = record ? (record as readonly unknown[])[4] as bigint : undefined;
  const jobs = record ? (record as readonly unknown[])[5] as bigint : undefined;
  const active = record ? (record as readonly unknown[])[6] as boolean : undefined;

  function send(functionName: "stake" | "unstake" | "deactivate" | "reactivate") {
    setErr("");
    try {
      writeContract({
        address: REGISTRY_ADDRESS,
        abi: ProviderRegistryABI.abi,
        functionName,
        args: functionName === "unstake" ? [parseEther(amount)] : [],
        value: functionName === "stake" ? parseEther(amount) : undefined,
        chainId: monadTestnet.id,
      } as never);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Transaction failed");
    }
  }

  const busy = isPending || isConfirming;
  const btn = {
    height: 38, padding: "0 14px", borderRadius: 8, fontSize: 12, fontWeight: 700,
    border: "1px solid rgba(255,255,255,0.08)", background: "rgba(255,255,255,0.06)",
    color: "#e5e7eb", cursor: busy ? "default" : "pointer", opacity: busy ? 0.5 : 1,
  } as const;

  return (
    <section style={{ marginTop: 32, paddingTop: 24, borderTop: "1px solid rgba(255,255,255,0.06)", display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <p style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb" }}>Stake &amp; availability</p>
        <p style={{ fontSize: 12, color: "#6b7280", marginTop: 4 }}>
          Your stake is the collateral that makes your bids credible. Falling below the minimum
          deactivates you; a proved failure can slash half of it.
        </p>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
        {[
          { label: "Staked", value: staked !== undefined ? `${formatEther(staked)} MON` : "—" },
          { label: "Jobs done", value: jobs !== undefined ? jobs.toString() : "—" },
          { label: "Slashes", value: slashes !== undefined ? slashes.toString() : "—" },
          { label: "Status", value: active === undefined ? "—" : active ? "Accepting jobs" : "Paused" },
        ].map((s) => (
          <div key={s.label} style={{ background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.06)", borderRadius: 10, padding: 12 }}>
            <p style={{ fontSize: 11, color: "#6b7280" }}>{s.label}</p>
            <p style={{ fontSize: 13, fontWeight: 700, color: s.label === "Status" ? (active ? "#22c55e" : "#eab308") : "#f9fafb", marginTop: 4 }}>{s.value}</p>
          </div>
        ))}
      </div>

      {err && (
        <div style={{ borderRadius: 8, padding: 10, fontSize: 12, fontFamily: "monospace", background: "rgba(239,68,68,0.08)", border: "1px solid rgba(239,68,68,0.25)", color: "#ef4444" }}>{err}</div>
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <input
          type="number" step="0.01" min="0.01"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          style={{ width: 120, fontSize: 13, padding: "9px 12px", borderRadius: 8, outline: "none", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.08)", color: "#e5e7eb" }}
        />
        <span style={{ fontSize: 12, color: "#6b7280" }}>MON</span>
        <button onClick={() => send("stake")} disabled={busy} style={{ ...btn, background: ACCENT, color: "#111111", border: "none" }}>Add stake</button>
        <button onClick={() => send("unstake")} disabled={busy} style={btn}>Withdraw</button>
        <div style={{ marginLeft: "auto", display: "flex", gap: 8 }}>
          {active ? (
            <button onClick={() => send("deactivate")} disabled={busy} style={btn}>Pause jobs</button>
          ) : (
            <button onClick={() => send("reactivate")} disabled={busy} style={btn}>Resume jobs</button>
          )}
        </div>
      </div>

      {txHash && (
        <a href={`https://testnet.monadscan.com/tx/${txHash}`} target="_blank" rel="noreferrer" style={{ fontSize: 12, fontFamily: "monospace", color: ACCENT, textDecoration: "none" }}>
          {isConfirming ? "Confirming" : "Submitted"} — view on MonadScan ↗
        </a>
      )}
    </section>
  );
}
