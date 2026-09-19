import { HardhatUserConfig } from "hardhat/config";
import "@nomicfoundation/hardhat-toolbox";
import * as dotenv from "dotenv";

dotenv.config({ path: "../.env" });

const PRIVATE_KEY = process.env.DEPLOYER_PRIVATE_KEY;
const MONAD_TESTNET_RPC = process.env.MONAD_TESTNET_RPC_URL ?? "https://testnet-rpc.monad.xyz";
// MonadScan's verification API is optional. Leave this blank if the explorer
// does not expose an API key for the current testnet environment.
const MONADSCAN_API_KEY = process.env.MONADSCAN_API_KEY ?? "";

const config: HardhatUserConfig = {
  solidity: {
    version: "0.8.24",
    settings: {
      optimizer: { enabled: true, runs: 200 },
    },
  },
  networks: {
    hardhat: {},
    monadTestnet: {
      url: MONAD_TESTNET_RPC,
      accounts: PRIVATE_KEY ? [PRIVATE_KEY] : [],
      chainId: 10143,
    },
  },
  etherscan: {
    apiKey: MONADSCAN_API_KEY,
    customChains: [
      {
        network: "monadTestnet",
        chainId: 10143,
        urls: {
          apiURL: "https://api.monadscan.com/api",
          browserURL: "https://testnet.monadscan.com",
        },
      },
    ],
  },
  paths: {
    sources: "./contracts",
    tests:   "./test",
    cache:   "./cache",
    artifacts: "./artifacts",
  },
};

export default config;
