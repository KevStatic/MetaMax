package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// EAS contract address on Monad Testnet.
const EASContractAddress = "0xC2679fBD37d54388Ce493F1DB75320D236e1815e"

// AttestationResult is returned after submitting an EAS attestation.
type AttestationResult struct {
	TxHash string `json:"tx_hash"`
}

var easAttestABI abi.ABI

func init() {
	const abiJSON = `[{
		"name": "attest",
		"type": "function",
		"stateMutability": "payable",
		"inputs": [{
			"name": "request",
			"type": "tuple",
			"components": [
				{"name": "schema", "type": "bytes32"},
				{"name": "data", "type": "tuple", "components": [
					{"name": "recipient",       "type": "address"},
					{"name": "expirationTime", "type": "uint64"},
					{"name": "revocable",       "type": "bool"},
					{"name": "refUID",          "type": "bytes32"},
					{"name": "data",            "type": "bytes"},
					{"name": "value",           "type": "uint256"}
				]}
			]
		}],
		"outputs": [{"name": "", "type": "bytes32"}]
	}]`
	var err error
	easAttestABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid EAS ABI: %v", err))
	}
}

// SubmitAttestation signs and submits an EAS attest() transaction.
// The attestation data encodes: teamId, actionMerkleRoot, sessionId.
func SubmitAttestation(ctx context.Context, rpcURL, privateKeyHex, schemaUID string,
	sessionID, teamID string, merkleRoot [32]byte) (*AttestationResult, error) {

	client := newRPCClient(rpcURL)

	privKeyHex := strings.TrimPrefix(privateKeyHex, "0x")
	privKey, err := crypto.HexToECDSA(privKeyHex)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	fromAddr := crypto.PubkeyToAddress(privKey.PublicKey)
	chainID := big.NewInt(10143) // Monad Testnet

	nonceResult, err := client.call(ctx, "eth_getTransactionCount", fromAddr.Hex(), "latest")
	if err != nil {
		return nil, fmt.Errorf("get nonce: %w", err)
	}
	var nonceHex string
	if err := json.Unmarshal(nonceResult, &nonceHex); err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	gasPriceResult, err := client.call(ctx, "eth_gasPrice")
	if err != nil {
		return nil, fmt.Errorf("get gas price: %w", err)
	}
	var gasPriceHex string
	if err := json.Unmarshal(gasPriceResult, &gasPriceHex); err != nil {
		return nil, fmt.Errorf("decode gas price: %w", err)
	}
	gasPrice, _ := new(big.Int).SetString(strings.TrimPrefix(gasPriceHex, "0x"), 16)

	// ABI-encode the inner attestation data
	var teamIDBuf [32]byte
	copy(teamIDBuf[:], crypto.Keccak256([]byte(teamID)))

	innerABI, _ := abi.JSON(strings.NewReader(`[{"name":"f","type":"function","inputs":[
		{"name":"teamId","type":"bytes32"},
		{"name":"merkleRoot","type":"bytes32"},
		{"name":"sessionId","type":"string"}
	],"outputs":[]}]`))
	innerData, err := innerABI.Pack("f", teamIDBuf, merkleRoot, sessionID)
	if err != nil {
		return nil, fmt.Errorf("pack attestation data: %w", err)
	}
	innerData = innerData[4:] // strip 4-byte selector

	schemaBytes, err := hex.DecodeString(strings.TrimPrefix(schemaUID, "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse schema UID: %w", err)
	}
	var schemaHash [32]byte
	copy(schemaHash[:], schemaBytes)

	type attestDataStruct struct {
		Recipient      gethcommon.Address
		ExpirationTime uint64
		Revocable      bool
		RefUID         [32]byte
		Data           []byte
		Value          *big.Int
	}
	type attestRequestStruct struct {
		Schema [32]byte
		Data   attestDataStruct
	}

	calldata, err := easAttestABI.Pack("attest", attestRequestStruct{
		Schema: schemaHash,
		Data: attestDataStruct{
			Recipient:      gethcommon.Address{},
			ExpirationTime: 0,
			Revocable:      false,
			RefUID:         [32]byte{},
			Data:           innerData,
			Value:          big.NewInt(0),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("pack attest calldata: %w", err)
	}

	to := gethcommon.HexToAddress(EASContractAddress)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce.Uint64(),
		To:       &to,
		Value:    big.NewInt(0),
		Gas:      300_000,
		GasPrice: gasPrice,
		Data:     calldata,
	})

	signer := types.NewEIP155Signer(chainID)
	signedTx, err := types.SignTx(tx, signer, privKey)
	if err != nil {
		return nil, fmt.Errorf("sign tx: %w", err)
	}

	rawTxBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal tx: %w", err)
	}

	txResult, err := client.call(ctx, "eth_sendRawTransaction", "0x"+hex.EncodeToString(rawTxBytes))
	if err != nil {
		return nil, fmt.Errorf("send raw transaction: %w", err)
	}
	var txHash string
	if err := json.Unmarshal(txResult, &txHash); err != nil {
		return nil, fmt.Errorf("decode tx hash: %w", err)
	}
	return &AttestationResult{TxHash: txHash}, nil
}

// WaitForAttestationUID polls for the tx receipt (up to 90s) and returns
// the attestation UID from the Attested event log.
func WaitForAttestationUID(ctx context.Context, rpcURL, txHash string) (string, error) {
	ctx2, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client := newRPCClient(rpcURL)
	for {
		select {
		case <-ctx2.Done():
			return "", fmt.Errorf("timeout waiting for tx %s", txHash)
		case <-time.After(2 * time.Second):
		}

		result, err := client.call(ctx2, "eth_getTransactionReceipt", txHash)
		if err != nil || string(result) == "null" {
			continue
		}
		type receipt struct {
			Logs []struct {
				Topics []string `json:"topics"`
				Data   string   `json:"data"`
			} `json:"logs"`
			Status string `json:"status"`
		}
		var rec receipt
		if err := json.Unmarshal(result, &rec); err != nil {
			continue
		}
		if rec.Status != "0x1" {
			return "", fmt.Errorf("tx reverted")
		}
		// Attested(bytes32 uid, ...) — UID is the first 32 bytes of the first log's data
		// topic[0] = keccak256("Attested(address,address,bytes32,bytes32)")
		for _, log := range rec.Logs {
			if len(log.Topics) > 0 {
				// The UID is embedded in topics[2] per EAS spec
				if len(log.Topics) >= 3 {
					return log.Topics[2], nil
				}
				if len(log.Data) >= 66 {
					return log.Data[:66], nil
				}
			}
		}
		return "", nil
	}
}

// IsAttestationValid calls EAS.isAttestationValid(uid).
func IsAttestationValid(ctx context.Context, rpcURL, attestationUID string) (bool, error) {
	const abiJSON = `[{
		"name": "isAttestationValid",
		"type": "function",
		"stateMutability": "view",
		"inputs": [{"name": "uid", "type": "bytes32"}],
		"outputs": [{"name": "", "type": "bool"}]
	}]`
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return false, fmt.Errorf("parse abi: %w", err)
	}
	uidBytes, err := hex.DecodeString(strings.TrimPrefix(attestationUID, "0x"))
	if err != nil {
		return false, fmt.Errorf("decode uid: %w", err)
	}
	var uid32 [32]byte
	copy(uid32[:], uidBytes)
	calldata, err := parsed.Pack("isAttestationValid", uid32)
	if err != nil {
		return false, fmt.Errorf("pack calldata: %w", err)
	}
	type ethCallParams struct {
		To   string `json:"to"`
		Data string `json:"data"`
	}
	client := newRPCClient(rpcURL)
	result, err := client.call(ctx, "eth_call", ethCallParams{
		To:   EASContractAddress,
		Data: "0x" + hex.EncodeToString(calldata),
	}, "latest")
	if err != nil {
		return false, fmt.Errorf("eth_call: %w", err)
	}
	var hexResult string
	if err := json.Unmarshal(result, &hexResult); err != nil {
		return false, fmt.Errorf("decode result: %w", err)
	}
	decoded, err := parsed.Unpack("isAttestationValid", gethcommon.FromHex(hexResult))
	if err != nil || len(decoded) == 0 {
		return false, err
	}
	valid, _ := decoded[0].(bool)
	return valid, nil
}
