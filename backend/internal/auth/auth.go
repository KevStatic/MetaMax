package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/golang-jwt/jwt/v5"
)

const (
	nonceExpiry   = 10 * time.Minute
	nonceBytesLen = 16
)

type nonce struct {
	value     string
	expiresAt time.Time
}

// Service handles SIWE nonce management and JWT issuance.
type Service struct {
	jwtSecret []byte

	mu     sync.Mutex
	nonces map[string]nonce // address (lowercased) → nonce
}

// New creates a new auth service.
func New(jwtSecret string) *Service {
	return &Service{
		jwtSecret: []byte(jwtSecret),
		nonces:    make(map[string]nonce),
	}
}

// IssueNonce generates and stores a fresh nonce for the given wallet address.
func (s *Service) IssueNonce(address string) string {
	b := make([]byte, nonceBytesLen)
	_, _ = rand.Read(b)
	n := nonce{
		value:     hex.EncodeToString(b),
		expiresAt: time.Now().Add(nonceExpiry),
	}
	s.mu.Lock()
	s.nonces[strings.ToLower(address)] = n
	s.mu.Unlock()
	return n.value
}

// ConsumeNonce verifies that nonce matches the one issued for address and
// removes it (one-time use).
func (s *Service) ConsumeNonce(address, nonceValue string) error {
	key := strings.ToLower(address)
	s.mu.Lock()
	n, ok := s.nonces[key]
	if ok {
		delete(s.nonces, key)
	}
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("no nonce issued for address %s", address)
	}
	if time.Now().After(n.expiresAt) {
		return fmt.Errorf("nonce expired")
	}
	if n.value != nonceValue {
		return fmt.Errorf("nonce mismatch")
	}
	return nil
}

// IssueJWT creates a signed JWT for the given wallet address and team ID.
func (s *Service) IssueJWT(address, teamID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":     strings.ToLower(address),
		"team_id": teamID,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateJWT parses and validates a JWT, returning the address and team ID.
func (s *Service) ValidateJWT(tokenString string) (address, teamID string, err error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", fmt.Errorf("invalid token claims")
	}
	address, _ = claims["sub"].(string)
	teamID, _ = claims["team_id"].(string)
	if address == "" {
		return "", "", fmt.Errorf("missing sub claim")
	}
	return address, teamID, nil
}

// GCNonces removes expired nonces. Call periodically (e.g., every minute).
func (s *Service) GCNonces() {
	now := time.Now()
	s.mu.Lock()
	for addr, n := range s.nonces {
		if now.After(n.expiresAt) {
			delete(s.nonces, addr)
		}
	}
	s.mu.Unlock()
}

// VerifyPersonalSign checks that `signature` is a valid EIP-191 personal_sign
// of `nonce` produced by the private key behind `address`.
//
// The frontend signs the bare nonce with wagmi's signMessage, which prefixes
// "\x19Ethereum Signed Message:\n<len>" before hashing — accounts.TextHash
// reproduces exactly that digest, so the recovered address must equal the one
// the caller claims. Without this check any caller could assert any address.
func VerifyPersonalSign(address, nonce, signature string) error {
	if !ethcommon.IsHexAddress(address) {
		return fmt.Errorf("invalid address")
	}
	sig, err := hexutil.Decode(signature)
	if err != nil {
		return fmt.Errorf("signature must be 0x-prefixed hex")
	}
	if len(sig) != crypto.SignatureLength {
		return fmt.Errorf("signature must be %d bytes, got %d", crypto.SignatureLength, len(sig))
	}

	// Wallets return v as 27/28; SigToPub expects a 0/1 recovery id.
	v := sig[crypto.RecoveryIDOffset]
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return fmt.Errorf("invalid signature recovery id")
	}
	normalized := make([]byte, crypto.SignatureLength)
	copy(normalized, sig)
	normalized[crypto.RecoveryIDOffset] = v

	pub, err := crypto.SigToPub(accounts.TextHash([]byte(nonce)), normalized)
	if err != nil {
		return fmt.Errorf("could not recover signer: %w", err)
	}
	if recovered := crypto.PubkeyToAddress(*pub); !strings.EqualFold(recovered.Hex(), address) {
		return fmt.Errorf("signature does not match address")
	}
	return nil
}
