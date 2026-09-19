package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Provider mirrors the on-chain ProviderRegistry.Provider struct.
type Provider struct {
	Wallet        gethcommon.Address
	Endpoint      string
	PricePerHour  *big.Int
	StakedAmount  *big.Int
	SlashCount    *big.Int
	JobsCompleted *big.Int
	Active        bool
}

var providerABI abi.ABI

func init() {
	const abiJSON = `[{
		"name": "getActiveProviders",
		"type": "function",
		"stateMutability": "view",
		"inputs": [],
		"outputs": [{
			"name": "",
			"type": "tuple[]",
			"components": [
				{"name": "wallet",        "type": "address"},
				{"name": "endpoint",      "type": "string"},
				{"name": "pricePerHour",  "type": "uint256"},
				{"name": "stakedAmount",  "type": "uint256"},
				{"name": "slashCount",    "type": "uint256"},
				{"name": "jobsCompleted", "type": "uint256"},
				{"name": "active",        "type": "bool"}
			]
		}]
	}]`
	var err error
	providerABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid provider ABI: %v", err))
	}
}

// GetActiveProviders returns all active providers from the ProviderRegistry.
func GetActiveProviders(ctx context.Context, rpcURL, registryAddress string) ([]Provider, error) {
	client := newRPCClient(rpcURL)
	selector := crypto.Keccak256([]byte("getActiveProviders()"))[:4]
	calldata := "0x" + hex.EncodeToString(selector)

	result, err := client.call(ctx, "eth_call", map[string]string{
		"to":   registryAddress,
		"data": calldata,
	}, "latest")
	if err != nil {
		return nil, fmt.Errorf("eth_call getActiveProviders: %w", err)
	}

	var hexStr string
	if err := json.Unmarshal(result, &hexStr); err != nil {
		return nil, fmt.Errorf("decode eth_call result: %w", err)
	}
	hexStr = strings.TrimPrefix(hexStr, "0x")
	rawBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}

	out, err := providerABI.Methods["getActiveProviders"].Outputs.Unpack(rawBytes)
	if err != nil {
		return nil, fmt.Errorf("abi unpack providers: %w", err)
	}
	if len(out) == 0 {
		return []Provider{}, nil
	}

	raw, err := json.Marshal(out[0])
	if err != nil {
		return nil, err
	}

	type onChainProvider struct {
		Wallet        gethcommon.Address `json:"wallet"`
		Endpoint      string             `json:"endpoint"`
		PricePerHour  *big.Int           `json:"pricePerHour"`
		StakedAmount  *big.Int           `json:"stakedAmount"`
		SlashCount    *big.Int           `json:"slashCount"`
		JobsCompleted *big.Int           `json:"jobsCompleted"`
		Active        bool               `json:"active"`
	}
	var providers []onChainProvider
	if err := json.Unmarshal(raw, &providers); err != nil {
		return nil, fmt.Errorf("unmarshal providers: %w", err)
	}

	out2 := make([]Provider, len(providers))
	for i, p := range providers {
		out2[i] = Provider{
			Wallet:        p.Wallet,
			Endpoint:      p.Endpoint,
			PricePerHour:  p.PricePerHour,
			StakedAmount:  p.StakedAmount,
			SlashCount:    p.SlashCount,
			JobsCompleted: p.JobsCompleted,
			Active:        p.Active,
		}
	}
	return out2, nil
}

// recordJobABI is the minimal ABI fragment for recordJobCompleted.
var recordJobABI abi.ABI

func init() {
	const abiJSON = `[{
		"name": "recordJobCompleted",
		"type": "function",
		"stateMutability": "nonpayable",
		"inputs": [{"name": "providerWallet", "type": "address"}],
		"outputs": []
	}]`
	var err error
	recordJobABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid recordJobCompleted ABI: %v", err))
	}
}

// RecordJobCompleted sends a recordJobCompleted(providerWallet) transaction to the registry,
// incrementing the provider's on-chain reputation counter. The caller (agent wallet)
// must be the contract owner or slashAuthority. Returns the transaction hash.
func RecordJobCompleted(ctx context.Context, rpcURL, privateKeyHex, registryAddress string, providerWallet gethcommon.Address) (string, error) {
	calldata, err := recordJobABI.Pack("recordJobCompleted", providerWallet)
	if err != nil {
		return "", fmt.Errorf("pack recordJobCompleted: %w", err)
	}
	return sendRegistryTx(ctx, rpcURL, privateKeyHex, registryAddress, calldata, 100_000)
}

// SlashProvider slashes 50% of a provider's registry stake. evidence is a
// keccak256 commitment to the off-chain evidence (MetaMax uses the session's
// Merkle root). Caller must be the registry's slashAuthority or owner.
func SlashProvider(ctx context.Context, rpcURL, privateKeyHex, registryAddress string, providerWallet gethcommon.Address, evidence [32]byte) (string, error) {
	calldata, err := registrySlashABI.Pack("slash", providerWallet, evidence)
	if err != nil {
		return "", fmt.Errorf("pack slash: %w", err)
	}
	return sendRegistryTx(ctx, rpcURL, privateKeyHex, registryAddress, calldata, 160_000)
}

// registrySlashABI is the minimal ABI fragment for slash().
var registrySlashABI abi.ABI

func init() {
	const abiJSON = `[{
		"name": "slash",
		"type": "function",
		"stateMutability": "nonpayable",
		"inputs": [
			{"name": "providerWallet", "type": "address"},
			{"name": "evidence",       "type": "bytes32"}
		],
		"outputs": []
	}]`
	var err error
	registrySlashABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid slash ABI: %v", err))
	}
}

// sendRegistryTx signs and submits a ProviderRegistry transaction on Monad.
func sendRegistryTx(ctx context.Context, rpcURL, privateKeyHex, registryAddress string, calldata []byte, gasLimit uint64) (string, error) {
	client := newRPCClient(rpcURL)

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	fromAddr := crypto.PubkeyToAddress(privKey.PublicKey)

	nonceResult, err := client.call(ctx, "eth_getTransactionCount", fromAddr.Hex(), "pending")
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}
	var nonceHex string
	if err := json.Unmarshal(nonceResult, &nonceHex); err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	gasPrice := big.NewInt(2_000_000_000) // 2 gwei fallback
	if gpResult, err := client.call(ctx, "eth_gasPrice"); err == nil {
		var gasPriceHex string
		if json.Unmarshal(gpResult, &gasPriceHex) == nil {
			if gp, ok := new(big.Int).SetString(strings.TrimPrefix(gasPriceHex, "0x"), 16); ok && gp.Sign() > 0 {
				gasPrice = gp
			}
		}
	}

	toAddr := gethcommon.HexToAddress(registryAddress)
	tx := types.NewTransaction(nonce.Uint64(), toAddr, big.NewInt(0), gasLimit, gasPrice, calldata)
	signed, err := types.SignTx(tx, types.NewEIP155Signer(MonadChainID()), privKey)
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

// SelectCheapestProvider queries the registry and returns the cheapest active provider.
// Falls back to a local MetaMax node sentinel if none are available.
func SelectCheapestProvider(ctx context.Context, rpcURL, registryAddress string) (*Provider, error) {
	providers, err := GetActiveProviders(ctx, rpcURL, registryAddress)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		// Fallback: MetaMax local node
		return &Provider{
			Endpoint:     "http://localhost:8081",
			PricePerHour: big.NewInt(0),
			Active:       true,
		}, nil
	}
	sort.Slice(providers, func(i, j int) bool {
		cmp := providers[i].PricePerHour.Cmp(providers[j].PricePerHour)
		if cmp != 0 {
			return cmp < 0
		}
		return providers[i].JobsCompleted.Cmp(providers[j].JobsCompleted) > 0
	})
	return &providers[0], nil
}
