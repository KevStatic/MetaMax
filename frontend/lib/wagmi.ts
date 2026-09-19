import { getDefaultConfig } from "@rainbow-me/rainbowkit";
import { monadTestnet } from "./monad";

const projectId =
  process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID || "00000000000000000000000000000000";

export const wagmiConfig = getDefaultConfig({
  appName: "MetaMax — Trustless Agentic Cloud",
  projectId,
  chains: [monadTestnet],
  ssr: true,
});
