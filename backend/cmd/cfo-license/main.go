// cfo-license is the on-device CLI that ships to customers.
//
//	cfo-license status                      — show binding state, expiry, features
//	cfo-license deactivate [-out migration.dat] — emit signed migration token
//	cfo-license activate <migration.dat>    — bind license to this machine
//	cfo-license export-request [-out request.dat] — produce renewal request
//
// Defaults assume the license.lic and the encrypted state file live in
// the backend data dir. Override via -license / -state flags.
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cfo/backend/internal/license"
)

func defaultLicensePath() string {
	if p := os.Getenv("LICENSE_FILE"); p != "" {
		return p
	}
	return "license.lic"
}
func defaultStatePath() string {
	if p := os.Getenv("LICENSE_STATE"); p != "" {
		return p
	}
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "./backend/data"
	}
	return filepath.Join(dir, "state", "license.state.enc")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "status":
		cmdStatus(os.Args[2:])
	case "deactivate":
		cmdDeactivate(os.Args[2:])
	case "activate":
		cmdActivate(os.Args[2:])
	case "export-request":
		cmdExportRequest(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`cfo-license — on-device license management

Commands:
  status            Show current license binding, expiry, features, machine ID
  deactivate        Generate a signed migration.dat for moving to a new machine
  activate <file>   Bind the license to THIS machine using a migration.dat
  export-request    Generate a request.dat to send to the vendor for renewal

Environment:
  LICENSE_FILE     path to license.lic              (default: ./license.lic)
  LICENSE_STATE    path to encrypted state file     (default: $DATA_DIR/state/license.state.enc)

Run "cfo-license <command> -h" for command-specific flags.`)
}

// ---------------------------------------------------------------------
// status
// ---------------------------------------------------------------------
func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	licPath := fs.String("license", defaultLicensePath(), "path to license.lic")
	statePath := fs.String("state", defaultStatePath(), "path to encrypted state")
	jsonOut := fs.Bool("json", false, "emit JSON (machine-readable)")
	_ = fs.Parse(args)

	v := license.NewVerifier(*licPath, *statePath)
	r := v.Verify(license.VerifyOptions{SkipMachineBind: false})

	if *jsonOut {
		writeJSON(os.Stdout, r)
		return
	}

	printHuman(r, *licPath, *statePath)
	if !r.OK {
		os.Exit(1)
	}
}

func printHuman(r license.VerifyResult, licPath, statePath string) {
	fmt.Println("=== AI CFO License Status ===")
	fmt.Println()
	fmt.Printf("  license file : %s\n", licPath)
	fmt.Printf("  state file   : %s\n", statePath)
	if r.Payload != nil {
		fmt.Printf("  customer     : %s (%s)\n", r.Payload.CustomerName, r.Payload.CustomerID)
		fmt.Printf("  type         : %s\n", r.Payload.LicenseType)
		fmt.Printf("  issued       : %s\n", r.Payload.IssuedAt.Format("2006-01-02"))
		fmt.Printf("  expires      : %s\n", r.Payload.Expiry.Format("2006-01-02"))
		fmt.Printf("  features     : %v\n", r.Payload.Features)
		fmt.Printf("  max users    : %d (informational only)\n", r.Payload.MaxUsers)
	}
	fmt.Printf("  machine id   : %s\n", r.MachineID)
	fmt.Println()
	if r.OK {
		fmt.Printf("  STATUS       : valid — %d days remaining\n", r.DaysRemaining)
		if r.WarningDays > 0 {
			fmt.Printf("  ⚠ WARNING    : license expires soon; run `cfo-license export-request` to start renewal\n")
		}
	} else {
		fmt.Printf("  STATUS       : INVALID — %s\n", r.Reason)
		fmt.Printf("  reason       : %s\n", r.Message)
	}
}

// ---------------------------------------------------------------------
// deactivate
// ---------------------------------------------------------------------
func cmdDeactivate(args []string) {
	fs := flag.NewFlagSet("deactivate", flag.ExitOnError)
	licPath := fs.String("license", defaultLicensePath(), "path to license.lic")
	statePath := fs.String("state", defaultStatePath(), "path to encrypted state")
	out := fs.String("out", "migration.dat", "output migration file")
	pubOut := fs.String("pub-out", "migration_pub.key",
		"output: ed25519 PUBLIC key needed on the destination machine for activate")
	_ = fs.Parse(args)

	v := license.NewVerifier(*licPath, *statePath)
	m, err := v.Deactivate()
	if err != nil {
		fail("deactivate: %v", err)
	}

	// Also export the activation public key so the destination machine
	// can verify the migration without contacting the vendor.
	st, err := license.LoadState(*statePath)
	if err != nil {
		fail("deactivate: read state: %v", err)
	}
	if err := writeFileJSON(*out, m, 0o644); err != nil {
		fail("deactivate: write: %v", err)
	}
	if err := os.WriteFile(*pubOut, []byte(st.ActivationPubKeyB64+"\n"), 0o644); err != nil {
		fail("deactivate: write pub: %v", err)
	}

	fmt.Println("OK    deactivated. Backend on THIS host will refuse to start until reactivated.")
	fmt.Printf("        migration : %s\n", *out)
	fmt.Printf("        pub key   : %s\n", *pubOut)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Copy license.lic, migration.dat, AND migration_pub.key to the new machine.")
	fmt.Println("  2. On the new machine, run:")
	fmt.Printf("       cfo-license activate %s -pub %s\n", *out, *pubOut)
}

// ---------------------------------------------------------------------
// activate
// ---------------------------------------------------------------------
func cmdActivate(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(`cfo-license activate <migration.dat> [flags]

  -license <path>   path to license.lic       (env LICENSE_FILE)
  -state   <path>   path to encrypted state   (env LICENSE_STATE)
  -pub     <path>   ed25519 public key from old host (default: migration_pub.key)`)
		if len(args) == 0 {
			os.Exit(2)
		}
		return
	}
	migrationPath := args[0]

	fs := flag.NewFlagSet("activate", flag.ExitOnError)
	licPath := fs.String("license", defaultLicensePath(), "path to license.lic")
	statePath := fs.String("state", defaultStatePath(), "path to encrypted state")
	pubPath := fs.String("pub", "migration_pub.key", "activation pubkey from old host")
	_ = fs.Parse(args[1:])

	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		fail("activate: read migration: %v", err)
	}
	var m license.MigrationFile
	if err := json.Unmarshal(raw, &m); err != nil {
		fail("activate: bad migration.dat: %v", err)
	}
	pubRaw, err := os.ReadFile(*pubPath)
	if err != nil {
		fail("activate: read pub: %v", err)
	}
	pub, err := base64.StdEncoding.DecodeString(string(bytesTrim(pubRaw)))
	if err != nil {
		fail("activate: pub base64: %v", err)
	}

	v := license.NewVerifier(*licPath, *statePath)
	if err := v.Activate(m, pub); err != nil {
		fail("activate: %v", err)
	}
	r := v.Verify(license.VerifyOptions{})
	if !r.OK {
		fail("activate: verify after bind failed: %s", r.Message)
	}
	fmt.Println("OK    license activated on this machine.")
	fmt.Printf("        customer  : %s (%s)\n", r.Payload.CustomerName, r.Payload.CustomerID)
	fmt.Printf("        machine   : %s\n", r.MachineID)
	fmt.Printf("        expires   : %s (%d days remaining)\n",
		r.Payload.Expiry.Format("2006-01-02"), r.DaysRemaining)
}

// ---------------------------------------------------------------------
// export-request
// ---------------------------------------------------------------------
func cmdExportRequest(args []string) {
	fs := flag.NewFlagSet("export-request", flag.ExitOnError)
	licPath := fs.String("license", defaultLicensePath(), "path to license.lic")
	statePath := fs.String("state", defaultStatePath(), "path to encrypted state")
	out := fs.String("out", "request.dat", "output file path")
	_ = fs.Parse(args)

	v := license.NewVerifier(*licPath, *statePath)
	req, err := v.ExportRenewalRequest()
	if err != nil {
		fail("export-request: %v", err)
	}
	if err := writeFileJSON(*out, req, 0o644); err != nil {
		fail("export-request: write: %v", err)
	}
	fmt.Printf("OK    renewal request written to %s\n", *out)
	fmt.Println("    Send this file to your vendor. They will send back a renewed license.lic.")
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func writeJSON(w *os.File, v any) {
	raw, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(w, string(raw))
}

func writeFileJSON(path string, v any, mode os.FileMode) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, mode)
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == ' ' || b[len(b)-1] == '\r' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n') {
		b = b[1:]
	}
	return b
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR  "+format+"\n", args...)
	os.Exit(1)
}
