package split

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

// GenerateTonWalletAddress creates a new TON wallet v4r2 address for collecting funds.
// It generates a new ed25519 keypair and derives the standard wallet address
// without performing any network calls.
func GenerateTonWalletAddress(api *ton.APIClient) (string, error) {
	// Generate ed25519 keypair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("ed25519 keygen failed: %w", err)
	}

	w, err := wallet.FromPrivateKey(api, priv, wallet.V4R2)
	if err != nil {
		return "", fmt.Errorf("wallet init failed: %w", err)
	}
	return w.WalletAddress().String(), nil
}
