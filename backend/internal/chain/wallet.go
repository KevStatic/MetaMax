package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// The autonomous agent wallet. MetaMax's deployment agent holds its own Monad
// key: it posts auctions, funds escrow for the provider it picked, attests the
// execution proof and settles payment without a human signing anything. These
// helpers expose that wallet so the UI can show who is spending and how much
// MON is left.

// WalletInfo describes an agent or provider wallet on Monad Testnet.
type WalletInfo struct {
	Address     string `json:"address"`
	BalanceWei  string `json:"balance_wei"`
	BalanceMON  string `json:"balance_mon"`
	ChainID     int64  `json:"chain_id"`
	ExplorerURL string `json:"explorer_url"`
	Funded      bool   `json:"funded"`
}

// AddressFromPrivateKey derives the public address for a hex private key.
func AddressFromPrivateKey(privateKeyHex string) (gethcommon.Address, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return gethcommon.Address{}, fmt.Errorf("parse private key: %w", err)
	}
	return crypto.PubkeyToAddress(key.PublicKey), nil
}

// GetBalance returns the native MON balance of an address in wei.
func GetBalance(ctx context.Context, rpcURL, address string) (*big.Int, error) {
	client := newRPCClient(rpcURL)
	result, err := client.call(ctx, "eth_getBalance", address, "latest")
	if err != nil {
		return nil, fmt.Errorf("eth_getBalance: %w", err)
	}
	var hexBal string
	if err := json.Unmarshal(result, &hexBal); err != nil {
		return nil, fmt.Errorf("decode balance: %w", err)
	}
	bal, ok := new(big.Int).SetString(strings.TrimPrefix(hexBal, "0x"), 16)
	if !ok {
		return nil, fmt.Errorf("invalid balance %q", hexBal)
	}
	return bal, nil
}

// AgentWallet resolves the agent wallet's address and live MON balance.
// A zero-value WalletInfo with Funded=false is returned when no key is set,
// which lets the UI explain that the node is running in read-only mode.
func AgentWallet(ctx context.Context, rpcURL, privateKeyHex string) (*WalletInfo, error) {
	if privateKeyHex == "" {
		return &WalletInfo{ChainID: MonadTestnetChainID}, nil
	}
	addr, err := AddressFromPrivateKey(privateKeyHex)
	if err != nil {
		return nil, err
	}
	info := &WalletInfo{
		Address:     addr.Hex(),
		ChainID:     MonadTestnetChainID,
		ExplorerURL: MonadAddressURL(addr.Hex()),
		BalanceWei:  "0",
		BalanceMON:  "0",
	}
	bal, err := GetBalance(ctx, rpcURL, addr.Hex())
	if err != nil {
		// Address is still useful even if the RPC is unreachable.
		return info, nil
	}
	info.BalanceWei = bal.String()
	info.BalanceMON = FormatMON(bal)
	info.Funded = bal.Sign() > 0
	return info, nil
}

// FormatMON renders a wei amount as a human-readable MON string (6 decimals).
func FormatMON(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	f := new(big.Float).SetInt(wei)
	f.Quo(f, big.NewFloat(1e18))
	return f.Text('f', 6)
}

// MONToWei converts a decimal MON amount (e.g. "0.01") to wei.
func MONToWei(mon string) (*big.Int, error) {
	f, ok := new(big.Float).SetString(strings.TrimSpace(mon))
	if !ok {
		return nil, fmt.Errorf("invalid MON amount %q", mon)
	}
	f.Mul(f, big.NewFloat(1e18))
	wei, _ := f.Int(nil)
	return wei, nil
}
