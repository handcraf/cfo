// Package license — deterministic machine fingerprinting.
//
// The fingerprint is SHA-256 of a concatenation of hardware/system
// identifiers. Different OSes / environments expose different things,
// so we collect best-effort signals and HASH them together. The hash
// is stable on a given machine across reboots (as long as the same
// signals are available) and uncorrelated across machines.
//
// Why so many sources? Because in air-gapped enterprise deployments
// we run on a mix of:
//
//   - Bare-metal Linux (dmidecode + /etc/machine-id available)
//   - macOS dev box   (IOPlatformUUID via ioreg)
//   - Docker container (dmidecode missing, hostname stable per-container)
//   - Kubernetes pod   (everything ephemeral — pods get rescheduled,
//     the fingerprint MUST be overridable via env var)
//
// The fingerprint accepts an env-var override (LICENSE_MACHINE_ID) for
// exactly this reason: in K8s, the customer mounts a stable ID from a
// ConfigMap, and the in-pod fingerprint resolution doesn't matter.
package license

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// FingerprintConfig allows tests to inject stub signal collectors so
// we don't depend on `ioreg` or `dmidecode` being installed in CI.
// Production callers use DefaultFingerprintConfig().
type FingerprintConfig struct {
	HostnameFn       func() (string, error)
	MachineIDFn      func() (string, error)
	BoardUUIDFn      func() (string, error)
	DiskSerialFn     func() (string, error)
	CPUInfoFn        func() (string, error)
	MACAddressesFn   func() ([]string, error)
	EnvOverrideValue string // value of LICENSE_MACHINE_ID; "" = no override
}

// DefaultFingerprintConfig returns the production signal collectors.
func DefaultFingerprintConfig() FingerprintConfig {
	return FingerprintConfig{
		HostnameFn:       os.Hostname,
		MachineIDFn:      defaultMachineID,
		BoardUUIDFn:      defaultBoardUUID,
		DiskSerialFn:     defaultDiskSerial,
		CPUInfoFn:        defaultCPUInfo,
		MACAddressesFn:   defaultMACs,
		EnvOverrideValue: os.Getenv("LICENSE_MACHINE_ID"),
	}
}

// MachineID returns the stable fingerprint for the host running this
// process. The string is a lowercase hex SHA-256 (64 chars).
//
// If LICENSE_MACHINE_ID is set, that value is used verbatim AFTER hashing
// it with SHA-256 — so the override is also fixed-length and doesn't
// leak whatever raw value the customer chose.
//
// The fingerprint is deterministic per (host, kernel-uname, hw set) and
// uses a sorted/canonical ordering so collection order doesn't affect
// the result.
func MachineID() (string, error) {
	return MachineIDWith(DefaultFingerprintConfig())
}

// MachineIDWith is the test-injectable variant.
func MachineIDWith(cfg FingerprintConfig) (string, error) {
	if cfg.EnvOverrideValue != "" {
		sum := sha256.Sum256([]byte("override:" + cfg.EnvOverrideValue))
		return hex.EncodeToString(sum[:]), nil
	}

	// Collect every signal we can; missing/erroring signals contribute
	// nothing but don't fail the whole call. We need AT LEAST one
	// non-empty signal — otherwise the fingerprint would be the SHA of
	// an empty string, which would be identical on every fingerprint-less
	// machine and would defeat the point of binding.
	collected := map[string]string{
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
	}
	tryAdd := func(name string, fn func() (string, error)) {
		if fn == nil {
			return
		}
		v, err := fn()
		if err == nil && strings.TrimSpace(v) != "" {
			collected[name] = strings.TrimSpace(v)
		}
	}
	tryAdd("hostname", cfg.HostnameFn)
	tryAdd("machine_id", cfg.MachineIDFn)
	tryAdd("board_uuid", cfg.BoardUUIDFn)
	tryAdd("disk_serial", cfg.DiskSerialFn)
	tryAdd("cpu", cfg.CPUInfoFn)
	if cfg.MACAddressesFn != nil {
		if macs, err := cfg.MACAddressesFn(); err == nil && len(macs) > 0 {
			sort.Strings(macs)
			collected["macs"] = strings.Join(macs, ",")
		}
	}

	// Require at least one identifying signal beyond os/arch.
	if len(collected) <= 2 {
		return "", New(ReasonUnknown, "machine fingerprint: no usable hardware signals (set LICENSE_MACHINE_ID env var)")
	}

	// Canonical ordering for the hash input.
	keys := make([]string, 0, len(collected))
	for k := range collected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s\n", k, collected[k])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------------------------------------------------------------------
// Platform-specific signal collectors. Each returns "" + nil error on
// "expected absence" (e.g. ioreg missing on Linux) — only return errors
// for genuine surprises.
// ---------------------------------------------------------------------

func defaultMachineID() (string, error) {
	// Linux + systemd: /etc/machine-id is the canonical stable ID.
	// macOS doesn't have it. Inside Docker, it's usually inherited
	// from the host's host-mounted file — good enough for our needs.
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", nil
}

func defaultBoardUUID() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// Best-effort: /sys/class/dmi/id/product_uuid (root-only, often
		// available in privileged containers). Fall back to product_serial.
		for _, p := range []string{
			"/sys/class/dmi/id/product_uuid",
			"/sys/class/dmi/id/product_serial",
		} {
			if b, err := os.ReadFile(p); err == nil {
				return strings.TrimSpace(string(b)), nil
			}
		}
		// Try dmidecode if the binary is installed and we're root.
		if _, err := exec.LookPath("dmidecode"); err == nil {
			out, err := exec.Command("dmidecode", "-s", "system-uuid").Output()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
	case "darwin":
		// macOS: IOPlatformUUID via ioreg. Always present.
		out, err := exec.Command(
			"ioreg", "-d2", "-c", "IOPlatformExpertDevice",
		).Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformUUID") {
					if idx := strings.Index(line, `"IOPlatformUUID" = "`); idx >= 0 {
						s := line[idx+len(`"IOPlatformUUID" = "`):]
						if end := strings.Index(s, `"`); end >= 0 {
							return s[:end], nil
						}
					}
				}
			}
		}
	}
	return "", nil
}

func defaultDiskSerial() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// /sys/block/*/device/serial works for SATA/NVMe on most setups.
		entries, err := os.ReadDir("/sys/block")
		if err != nil {
			return "", nil
		}
		var serials []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "loop") ||
				strings.HasPrefix(e.Name(), "ram") ||
				strings.HasPrefix(e.Name(), "dm-") {
				continue
			}
			b, err := os.ReadFile("/sys/block/" + e.Name() + "/device/serial")
			if err == nil {
				if s := strings.TrimSpace(string(b)); s != "" {
					serials = append(serials, s)
				}
			}
		}
		if len(serials) > 0 {
			sort.Strings(serials)
			return strings.Join(serials, "|"), nil
		}
	case "darwin":
		// diskutil info /
		out, err := exec.Command("diskutil", "info", "/").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "Volume UUID") ||
					strings.Contains(line, "Disk / Partition UUID") {
					if idx := strings.LastIndex(line, ":"); idx >= 0 {
						return strings.TrimSpace(line[idx+1:]), nil
					}
				}
			}
		}
	}
	return "", nil
}

func defaultCPUInfo() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// /proc/cpuinfo's "model name" line.
		b, err := os.ReadFile("/proc/cpuinfo")
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "model name") {
					if idx := strings.Index(line, ":"); idx >= 0 {
						return strings.TrimSpace(line[idx+1:]), nil
					}
				}
			}
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return fmt.Sprintf("%s/%s/cpus=%d", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()), nil
}

func defaultMACs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ifc := range ifaces {
		// Skip loopback and zero/unset MACs.
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		mac := ifc.HardwareAddr.String()
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		// Skip likely-virtual interfaces (Docker bridges, veth).
		lower := strings.ToLower(ifc.Name)
		if strings.HasPrefix(lower, "docker") ||
			strings.HasPrefix(lower, "br-") ||
			strings.HasPrefix(lower, "veth") ||
			strings.HasPrefix(lower, "virbr") {
			continue
		}
		out = append(out, mac)
	}
	return out, nil
}
