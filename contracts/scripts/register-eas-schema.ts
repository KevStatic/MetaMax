/**
 * register-eas-schema.ts
 *
 * Registers the MetaMax attestation schema on EAS (Monad Testnet).
 *
 * Schema fields:
 *   bytes32 teamId              — keccak256 of the team identifier
 *   bytes32 actionMerkleRoot    — Merkle root of all agent action hashes
 *   bytes32 containerStateHash  — Hash of final container state
 *   string  sessionId           — Human-readable session identifier
 *   string  providerAddr        — Winning provider wallet address (as string)
 *
 * Run:
 *   npm run eas:register
 */

import { ethers, network } from "hardhat";
import * as fs from "fs";
import * as path from "path";

const EAS_SCHEMA_REGISTRY: Record<string, string> = {
  monadTestnet: "0x0a7E2Ff54e76B8E6659aedc9103FB21c038050D0",
  hardhat:     "0x0000000000000000000000000000000000000000",
};

const SCHEMA_REGISTRY_ABI = [
  "function register(string calldata schema, address resolver, bool revocable) external returns (bytes32)",
  "event Registered(bytes32 indexed uid, address indexed registerer)",
];

const COMPUT3_SCHEMA =
  "bytes32 teamId,bytes32 actionMerkleRoot,bytes32 containerStateHash,string sessionId,string providerAddr";

async function main() {
  const [deployer] = await ethers.getSigners();
  console.log("Registering EAS schema with:", deployer.address);
  console.log("Network:", network.name);

  const registryAddress = EAS_SCHEMA_REGISTRY[network.name];
  if (!registryAddress || registryAddress === "0x0000000000000000000000000000000000000000") {
    console.warn("EAS SchemaRegistry not configured for network:", network.name);
    return;
  }

  const registry = new ethers.Contract(registryAddress, SCHEMA_REGISTRY_ABI, deployer);

  console.log("\nSchema:", COMPUT3_SCHEMA);
  const tx = await registry.register(COMPUT3_SCHEMA, ethers.ZeroAddress, true);
  console.log("Transaction submitted:", tx.hash);
  const receipt = await tx.wait();

  let schemaUid = "";
  for (const log of receipt.logs) {
    if (log.topics.length >= 2) {
      schemaUid = log.topics[1];
      break;
    }
  }

  console.log("\n✅  EAS schema registered!");
  console.log("   Schema UID:", schemaUid);
  console.log("   View: https://testnet.monadscan.com/schema/view/" + schemaUid);

  const deploymentsPath = path.join(__dirname, "../deployments.json");
  let deployments: Record<string, Record<string, string>> = {};
  if (fs.existsSync(deploymentsPath)) {
    try { deployments = JSON.parse(fs.readFileSync(deploymentsPath, "utf-8")); } catch {}
  }
  if (!deployments[network.name]) deployments[network.name] = {};
  deployments[network.name].EASSchemaUID = schemaUid;
  fs.writeFileSync(deploymentsPath, JSON.stringify(deployments, null, 2));
  console.log("   Schema UID saved to deployments.json");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
