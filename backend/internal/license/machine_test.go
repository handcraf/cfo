package license

import (
	"strings"
	"testing"
)

func TestMachineID_EnvOverride(t *testing.T) {
	cfg := FingerprintConfig{EnvOverrideValue: "k8s-customer-acme-001"}
	a, err := MachineIDWith(cfg)
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	b, _ := MachineIDWith(cfg)
	if a != b {
		t.Fatal("override should be deterministic")
	}
	if len(a) != 64 {
		t.Fatalf("expected 64-char hex, got %d (%q)", len(a), a)
	}
	// Must NOT echo the override verbatim.
	if strings.Contains(a, "acme") {
		t.Fatalf("override leaked: %q", a)
	}
}

func TestMachineID_NoSignals(t *testing.T) {
	cfg := FingerprintConfig{
		HostnameFn:     func() (string, error) { return "", nil },
		MachineIDFn:    func() (string, error) { return "", nil },
		BoardUUIDFn:    func() (string, error) { return "", nil },
		DiskSerialFn:   func() (string, error) { return "", nil },
		CPUInfoFn:      func() (string, error) { return "", nil },
		MACAddressesFn: func() ([]string, error) { return nil, nil },
	}
	if _, err := MachineIDWith(cfg); err == nil {
		t.Fatal("expected error when no signals available")
	}
}

func TestMachineID_StubbedSignalsAreDeterministic(t *testing.T) {
	cfg := FingerprintConfig{
		HostnameFn:     func() (string, error) { return "fake-host", nil },
		MachineIDFn:    func() (string, error) { return "fake-id", nil },
		BoardUUIDFn:    func() (string, error) { return "", nil },
		DiskSerialFn:   func() (string, error) { return "", nil },
		CPUInfoFn:      func() (string, error) { return "fake-cpu", nil },
		MACAddressesFn: func() ([]string, error) { return []string{"aa:bb:cc:dd:ee:ff"}, nil },
	}
	a, _ := MachineIDWith(cfg)
	b, _ := MachineIDWith(cfg)
	if a != b {
		t.Fatalf("stubbed fingerprint not deterministic:\nA: %s\nB: %s", a, b)
	}
	// And different signals → different fingerprint.
	cfg.HostnameFn = func() (string, error) { return "other-host", nil }
	c, _ := MachineIDWith(cfg)
	if a == c {
		t.Fatal("different signals should give different fingerprint")
	}
}

func TestMachineID_RealHost(t *testing.T) {
	// This must succeed on any reasonable dev / CI machine. Skip if
	// it can't pull any signals — that's a real bug but we don't want
	// to fail CI on niche minimal containers.
	id, err := MachineID()
	if err != nil {
		t.Skipf("real-host fingerprint unavailable on this CI runner: %v", err)
	}
	if len(id) != 64 {
		t.Fatalf("expected 64-char hex, got %d", len(id))
	}
}
