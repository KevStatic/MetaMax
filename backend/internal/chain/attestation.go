package chain

// Native Monad execution-attestation transaction support. This deliberately
// stores only commitment hashes, never logs or workload contents.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var executionAttestationABI abi.ABI

func init() {
	var err error
	executionAttestationABI, err = abi.JSON(strings.NewReader(`[{"type":"function","name":"attest","stateMutability":"nonpayable","inputs":[{"name":"sessionId","type":"string"},{"name":"teamId","type":"bytes32"},{"name":"merkleRoot","type":"bytes32"}],"outputs":[]}]`))
	if err != nil { panic(fmt.Sprintf("chain: invalid ExecutionAttestation ABI: %v", err)) }
}

// SubmitMonadAttestation commits a completed execution's Merkle root to the
// app-owned ExecutionAttestation contract on Monad Testnet (chain ID 10143).
func SubmitMonadAttestation(ctx context.Context, rpcURL, privateKeyHex, contractAddress, sessionID, teamID string, merkleRoot [32]byte) (*AttestationResult, error) {
	if !common.IsHexAddress(contractAddress) { return nil, fmt.Errorf("invalid execution attestation address") }
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil { return nil, fmt.Errorf("parse private key: %w", err) }
	client := newRPCClient(rpcURL)
	from := crypto.PubkeyToAddress(key.PublicKey)
	result, err := client.call(ctx, "eth_getTransactionCount", from.Hex(), "pending")
	if err != nil { return nil, fmt.Errorf("get nonce: %w", err) }
	var nonceHex string
	if err := json.Unmarshal(result, &nonceHex); err != nil { return nil, err }
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)
	result, err = client.call(ctx, "eth_gasPrice")
	if err != nil { return nil, fmt.Errorf("get gas price: %w", err) }
	var gasHex string
	if err := json.Unmarshal(result, &gasHex); err != nil { return nil, err }
	gas, _ := new(big.Int).SetString(strings.TrimPrefix(gasHex, "0x"), 16)
	var teamHash [32]byte
	copy(teamHash[:], crypto.Keccak256([]byte(teamID)))
	calldata, err := executionAttestationABI.Pack("attest", sessionID, teamHash, merkleRoot)
	if err != nil { return nil, fmt.Errorf("pack attestation: %w", err) }
	to := common.HexToAddress(contractAddress)
	tx := types.NewTx(&types.LegacyTx{Nonce: nonce.Uint64(), To: &to, Gas: 180_000, GasPrice: gas, Value: big.NewInt(0), Data: calldata})
	signed, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(10143)), key)
	if err != nil { return nil, fmt.Errorf("sign attestation: %w", err) }
	raw, err := signed.MarshalBinary()
	if err != nil { return nil, err }
	result, err = client.call(ctx, "eth_sendRawTransaction", "0x"+hex.EncodeToString(raw))
	if err != nil { return nil, fmt.Errorf("send attestation: %w", err) }
	var hash string
	if err := json.Unmarshal(result, &hash); err != nil { return nil, err }
	return &AttestationResult{TxHash: hash}, nil
}
