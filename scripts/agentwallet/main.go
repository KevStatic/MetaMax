// Generates a keypair for AGENT_WALLET_PRIVATE_KEY.
//
// The agent wallet signs unattended on testnet, so it is generated standalone
// rather than exported from a personal MetaMask account: nothing here derives
// from your Secret Recovery Phrase, and a leaked .env cannot touch your real
// accounts.
//
// With -env, the key is written straight into the .env file and never printed,
// so it cannot end up in terminal scrollback, a screenshot, or a chat window.
// Only the address is shown — that value is public.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)



func main() {
	envPath := flag.String("env", "", "path to a .env file to update in place")
	varName := flag.String("var", "AGENT_WALLET_PRIVATE_KEY", "env variable to set")
	flag.Parse()

	key, err := crypto.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not generate key:", err)
		os.Exit(1)
	}
	priv := fmt.Sprintf("0x%x", crypto.FromECDSA(key))
	addr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	if *envPath == "" {
		fmt.Printf("\n  Paste into .env:\n    %s=%s\n", *varName, priv)
		fmt.Printf("\n  Fund at https://faucet.monad.xyz:\n    %s\n\n", addr)
		return
	}

	raw, err := os.ReadFile(*envPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not read env file:", err)
		os.Exit(1)
	}

	line := *varName + "=" + priv
	re := regexp.MustCompile(`(?m)^` + *varName + `=.*$`)
	out := string(raw)
	if re.MatchString(out) {
		out = re.ReplaceAllString(out, line)
	} else {
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += line + "\n"
	}
	if err := os.WriteFile(*envPath, []byte(out), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "could not write env file:", err)
		os.Exit(1)
	}

	fmt.Printf("\n  %s written to %s (not displayed).\n", *varName, *envPath)
	fmt.Printf("\n  Fund this address at https://faucet.monad.xyz:\n    %s\n\n", addr)
	fmt.Println("  Testnet only. Never send real assets to this address.")
	fmt.Println()
}
