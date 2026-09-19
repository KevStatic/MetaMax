// useEscrow.ts — lock native MON in DeploymentEscrow for a deployment session.
import { useWriteContract } from "wagmi";
import { parseEther, sha256, toBytes } from "viem";
import { DeploymentEscrowABI, deployments } from "./contracts/typechain";

/**
 * Session IDs are strings; the escrow contract keys on bytes32. The backend
 * derives that key with SHA-256 (chain.SessionIDToJobID), so the frontend must
 * use the same hash — otherwise a user-funded escrow and the agent's later
 * release() would address two different escrow entries.
 */
export function sessionIdToBytes32(sessionId: string): `0x${string}` {
  return sha256(toBytes(sessionId));
}

export function useEscrow() {
  const { writeContractAsync, isPending } = useWriteContract();

  async function deposit(sessionId: string, providerAddress: `0x${string}`, monAmount: string): Promise<`0x${string}`> {
    const contractAddress = deployments.monadTestnet.DeploymentEscrow;
    if (!contractAddress) throw new Error("DeploymentEscrow address not configured");

    const hash = await writeContractAsync({
      address: contractAddress,
      abi: DeploymentEscrowABI.abi,
      functionName: "deposit",
      args: [sessionIdToBytes32(sessionId), providerAddress],
      value: parseEther(monAmount),
    });
    return hash;
  }

  /**
   * Start a streaming session: 20% is paid to the provider upfront and the rest
   * drips at ratePerSecond until the session is stopped.
   */
  async function startStreaming(
    sessionId: string,
    providerAddress: `0x${string}`,
    monAmount: string,
    ratePerSecondWei: bigint,
  ): Promise<`0x${string}`> {
    const contractAddress = deployments.monadTestnet.DeploymentEscrow;
    if (!contractAddress) throw new Error("DeploymentEscrow address not configured");

    return writeContractAsync({
      address: contractAddress,
      abi: DeploymentEscrowABI.abi,
      functionName: "startSession",
      args: [sessionIdToBytes32(sessionId), providerAddress, ratePerSecondWei],
      value: parseEther(monAmount),
    });
  }

  return { deposit, startStreaming, isPending };
}
