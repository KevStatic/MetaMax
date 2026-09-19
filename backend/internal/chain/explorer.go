package chain

import (
	"math/big"
	"strings"
)

// Monad Testnet network constants. Everything on-chain in MetaMax targets this
// network, so keep the chain ID and explorer in one place.
const (
	MonadTestnetChainID = 10143
	MonadExplorerBase   = "https://testnet.monadscan.com"
	MonadNativeSymbol   = "MON"
)

// MonadChainID returns the chain ID used when signing every MetaMax transaction.
func MonadChainID() *big.Int { return big.NewInt(MonadTestnetChainID) }

// MonadTxURL returns the MonadScan link for a transaction hash. Every on-chain
// event surfaced to the UI carries one of these so a judge can click through.
func MonadTxURL(hash string) string {
	if hash == "" {
		return ""
	}
	return MonadExplorerBase + "/tx/" + ensure0x(hash)
}

// MonadAddressURL returns the MonadScan link for an address (wallet or contract).
func MonadAddressURL(address string) string {
	if address == "" {
		return ""
	}
	return MonadExplorerBase + "/address/" + ensure0x(address)
}

func ensure0x(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return s
	}
	return "0x" + s
}
