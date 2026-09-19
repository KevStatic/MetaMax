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
	"github.com/ethereum/go-ethereum/crypto"
)

// USDC contract on Monad Testnet (Circle's official deployment).
const USDCAddress = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"

// TransferAuth holds the EIP-3009 transferWithAuthorization parameters.
type TransferAuth struct {
	From        gethcommon.Address
	To          gethcommon.Address
	Value       *big.Int
	ValidAfter  *big.Int
	ValidBefore *big.Int
	Nonce       [32]byte
	V           uint8
	R           [32]byte
	S           [32]byte
}

var (
	usdcDomainSeparator [32]byte
	transferTypeHash    [32]byte
	usdcABI             abi.ABI
)

func init() {
	transferTypeString := "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
	transferTypeHash = [32]byte(crypto.Keccak256Hash([]byte(transferTypeString)))

	domainTypeString := "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
	domainTypeHash := crypto.Keccak256Hash([]byte(domainTypeString))

	nameHash := crypto.Keccak256Hash([]byte("USD Coin"))
	versionHash := crypto.Keccak256Hash([]byte("2"))
	chainID := big.NewInt(10143) // Monad Testnet
	verifyingContract := gethcommon.HexToAddress(USDCAddress)

	domainEncABI, _ := abi.JSON(strings.NewReader(`[{"name":"f","type":"function","inputs":[
		{"name":"domainTypeHash","type":"bytes32"},
		{"name":"nameHash","type":"bytes32"},
		{"name":"versionHash","type":"bytes32"},
		{"name":"chainId","type":"uint256"},
		{"name":"verifyingContract","type":"address"}
	],"outputs":[]}]`))
	packed, err := domainEncABI.Pack("f",
		[32]byte(domainTypeHash), [32]byte(nameHash), [32]byte(versionHash),
		chainID, verifyingContract)
	if err != nil {
		panic(fmt.Sprintf("chain: compute USDC domain separator: %v", err))
	}
	usdcDomainSeparator = [32]byte(crypto.Keccak256Hash(packed[4:]))

	const transferABIJSON = `[{
		"name": "transferWithAuthorization",
		"type": "function",
		"stateMutability": "nonpayable",
		"inputs": [
			{"name": "from",        "type": "address"},
			{"name": "to",          "type": "address"},
			{"name": "value",       "type": "uint256"},
			{"name": "validAfter",  "type": "uint256"},
			{"name": "validBefore", "type": "uint256"},
			{"name": "nonce",       "type": "bytes32"},
			{"name": "v",           "type": "uint8"},
			{"name": "r",           "type": "bytes32"},
			{"name": "s",           "type": "bytes32"}
		],
		"outputs": []
	}]`
	usdcABI, err = abi.JSON(strings.NewReader(transferABIJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: parse USDC ABI: %v", err))
	}
}

// VerifyTransferAuth verifies the EIP-712 signature on a transferWithAuthorization.
func VerifyTransferAuth(auth TransferAuth) error {
	structEncABI, _ := abi.JSON(strings.NewReader(`[{"name":"f","type":"function","inputs":[
		{"name":"typeHash",     "type":"bytes32"},
		{"name":"from",         "type":"address"},
		{"name":"to",           "type":"address"},
		{"name":"value",        "type":"uint256"},
		{"name":"validAfter",   "type":"uint256"},
		{"name":"validBefore",  "type":"uint256"},
		{"name":"nonce",        "type":"bytes32"}
	],"outputs":[]}]`))
	packed, err := structEncABI.Pack("f",
		transferTypeHash, auth.From, auth.To,
		auth.Value, auth.ValidAfter, auth.ValidBefore, auth.Nonce)
	if err != nil {
		return fmt.Errorf("pack struct: %w", err)
	}
	structHash := crypto.Keccak256Hash(packed[4:])
	digest := crypto.Keccak256Hash(
		[]byte("\x19\x01"),
		usdcDomainSeparator[:],
		structHash[:],
	)

	sig := make([]byte, 65)
	copy(sig[0:32], auth.R[:])
	copy(sig[32:64], auth.S[:])
	sig[64] = auth.V
	if sig[64] >= 27 {
		sig[64] -= 27
	}

	pub, err := crypto.SigToPub(digest[:], sig)
	if err != nil {
		return fmt.Errorf("recover pub key: %w", err)
	}
	recovered := crypto.PubkeyToAddress(*pub)
	if recovered != auth.From {
		return fmt.Errorf("signer mismatch: got %s want %s", recovered.Hex(), auth.From.Hex())
	}
	return nil
}

// ExecuteTransferAuth submits a transferWithAuthorization transaction on-chain.
func ExecuteTransferAuth(ctx context.Context, rpcURL, privateKeyHex string, auth TransferAuth) (string, error) {
	client := newRPCClient(rpcURL)

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	fromAddr := crypto.PubkeyToAddress(privKey.PublicKey)
	chainID := big.NewInt(10143) // Monad Testnet

	nonceResult, err := client.call(ctx, "eth_getTransactionCount", fromAddr.Hex(), "latest")
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}
	var nonceHex string
	json.Unmarshal(nonceResult, &nonceHex)
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	gasPriceResult, _ := client.call(ctx, "eth_gasPrice")
	var gasPriceHex string
	json.Unmarshal(gasPriceResult, &gasPriceHex)
	gasPrice, _ := new(big.Int).SetString(strings.TrimPrefix(gasPriceHex, "0x"), 16)

	calldata, err := usdcABI.Pack("transferWithAuthorization",
		auth.From, auth.To, auth.Value, auth.ValidAfter, auth.ValidBefore,
		auth.Nonce, auth.V, auth.R, auth.S)
	if err != nil {
		return "", fmt.Errorf("pack calldata: %w", err)
	}

	to := gethcommon.HexToAddress(USDCAddress)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce.Uint64(),
		To:       &to,
		Value:    big.NewInt(0),
		Gas:      100_000,
		GasPrice: gasPrice,
		Data:     calldata,
	})

	signer := types.NewEIP155Signer(chainID)
	signedTx, err := types.SignTx(tx, signer, privKey)
	if err != nil {
		return "", fmt.Errorf("sign tx: %w", err)
	}
	rawTxBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return "", err
	}

	txResult, err := client.call(ctx, "eth_sendRawTransaction", "0x"+hex.EncodeToString(rawTxBytes))
	if err != nil {
		return "", fmt.Errorf("send raw transaction: %w", err)
	}
	var txHash string
	json.Unmarshal(txResult, &txHash)
	return txHash, nil
}
