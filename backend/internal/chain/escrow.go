package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

var escrowABI abi.ABI

func init() {
	const abiJSON = `[
		{
			"name": "deposit",
			"type": "function",
			"stateMutability": "payable",
			"inputs": [
				{"name": "sessionId", "type": "bytes32"},
				{"name": "provider",  "type": "address"}
			],
			"outputs": []
		},
		{
			"name": "release",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [{"name": "sessionId", "type": "bytes32"}],
			"outputs": []
		},
		{
			"name": "startSession",
			"type": "function",
			"stateMutability": "payable",
			"inputs": [
				{"name": "sessionId",     "type": "bytes32"},
				{"name": "provider",      "type": "address"},
				{"name": "ratePerSecond", "type": "uint256"}
			],
			"outputs": []
		},
		{
			"name": "releasePayment",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [{"name": "sessionId", "type": "bytes32"}],
			"outputs": []
		},
		{
			"name": "stopSession",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [{"name": "sessionId", "type": "bytes32"}],
			"outputs": []
		},
		{
			"name": "dispute",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [{"name": "sessionId", "type": "bytes32"}],
			"outputs": []
		},
		{
			"name": "resolveDispute",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [
				{"name": "sessionId",  "type": "bytes32"},
				{"name": "toProvider", "type": "bool"}
			],
			"outputs": []
		},
		{
			"name": "slashProvider",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [
				{"name": "sessionId", "type": "bytes32"},
				{"name": "evidence",  "type": "bytes32"}
			],
			"outputs": []
		}
	]`
	var err error
	escrowABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid escrow ABI: %v", err))
	}
}

// DepositEscrow calls deposit(sessionId, provider) on the DeploymentEscrow
// contract, locking depositWei of native MON for the session. The agent wallet
// must hold enough MON. Returns the transaction hash so the caller can render
// a MonadScan link.
func DepositEscrow(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
	provider gethcommon.Address,
	depositWei *big.Int,
) (string, error) {
	calldata, err := escrowABI.Pack("deposit", sessionID, provider)
	if err != nil {
		return "", fmt.Errorf("pack deposit: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, depositWei, calldata, 180_000)
}

// StartStreamingSession calls startSession(sessionId, provider, ratePerSecond).
// 20% of the deposit is paid to the provider immediately as an upfront; the
// remainder streams at ratePerSecond and is settled by ReleaseStreamingPayment.
func StartStreamingSession(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
	provider gethcommon.Address,
	ratePerSecond, depositWei *big.Int,
) (string, error) {
	calldata, err := escrowABI.Pack("startSession", sessionID, provider, ratePerSecond)
	if err != nil {
		return "", fmt.Errorf("pack startSession: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, depositWei, calldata, 220_000)
}

// ReleaseStreamingPayment settles the amount accrued so far on a streaming
// session. Callable by anyone, so the agent can settle on the user's behalf.
func ReleaseStreamingPayment(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
) (string, error) {
	calldata, err := escrowABI.Pack("releasePayment", sessionID)
	if err != nil {
		return "", fmt.Errorf("pack releasePayment: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, big.NewInt(0), calldata, 180_000)
}

// ReleaseEscrow calls release(sessionId) on the DeploymentEscrow contract,
// paying out a simple (non-streaming) escrow. The agent wallet must be the
// contract's releaseAuthority or owner.
func ReleaseEscrow(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
) (string, error) {
	calldata, err := escrowABI.Pack("release", sessionID)
	if err != nil {
		return "", fmt.Errorf("pack release: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, big.NewInt(0), calldata, 150_000)
}

// DisputeEscrow freezes a pending escrow while a dispute is resolved off-chain.
// Only the release authority or owner may call it.
func DisputeEscrow(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
) (string, error) {
	calldata, err := escrowABI.Pack("dispute", sessionID)
	if err != nil {
		return "", fmt.Errorf("pack dispute: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, big.NewInt(0), calldata, 120_000)
}

// ResolveDispute settles a frozen escrow: toProvider=true pays the provider,
// false refunds the user. Owner only.
func ResolveDispute(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
	toProvider bool,
) (string, error) {
	calldata, err := escrowABI.Pack("resolveDispute", sessionID, toProvider)
	if err != nil {
		return "", fmt.Errorf("pack resolveDispute: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, big.NewInt(0), calldata, 180_000)
}

// SlashStreamingProvider stops a streaming session, refunds the user and slashes
// the provider's registry stake. evidence is a keccak256 commitment to the
// off-chain evidence bundle (for MetaMax, the session's Merkle root).
func SlashStreamingProvider(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
	evidence [32]byte,
) (string, error) {
	calldata, err := escrowABI.Pack("slashProvider", sessionID, evidence)
	if err != nil {
		return "", fmt.Errorf("pack slashProvider: %w", err)
	}
	return sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, big.NewInt(0), calldata, 260_000)
}

// sendEscrowTx signs and sends a transaction to the escrow contract.
func sendEscrowTx(ctx context.Context, rpcURL, privateKeyHex, toAddress string, value *big.Int, calldata []byte, gasLimit uint64) (string, error) {
	client := newRPCClient(rpcURL)

	privKeyHex := strings.TrimPrefix(privateKeyHex, "0x")
	privKey, err := gethcrypto.HexToECDSA(privKeyHex)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	fromAddr := gethcrypto.PubkeyToAddress(privKey.PublicKey)

	nonceResult, err := client.call(ctx, "eth_getTransactionCount", fromAddr.Hex(), "pending")
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}
	var nonceHex string
	if err := json.Unmarshal(nonceResult, &nonceHex); err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	gasPriceResult, err := client.call(ctx, "eth_gasPrice")
	if err != nil {
		return "", fmt.Errorf("get gas price: %w", err)
	}
	var gasPriceHex string
	if err := json.Unmarshal(gasPriceResult, &gasPriceHex); err != nil {
		return "", fmt.Errorf("decode gas price: %w", err)
	}
	gasPrice, _ := new(big.Int).SetString(strings.TrimPrefix(gasPriceHex, "0x"), 16)

	toAddr := gethcommon.HexToAddress(toAddress)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce.Uint64(),
		To:       &toAddr,
		Value:    value,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     calldata,
	})
	signer := types.NewEIP155Signer(MonadChainID())
	signed, err := types.SignTx(tx, signer, privKey)
	if err != nil {
		return "", fmt.Errorf("sign tx: %w", err)
	}
	buf, err := signed.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshal tx: %w", err)
	}
	result, err := client.call(ctx, "eth_sendRawTransaction", "0x"+hex.EncodeToString(buf))
	if err != nil {
		return "", fmt.Errorf("eth_sendRawTransaction: %w", err)
	}
	var txHash string
	_ = json.Unmarshal(result, &txHash)
	return txHash, nil
}
