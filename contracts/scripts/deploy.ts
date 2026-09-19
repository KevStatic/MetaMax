import { ethers, run, network } from "hardhat";
import * as fs from "fs";
import * as path from "path";

async function main() {
  const [deployer] = await ethers.getSigners();
  console.log("Deploying contracts with:", deployer.address);
  console.log("Network:", network.name);

  const balance = await ethers.provider.getBalance(deployer.address);
  console.log("Balance:", ethers.formatEther(balance), "MON\n");

  let nonce = await ethers.provider.getTransactionCount(deployer.address, "latest");
  console.log("Starting nonce:", nonce);

  // ── ProviderRegistry ──────────────────────────────────────────────────────
  console.log("Deploying ProviderRegistry...");
  const ProviderRegistry = await ethers.getContractFactory("ProviderRegistry");
  const registry = await ProviderRegistry.deploy(deployer.address, deployer.address, { nonce: nonce++ });
  await registry.waitForDeployment();
  const registryAddress = await registry.getAddress();
  console.log("ProviderRegistry deployed to:", registryAddress);

  // ── DeploymentEscrow ──────────────────────────────────────────────────────
  console.log("\nDeploying DeploymentEscrow...");
  const DeploymentEscrow = await ethers.getContractFactory("DeploymentEscrow");
  const escrow = await DeploymentEscrow.deploy(deployer.address, deployer.address, registryAddress, { nonce: nonce++ });
  await escrow.waitForDeployment();
  const escrowAddress = await escrow.getAddress();
  console.log("DeploymentEscrow deployed to:", escrowAddress);

  // ── JobAuction ────────────────────────────────────────────────────────────
  console.log("\nDeploying JobAuction...");
  const JobAuction = await ethers.getContractFactory("JobAuction");
  const auction = await JobAuction.deploy(deployer.address, registryAddress, escrowAddress, deployer.address, { nonce: nonce++ });
  await auction.waitForDeployment();
  const auctionAddress = await auction.getAddress();
  console.log("JobAuction deployed to:", auctionAddress);

  console.log("\nDeploying ExecutionAttestation...");
  const ExecutionAttestation = await ethers.getContractFactory("ExecutionAttestation");
  const attestation = await ExecutionAttestation.deploy(deployer.address, { nonce: nonce++ });
  await attestation.waitForDeployment();
  const attestationAddress = await attestation.getAddress();
  console.log("ExecutionAttestation deployed to:", attestationAddress);

  // ── Save addresses ────────────────────────────────────────────────────────
  const deploymentsPath = path.join(__dirname, "../deployments.json");
  let deployments: Record<string, Record<string, string>> = {};
  if (fs.existsSync(deploymentsPath)) {
    try { deployments = JSON.parse(fs.readFileSync(deploymentsPath, "utf-8")); } catch {}
  }

  deployments[network.name] = {
    ProviderRegistry: registryAddress,
    DeploymentEscrow: escrowAddress,
    JobAuction:       auctionAddress,
    ExecutionAttestation: attestationAddress,
    deployedAt:       new Date().toISOString(),
    deployer:         deployer.address,
  };

  fs.writeFileSync(deploymentsPath, JSON.stringify(deployments, null, 2));
  console.log("\nDeployment addresses saved to deployments.json");

  // ── Verify on MonadScan ────────────────────────────────────────────────────
  if (network.name !== "hardhat" && network.name !== "localhost") {
    console.log("\nWaiting 20s before verification...");
    await new Promise((r) => setTimeout(r, 20_000));

    const verifyList = [
      { name: "ProviderRegistry", address: registryAddress, args: [deployer.address, deployer.address] },
      { name: "DeploymentEscrow", address: escrowAddress,  args: [deployer.address, deployer.address, registryAddress] },
      { name: "JobAuction",       address: auctionAddress, args: [deployer.address, registryAddress, escrowAddress, deployer.address] },
      { name: "ExecutionAttestation", address: attestationAddress, args: [deployer.address] },
    ];

    for (const c of verifyList) {
      console.log(`Verifying ${c.name}...`);
      try {
        await run("verify:verify", { address: c.address, constructorArguments: c.args });
      } catch (e: any) {
        console.warn(`  ${c.name} verification failed: ${e.message}`);
      }
    }
  }

  console.log("\n✅  Deploy complete.");
  console.log(`   ProviderRegistry : ${registryAddress}`);
  console.log(`   DeploymentEscrow : ${escrowAddress}`);
  console.log(`   JobAuction       : ${auctionAddress}`);
  console.log(`   ExecutionAttestation : ${attestationAddress}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
