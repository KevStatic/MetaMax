package chain

import (
	"context"
	stdsha256 "crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// Pre-computed event topic hashes.
var (
	topicJobAwarded         = gethcrypto.Keccak256Hash([]byte("JobAwarded(bytes32,address,uint256,uint256)"))
	topicJobAwardedFallback = gethcrypto.Keccak256Hash([]byte("JobAwardedToFallback(bytes32,address,uint256)"))
	topicJobPosted          = gethcrypto.Keccak256Hash([]byte("JobPosted(bytes32,address,uint256,uint256,uint256,uint256,uint256,uint256)"))
)

// JobAwardedResult holds the winner selected by closeAuction().
type JobAwardedResult struct {
	Winner        gethcommon.Address
	PricePerHour  *big.Int
	RatePerSecond *big.Int
	IsFallback    bool
}

// JobPostedEvent holds data from a JobPosted on-chain event.
type JobPostedEvent struct {
	JobID           [32]byte
	User            gethcommon.Address
	MaxPricePerHour *big.Int
	RamMb           *big.Int
	CpuCores        *big.Int
	DurationSeconds *big.Int
	BidDeadline     *big.Int // unix timestamp
	BlockNumber     uint64
}

var auctionABI abi.ABI

func init() {
	const abiJSON = `[
		{"name":"postJob","type":"function","stateMutability":"payable","inputs":[{"name":"jobId","type":"bytes32"},{"name":"maxPricePerHour","type":"uint256"},{"name":"ramMb","type":"uint256"},{"name":"cpuCores","type":"uint256"},{"name":"durationSeconds","type":"uint256"}],"outputs":[]},
		{"name":"submitBid","type":"function","stateMutability":"nonpayable","inputs":[{"name":"jobId","type":"bytes32"},{"name":"pricePerHour","type":"uint256"}],"outputs":[]},
		{"name":"closeAuction","type":"function","stateMutability":"nonpayable","inputs":[{"name":"jobId","type":"bytes32"}],"outputs":[]},
		{"name":"getBids","type":"function","stateMutability":"view","inputs":[{"name":"jobId","type":"bytes32"}],"outputs":[{"name":"","type":"tuple[]","components":[{"name":"provider","type":"address"},{"name":"pricePerHour","type":"uint256"},{"name":"submittedAt","type":"uint256"}]}]},
		{"name":"isBidWindowOpen","type":"function","stateMutability":"view","inputs":[{"name":"jobId","type":"bytes32"}],"outputs":[{"name":"","type":"bool"}]}
	]`
	var err error
	auctionABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid auction ABI: %v", err))
	}
}

// SessionIDToJobID converts a session ID to a deterministic bytes32 job ID.
func SessionIDToJobID(sessionID string) [32]byte {
	return stdsha256.Sum256([]byte(sessionID))
}

// CurrentBlock returns the latest block number on the chain.
func CurrentBlock(ctx context.Context, rpcURL string) (uint64, error) {
	client := newRPCClient(rpcURL)
	result, err := client.call(ctx, "eth_blockNumber")
	if err != nil {
		return 0, fmt.Errorf("eth_blockNumber: %w", err)
	}
	var hexStr string
	if err := json.Unmarshal(result, &hexStr); err != nil {
		return 0, fmt.Errorf("decode block number: %w", err)
	}
	n, _ := new(big.Int).SetString(strings.TrimPrefix(hexStr, "0x"), 16)
	return n.Uint64(), nil
}

// PostJob sends a postJob() transaction to the JobAuction contract.
// depositWei is the ETH to lock (forwarded to DeploymentEscrow on auction close).
func PostJob(
	ctx context.Context,
	rpcURL, privateKeyHex, auctionAddress string,
	jobID [32]byte,
	maxPricePerHour, ramMb, cpuCores, durationSeconds, depositWei *big.Int,
) (string, error) {
	calldata, err := auctionABI.Pack("postJob", jobID, maxPricePerHour, ramMb, cpuCores, durationSeconds)
	if err != nil {
		return "", fmt.Errorf("pack postJob: %w", err)
	}
	return sendAuctionTx(ctx, rpcURL, privateKeyHex, auctionAddress, depositWei, calldata)
}

// SubmitBid sends a submitBid() transaction from the provider's wallet.
func SubmitBid(
	ctx context.Context,
	rpcURL, privateKeyHex, auctionAddress string,
	jobID [32]byte,
	pricePerHour *big.Int,
) (string, error) {
	calldata, err := auctionABI.Pack("submitBid", jobID, pricePerHour)
	if err != nil {
		return "", fmt.Errorf("pack submitBid: %w", err)
	}
	return sendAuctionTx(ctx, rpcURL, privateKeyHex, auctionAddress, big.NewInt(0), calldata)
}

// CloseAuction sends a closeAuction() transaction.
// Call this after the 30-second bid window has closed.
func CloseAuction(
	ctx context.Context,
	rpcURL, privateKeyHex, auctionAddress string,
	jobID [32]byte,
) (string, error) {
	calldata, err := auctionABI.Pack("closeAuction", jobID)
	if err != nil {
		return "", fmt.Errorf("pack closeAuction: %w", err)
	}
	return sendAuctionTx(ctx, rpcURL, privateKeyHex, auctionAddress, big.NewInt(0), calldata)
}

// WatchJobAwarded polls eth_getLogs until a JobAwarded or JobAwardedToFallback event
// is found for the given jobID, or until the context expires.
func WatchJobAwarded(ctx context.Context, rpcURL, auctionAddress string, jobID [32]byte) (*JobAwardedResult, error) {
	client := newRPCClient(rpcURL)

	// Start polling from a few blocks back to catch recent events.
	blk, err := CurrentBlock(ctx, rpcURL)
	if err != nil {
		blk = 0
	}
	fromBlock := uint64(0)
	if blk > 5 {
		fromBlock = blk - 5
	}

	jobIDHex := "0x" + hex.EncodeToString(jobID[:])
	filter := map[string]any{
		"address": auctionAddress,
		"topics": []any{
			[]string{
				"0x" + hex.EncodeToString(topicJobAwarded[:]),
				"0x" + hex.EncodeToString(topicJobAwardedFallback[:]),
			},
			jobIDHex,
		},
		"fromBlock": fmt.Sprintf("0x%x", fromBlock),
		"toBlock":   "latest",
	}

	for {
		logsResult, err := client.call(ctx, "eth_getLogs", filter)
		if err == nil {
			var logs []ethLog
			if json.Unmarshal(logsResult, &logs) == nil {
				for _, l := range logs {
					if res, err := decodeJobAwardedLog(l); err == nil {
						return res, nil
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("WatchJobAwarded: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// PollJobPostedEvents returns all JobPosted events from fromBlock onwards.
// Also returns the next fromBlock to use in subsequent calls.
func PollJobPostedEvents(ctx context.Context, rpcURL, auctionAddress string, fromBlock uint64) ([]JobPostedEvent, uint64, error) {
	client := newRPCClient(rpcURL)

	filter := map[string]any{
		"address":   auctionAddress,
		"topics":    []any{"0x" + hex.EncodeToString(topicJobPosted[:])},
		"fromBlock": fmt.Sprintf("0x%x", fromBlock),
		"toBlock":   "latest",
	}

	logsResult, err := client.call(ctx, "eth_getLogs", filter)
	if err != nil {
		return nil, fromBlock, fmt.Errorf("eth_getLogs: %w", err)
	}

	var logs []ethLog
	if err := json.Unmarshal(logsResult, &logs); err != nil {
		return nil, fromBlock, fmt.Errorf("unmarshal logs: %w", err)
	}

	var events []JobPostedEvent
	maxBlock := fromBlock
	for _, l := range logs {
		blkBig, _ := new(big.Int).SetString(strings.TrimPrefix(l.BlockNumber, "0x"), 16)
		if blkBig != nil && blkBig.Uint64() > maxBlock {
			maxBlock = blkBig.Uint64()
		}
		ev, err := decodeJobPostedLog(l)
		if err != nil {
			continue
		}
		events = append(events, ev)
	}
	// advance past the last seen block to avoid re-processing
	if maxBlock > fromBlock {
		maxBlock++
	}
	return events, maxBlock, nil
}

// --- internal helpers ---

// ethLog is the JSON structure returned by eth_getLogs.
type ethLog struct {
	BlockNumber string   `json:"blockNumber"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
}

func decodeJobAwardedLog(l ethLog) (*JobAwardedResult, error) {
	if len(l.Topics) < 3 {
		return nil, fmt.Errorf("not enough topics: %d", len(l.Topics))
	}
	isFallback := strings.EqualFold(l.Topics[0], "0x"+hex.EncodeToString(topicJobAwardedFallback[:]))
	winner := gethcommon.HexToAddress(l.Topics[2])

	dataBytes, err := hex.DecodeString(strings.TrimPrefix(l.Data, "0x"))
	if err != nil {
		dataBytes = nil
	}

	readSlot := func(slot int) *big.Int {
		start := slot * 32
		if len(dataBytes) < start+32 {
			return big.NewInt(0)
		}
		return new(big.Int).SetBytes(dataBytes[start : start+32])
	}

	var pricePerHour, ratePerSecond *big.Int
	if isFallback {
		// data: abi.encode(ratePerSecond)
		ratePerSecond = readSlot(0)
		pricePerHour = new(big.Int).Mul(ratePerSecond, big.NewInt(3600))
	} else {
		// data: abi.encode(pricePerHour, ratePerSecond)
		pricePerHour = readSlot(0)
		ratePerSecond = readSlot(1)
	}

	return &JobAwardedResult{
		Winner:        winner,
		PricePerHour:  pricePerHour,
		RatePerSecond: ratePerSecond,
		IsFallback:    isFallback,
	}, nil
}

func decodeJobPostedLog(l ethLog) (JobPostedEvent, error) {
	if len(l.Topics) < 2 {
		return JobPostedEvent{}, fmt.Errorf("not enough topics: %d", len(l.Topics))
	}

	var jobID [32]byte
	jobIDBytes, err := hex.DecodeString(strings.TrimPrefix(l.Topics[1], "0x"))
	if err != nil || len(jobIDBytes) != 32 {
		return JobPostedEvent{}, fmt.Errorf("decode jobId: %w", err)
	}
	copy(jobID[:], jobIDBytes)

	var user gethcommon.Address
	if len(l.Topics) >= 3 {
		user = gethcommon.HexToAddress(l.Topics[2])
	}

	dataBytes, err := hex.DecodeString(strings.TrimPrefix(l.Data, "0x"))
	if err != nil {
		return JobPostedEvent{}, fmt.Errorf("decode data: %w", err)
	}

	readSlot := func(slot int) *big.Int {
		start := slot * 32
		if len(dataBytes) < start+32 {
			return big.NewInt(0)
		}
		return new(big.Int).SetBytes(dataBytes[start : start+32])
	}

	blkBig, _ := new(big.Int).SetString(strings.TrimPrefix(l.BlockNumber, "0x"), 16)
	return JobPostedEvent{
		JobID:           jobID,
		User:            user,
		MaxPricePerHour: readSlot(0), // maxPricePerHour
		RamMb:           readSlot(1), // ramMb
		CpuCores:        readSlot(2), // cpuCores
		DurationSeconds: readSlot(3), // durationSeconds
		// slot 4 = deposit (not stored in event struct)
		BidDeadline: readSlot(5), // bidDeadline (unix timestamp)
		BlockNumber: blkBig.Uint64(),
	}, nil
}

// sendAuctionTx signs and broadcasts a transaction to the auction contract.
func sendAuctionTx(ctx context.Context, rpcURL, privateKeyHex, toAddress string, value *big.Int, calldata []byte) (string, error) {
	client := newRPCClient(rpcURL)

	privKeyHex := strings.TrimPrefix(privateKeyHex, "0x")
	privKey, err := gethcrypto.HexToECDSA(privKeyHex)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	fromAddr := gethcrypto.PubkeyToAddress(privKey.PublicKey)
	chainID := big.NewInt(10143) // Monad Testnet

	nonceResult, err := client.call(ctx, "eth_getTransactionCount", fromAddr.Hex(), "latest")
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}
	var nonceHex string
	if err := json.Unmarshal(nonceResult, &nonceHex); err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	toAddr := gethcommon.HexToAddress(toAddress)
	gasPrice := big.NewInt(2_000_000_000) // 2 gwei
	gasLimit := uint64(300_000)

	tx := types.NewTransaction(nonce.Uint64(), toAddr, value, gasLimit, gasPrice, calldata)
	signer := types.NewEIP155Signer(chainID)
	signed, err := types.SignTx(tx, signer, privKey)
	if err != nil {
		return "", fmt.Errorf("sign tx: %w", err)
	}

	buf, err := signed.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshal tx: %w", err)
	}
	rawHex := "0x" + hex.EncodeToString(buf)

	result, err := client.call(ctx, "eth_sendRawTransaction", rawHex)
	if err != nil {
		return "", fmt.Errorf("eth_sendRawTransaction: %w", err)
	}
	var txHash string
	_ = json.Unmarshal(result, &txHash)
	return txHash, nil
}
