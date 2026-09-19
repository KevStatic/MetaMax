/**
 * become-provider.ts
 *
 * Register as a MetaMax compute provider on-chain.
 *
 * Usage:
 *   1. Add to your .env:
 *        PROVIDER_PRIVATE_KEY          your wallet private key (must hold >= 0.015 ETH)
 *        PROVIDER_ENDPOINT             public HTTPS URL of your node API
 *        PROVIDER_PRICE_PER_HOUR_WEI   price in wei per container-hour (default: 0.001 ETH)
 *        PROVIDER_REGISTRY_ADDRESS     ProviderRegistry contract address
 *
 *   2. Run:
 *        npm run become-provider
 */

import * as dotenv from "dotenv";
import * as path from "path";
import * as fs from "fs";
import { ethers } from "hardhat";

dotenv.config({ path: path.resolve(__dirname, "../../.env") });

const MIN_STAKE = ethers.parseEther("0.01");

async function main() {
  const privateKey = process.env.PROVIDER_PRIVATE_KEY || process.env.DEPLOYER_PRIVATE_KEY;
  if (!privateKey) {
    console.error("\n❌  Missing PROVIDER_PRIVATE_KEY in .env\n");
    process.exit(1);
  }

  const endpoint = process.env.PROVIDER_ENDPOINT;
  if (!endpoint || !endpoint.startsWith("http")) {
    console.error("\n❌  Missing or invalid PROVIDER_ENDPOINT in .env\n");
    process.exit(1);
  }

  const priceWei = BigInt(process.env.PROVIDER_PRICE_PER_HOUR_WEI ?? "1000000000000000");

  const registryAddress = process.env.PROVIDER_REGISTRY_ADDRESS;
  if (!registryAddress) {
    console.error("\n❌  Missing PROVIDER_REGISTRY_ADDRESS in .env\n");
    process.exit(1);
  }

  const provider = ethers.provider;
  const wallet = new ethers.Wallet(privateKey, provider);

  const artifactPath = path.resolve(
    __dirname,
    "../artifacts/contracts/ProviderRegistry.sol/ProviderRegistry.json"
  );
  if (!fs.existsSync(artifactPath)) {
    console.error("\n❌  Artifacts not found. Run: npm run compile\n");
    process.exit(1);
  }

  const { abi } = JSON.parse(fs.readFileSync(artifactPath, "utf8"));
  const registry = new ethers.Contract(registryAddress, abi, wallet);

  console.log("\n╔══════════════════════════════════════════════════════╗");
  console.log("║        MetaMax — Compute Provider Registration       ║");
  console.log("╚══════════════════════════════════════════════════════╝\n");
  console.log(`  Wallet   : ${wallet.address}`);
  console.log(`  Endpoint : ${endpoint}`);
  console.log(`  Price    : ${ethers.formatEther(priceWei)} ETH / container-hour`);
  console.log(`  Contract : ${registryAddress}`);

  const balance = await provider.getBalance(wallet.address);
  console.log(`  Balance  : ${ethers.formatEther(balance)} ETH`);

  const existing = await registry.providers(wallet.address);

  if (existing.wallet !== ethers.ZeroAddress) {
    console.log("\n  ℹ  Already registered.\n");
    console.log(`  Status   : ${existing.active ? "✅  Active" : "⚠️   Inactive"}`);
    console.log(`  Staked   : ${ethers.formatEther(existing.stakedAmount)} ETH`);
    console.log(`  Endpoint : ${existing.endpoint}`);
    console.log(`  Price    : ${ethers.formatEther(existing.pricePerHour)} ETH/hr`);
    console.log(`  Jobs     : ${existing.jobsCompleted}`);
    console.log(`  Slashes  : ${existing.slashCount}`);

    if (!existing.active && existing.stakedAmount < MIN_STAKE) {
      console.log("\n  Stake dropped below 0.01 ETH. Topping up...");
      const topUp = MIN_STAKE - existing.stakedAmount + ethers.parseEther("0.005");
      const tx = await registry.stake({ value: topUp });
      console.log(`  📤  Stake tx: ${tx.hash}`);
      const receipt = await tx.wait(1);
      console.log(`  ✅  Confirmed in block ${receipt.blockNumber}. Provider is active again.\n`);
    }
    return;
  }

  if (balance < MIN_STAKE + ethers.parseEther("0.005")) {
    console.error(`\n❌  Insufficient balance. Need at least 0.015 ETH.\n`);
    process.exit(1);
  }

  console.log("\n  Registering...");
  const stakeAmount = MIN_STAKE + ethers.parseEther("0.005");
  const tx = await registry.register(endpoint, priceWei, { value: stakeAmount });
  console.log(`  📤  Registration tx: ${tx.hash}`);
  console.log(`      https://testnet.monadscan.com/tx/${tx.hash}`);
  const receipt = await tx.wait(1);
  console.log(`  ✅  Confirmed in block ${receipt.blockNumber}.\n`);
  console.log(`  Staked  : ${ethers.formatEther(stakeAmount)} ETH`);
  console.log(`  You are now an active MetaMax provider.\n`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
