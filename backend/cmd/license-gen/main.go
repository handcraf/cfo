// license-gen is the vendor-side CLI for issuing license.lic files.
//
// IMPORTANT: This binary uses the PRIVATE key and must NEVER ship to
// customers. Keep it on a vendor host (or HSM-wrapped, ideally).
//
// Subcommands:
//
//	license-gen keygen    -out-dir config/
//	    Generates a fresh Ed25519 keypair. Writes:
//	        config/license_privkey.pem  (vendor only, mode 0600)
//	        config/license_pubkey.pem   (ships embedded into customer binary)
//	        backend/internal/license/pubkey_embed.pem  (the actual embed source)
//
//	license-gen create \
//	    -priv config/license_privkey.pem \
//	    -customer "ACME Corp" \
//	    -customer-id ACME-001 \
//	    -expiry 2027-05-15 \
//	    -features AI_CFO,Forecasting,FinancialReports \
//	    -max-users 50 \
//	    -type Enterprise \
//	    [-machine-id <hex>]                # optional vendor-side hard-bind
//	    -out license.lic
//
//	license-gen renew \
//	    -priv config/license_privkey.pem \
//	    -request request.dat \
//	    -expiry 2028-05-15 \
//	    [-features ...]                    # default: copy existing
//	    -out license.lic
//
//	license-gen show -in license.lic       # human-readable dump
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cfo/backend/internal/license"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		cmdKeygen(os.Args[2:])
	case "create":
		cmdCreate(os.Args[2:])
	case "renew":
		cmdRenew(os.Args[2:])
	case "show":
		cmdShow(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`license-gen — vendor-side license issuance (DO NOT SHIP)

Commands:
  keygen       Generate a fresh Ed25519 keypair
  create       Issue a new license.lic
  renew        Issue a renewed license.lic from a request.dat
  show         Pretty-print a license.lic for inspection

Run "license-gen <command> -h" for command-specific flags.`)
}

// ---------------------------------------------------------------------
// keygen
// ---------------------------------------------------------------------
func cmdKeygen(args []string) {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	outDir := fs.String("out-dir", "config", "directory to write keypair PEM files")
	embedPath := fs.String("embed-path", "backend/internal/license/pubkey_embed.pem",
		"also overwrite the embed-source PEM so the next build picks up the new key")
	_ = fs.Parse(args)

	pub, priv, err := license.GenerateKeypair()
	if err != nil {
		fail("keygen: %v", err)
	}
	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		fail("mkdir: %v", err)
	}
	privPath := *outDir + "/license_privkey.pem"
	pubPath := *outDir + "/license_pubkey.pem"
	if err := license.SavePrivateKeyPEM(privPath, priv); err != nil {
		fail("save priv: %v", err)
	}
	if err := license.SavePublicKeyPEM(pubPath, pub); err != nil {
		fail("save pub: %v", err)
	}
	if *embedPath != "" {
		if err := license.SavePublicKeyPEM(*embedPath, pub); err != nil {
			fail("save embed pub: %v", err)
		}
	}
	fmt.Printf("OK    keypair written:\n")
	fmt.Printf("        private : %s (mode 0600 — keep on vendor host)\n", privPath)
	fmt.Printf("        public  : %s\n", pubPath)
	if *embedPath != "" {
		fmt.Printf("        embed   : %s (will bake into next backend build)\n", *embedPath)
	}
	fmt.Printf("\n  Public key (base64): %s\n", base64.StdEncoding.EncodeToString(pub))
}

// ---------------------------------------------------------------------
// create
// ---------------------------------------------------------------------
func cmdCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	privPath := fs.String("priv", "config/license_privkey.pem", "vendor private key PEM")
	customerName := fs.String("customer", "", "customer display name (required)")
	customerID := fs.String("customer-id", "", "customer ID (required)")
	expiryStr := fs.String("expiry", "", "expiry date YYYY-MM-DD (required)")
	featuresCSV := fs.String("features", "AI_CFO", "comma-separated feature list")
	maxUsers := fs.Int("max-users", 1, "max users (informational only — not enforced)")
	licType := fs.String("type", string(license.TypeEnterprise),
		"license type: Trial | Startup | Enterprise | Unlimited")
	machineID := fs.String("machine-id", "", "optional: hard-bind to specific machine_id (skip for first-activation bind)")
	out := fs.String("out", "license.lic", "output file path")
	_ = fs.Parse(args)

	if *customerName == "" || *customerID == "" || *expiryStr == "" {
		fail("create: -customer, -customer-id, -expiry are required")
	}
	expiry, err := time.Parse("2006-01-02", *expiryStr)
	if err != nil {
		fail("create: bad -expiry: %v", err)
	}

	priv, err := license.LoadPrivateKeyPEM(*privPath)
	if err != nil {
		fail("create: %v", err)
	}

	var features []license.Feature
	for _, f := range strings.Split(*featuresCSV, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			features = append(features, license.Feature(f))
		}
	}
	if len(features) == 0 {
		fail("create: -features cannot be empty")
	}

	nonce, _ := randomHex(16)
	payload := license.Payload{
		CustomerName: *customerName,
		CustomerID:   *customerID,
		IssuedAt:     time.Now().UTC().Truncate(time.Second),
		Expiry:       expiry.UTC(),
		Features:     features,
		MaxUsers:     *maxUsers,
		LicenseType:  license.Type(*licType),
		MachineID:    *machineID,
		Nonce:        nonce,
	}
	f, _, err := license.SignPayload(payload, priv)
	if err != nil {
		fail("create: sign: %v", err)
	}
	if err := writeJSON(*out, f, 0o644); err != nil {
		fail("create: write: %v", err)
	}
	fmt.Printf("OK    license.lic written: %s\n", *out)
	fmt.Printf("        customer : %s (%s)\n", payload.CustomerName, payload.CustomerID)
	fmt.Printf("        type     : %s\n", payload.LicenseType)
	fmt.Printf("        expiry   : %s\n", payload.Expiry.Format("2006-01-02"))
	fmt.Printf("        features : %v\n", payload.Features)
}

// ---------------------------------------------------------------------
// renew
// ---------------------------------------------------------------------
func cmdRenew(args []string) {
	fs := flag.NewFlagSet("renew", flag.ExitOnError)
	privPath := fs.String("priv", "config/license_privkey.pem", "vendor private key PEM")
	reqPath := fs.String("request", "", "customer's request.dat (required)")
	expiryStr := fs.String("expiry", "", "new expiry YYYY-MM-DD (required)")
	featuresCSV := fs.String("features", "", "(optional) override features; default: re-use customer's last license features")
	out := fs.String("out", "license.lic", "output file path")
	customerName := fs.String("customer", "", "(optional) override customer name")
	licType := fs.String("type", "", "(optional) override license type")
	maxUsers := fs.Int("max-users", 0, "(optional) override max-users")
	_ = fs.Parse(args)

	if *reqPath == "" || *expiryStr == "" {
		fail("renew: -request and -expiry are required")
	}
	raw, err := os.ReadFile(*reqPath)
	if err != nil {
		fail("renew: read request: %v", err)
	}
	var req license.RenewalRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		fail("renew: bad request.dat: %v", err)
	}
	expiry, err := time.Parse("2006-01-02", *expiryStr)
	if err != nil {
		fail("renew: bad -expiry: %v", err)
	}
	priv, err := license.LoadPrivateKeyPEM(*privPath)
	if err != nil {
		fail("renew: %v", err)
	}

	var features []license.Feature
	if *featuresCSV != "" {
		for _, f := range strings.Split(*featuresCSV, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				features = append(features, license.Feature(f))
			}
		}
	} else {
		// Default: keep AI_CFO only. The vendor should explicitly pass
		// -features for anything richer than that.
		features = []license.Feature{license.FeatureAICFO}
	}

	cname := req.CustomerName
	if *customerName != "" {
		cname = *customerName
	}
	ltype := license.TypeEnterprise
	if *licType != "" {
		ltype = license.Type(*licType)
	}
	users := 1
	if *maxUsers > 0 {
		users = *maxUsers
	}

	nonce, _ := randomHex(16)
	payload := license.Payload{
		CustomerName: cname,
		CustomerID:   req.CustomerID,
		IssuedAt:     time.Now().UTC().Truncate(time.Second),
		Expiry:       expiry.UTC(),
		Features:     features,
		MaxUsers:     users,
		LicenseType:  ltype,
		MachineID:    req.MachineID, // bind renewal to the requesting machine
		Nonce:        nonce,
	}
	f, _, err := license.SignPayload(payload, priv)
	if err != nil {
		fail("renew: sign: %v", err)
	}
	if err := writeJSON(*out, f, 0o644); err != nil {
		fail("renew: write: %v", err)
	}
	fmt.Printf("OK    renewed license.lic written: %s\n", *out)
	fmt.Printf("        customer : %s (%s)\n", payload.CustomerName, payload.CustomerID)
	fmt.Printf("        expiry   : %s (was: %s)\n",
		payload.Expiry.Format("2006-01-02"), req.CurrentExpiry.Format("2006-01-02"))
}

// ---------------------------------------------------------------------
// show
// ---------------------------------------------------------------------
func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	in := fs.String("in", "license.lic", "license file path")
	_ = fs.Parse(args)

	raw, err := os.ReadFile(*in)
	if err != nil {
		fail("show: %v", err)
	}
	var f license.File
	if err := json.Unmarshal(raw, &f); err != nil {
		fail("show: bad file: %v", err)
	}
	payload, _, _, err := f.Decode()
	if err != nil {
		fail("show: decode: %v", err)
	}
	pretty, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(pretty))
	// And verification status (informational).
	if _, err := license.VerifyFile(f); err == nil {
		fmt.Println("\nsignature: VERIFIED against currently-embedded public key")
	} else {
		fmt.Printf("\nsignature: %v\n", err)
	}
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func writeJSON(path string, v any, mode os.FileMode) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, mode)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR  "+format+"\n", args...)
	os.Exit(1)
}
