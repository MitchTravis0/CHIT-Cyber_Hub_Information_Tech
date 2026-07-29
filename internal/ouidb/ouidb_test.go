package ouidb

import (
	"strings"
	"sync"
	"testing"
)

func TestParseMACForms(t *testing.T) {
	want := [6]byte{0x00, 0x1B, 0xC5, 0x0D, 0xEE, 0xFF}
	forms := []string{
		"00:1B:C5:0D:EE:FF",
		"00-1b-c5-0d-ee-ff",
		"001b.c50d.eeff",
		"001BC50DEEFF",
		"  00:1b:C5:0d:EE:ff  ",
		"00 1b c5 0d ee ff",
		"001B-C50D-EEFF",
	}
	for _, form := range forms {
		got, ok := ParseMAC(form)
		if !ok {
			t.Errorf("ParseMAC(%q) rejected a valid address", form)
			continue
		}
		if got != want {
			t.Errorf("ParseMAC(%q) = %v, want %v", form, got, want)
		}
	}
}

func TestParseMACRejects(t *testing.T) {
	bad := []string{
		"",
		"not a mac",
		"00:1B:C5:0D:EE",          // 40 bits
		"00:1B:C5:0D:EE:FF:AA",    // 56 bits
		"00:1B:C5:0D:EE:FG",       // G is not hex
		"00:1B:C5:0D:EE:F",        // odd digit count
		"192.168.1.1",             // an IP, a very likely paste
		"00:1B:C5:0D:EE:FF extra", // trailing words
	}
	for _, in := range bad {
		if _, ok := ParseMAC(in); ok {
			t.Errorf("ParseMAC(%q) accepted invalid input", in)
		}
	}
}

func TestNormalize(t *testing.T) {
	got, ok := Normalize("001b.c50d.eeff")
	if !ok || got != "00:1B:C5:0D:EE:FF" {
		t.Fatalf("Normalize = %q, %v; want 00:1B:C5:0D:EE:FF, true", got, ok)
	}
	if _, ok := Normalize("nope"); ok {
		t.Fatal("Normalize accepted invalid input")
	}
}

func TestLookupKnownVendors(t *testing.T) {
	// Long-standing assignments that will not disappear from the registry.
	cases := []struct {
		mac      string
		contains string
	}{
		{"00:0C:29:12:34:56", "VMware"},       // 24-bit, VMware guests
		{"B8:27:EB:00:11:22", "Raspberry Pi"}, // 24-bit, Raspberry Pi
		{"00:00:0C:AA:BB:CC", "Cisco"},        // 24-bit, the original Cisco OUI
		{"3C:5A:B4:01:02:03", "Google"},       // 24-bit, Google
		{"00:50:56:AA:BB:CC", "VMware"},       // 24-bit, VMware vSwitch
		{"00:1B:C5:00:00:01", "Converging"},   // 36-bit MA-S block
		{"00:55:DA:00:01:02", "Shinko"},       // 28-bit MA-M block
	}
	for _, c := range cases {
		vendor, ok := Lookup(c.mac)
		if !ok {
			t.Errorf("Lookup(%s) found nothing, want a vendor containing %q", c.mac, c.contains)
			continue
		}
		if !strings.Contains(vendor, c.contains) {
			t.Errorf("Lookup(%s) = %q, want it to contain %q", c.mac, vendor, c.contains)
		}
	}
}

// A 36-bit block sits inside an OUI shared by many vendors, so a different
// sub-block must not inherit its neighbour's name.
func TestLongestPrefixWins(t *testing.T) {
	first, ok1 := Lookup("00:1B:C5:00:00:01")
	second, ok2 := Lookup("00:1B:C5:00:10:01")
	if !ok1 || !ok2 {
		t.Fatalf("expected both MA-S sub-blocks to resolve, got %v / %v", ok1, ok2)
	}
	if first == second {
		t.Fatalf("both MA-S sub-blocks resolved to %q, the 36-bit map is not being consulted", first)
	}
}

func TestLookupUnknownPrefix(t *testing.T) {
	// Unicast, universally administered, and unassigned by IEEE.
	if vendor, ok := Lookup("02:00:00:00:00:00"); ok {
		t.Errorf("Lookup of a locally administered address returned %q", vendor)
	}
	if _, ok := Lookup("garbage"); ok {
		t.Error("Lookup accepted invalid input")
	}
}

func TestDescribeRandomized(t *testing.T) {
	// Bit 1 of the first octet set: exactly what a phone with MAC randomization
	// puts on the wire.
	info, ok := Describe("A6:5E:60:12:34:56")
	if !ok {
		t.Fatal("Describe rejected a valid address")
	}
	if !info.LocallyAdministered || !info.Randomized {
		t.Fatalf("A6:... should be locally administered and randomized, got %+v", info)
	}
	if info.Multicast || info.Broadcast {
		t.Fatalf("A6:... is unicast, got %+v", info)
	}
	if info.Found {
		t.Fatalf("a randomized address must not report a vendor, got %q", info.Vendor)
	}
	if !strings.Contains(info.Note, "randomized") {
		t.Fatalf("note should explain randomization, got %q", info.Note)
	}
}

func TestDescribeBits(t *testing.T) {
	cases := []struct {
		mac                      string
		local, random, mc, bcast bool
	}{
		{"00:0C:29:12:34:56", false, false, false, false}, // universal unicast
		{"02:00:00:00:00:01", true, true, false, false},   // locally administered
		{"06:00:00:00:00:01", true, true, false, false},   // bit 1 set, bit 0 clear
		{"01:00:5E:00:00:FB", false, false, true, false},  // IPv4 multicast
		{"33:33:00:00:00:01", true, false, true, false},   // IPv6 multicast
		{"FF:FF:FF:FF:FF:FF", true, false, true, true},    // broadcast
	}
	for _, c := range cases {
		info, ok := Describe(c.mac)
		if !ok {
			t.Errorf("Describe(%s) rejected a valid address", c.mac)
			continue
		}
		if info.LocallyAdministered != c.local || info.Randomized != c.random ||
			info.Multicast != c.mc || info.Broadcast != c.bcast {
			t.Errorf("Describe(%s) local=%v random=%v mc=%v bcast=%v; want %v %v %v %v",
				c.mac, info.LocallyAdministered, info.Randomized, info.Multicast, info.Broadcast,
				c.local, c.random, c.mc, c.bcast)
		}
	}
}

func TestDescribeNotes(t *testing.T) {
	cases := []struct {
		mac      string
		contains string
	}{
		{"FF:FF:FF:FF:FF:FF", "broadcast"},
		{"01:00:5E:00:00:FB", "multicast"},
		{"A6:5E:60:12:34:56", "randomized"},
		{"FC:FF:FF:FF:FF:FF", "not in the vendor database"},
	}
	for _, c := range cases {
		info, _ := Describe(c.mac)
		if !strings.Contains(info.Note, c.contains) {
			t.Errorf("Describe(%s).Note = %q, want it to contain %q", c.mac, info.Note, c.contains)
		}
	}
	info, _ := Describe("00:0C:29:12:34:56")
	if info.Note != "" {
		t.Errorf("a plain vendor hit needs no note, got %q", info.Note)
	}
}

func TestDescribeFields(t *testing.T) {
	info, ok := Describe("b827.eb00.1122")
	if !ok {
		t.Fatal("Describe rejected a valid address")
	}
	if info.MAC != "B8:27:EB:00:11:22" {
		t.Errorf("MAC = %q, want B8:27:EB:00:11:22", info.MAC)
	}
	if info.OUI != "B8:27:EB" {
		t.Errorf("OUI = %q, want B8:27:EB", info.OUI)
	}
	if !info.Found || info.Vendor == "" {
		t.Errorf("expected a vendor, got %+v", info)
	}
	if _, ok := Describe("hello"); ok {
		t.Error("Describe accepted invalid input")
	}
}

func TestMetadata(t *testing.T) {
	m := Metadata()
	if m.Records < 40000 {
		t.Errorf("Records = %d, the embedded database looks truncated", m.Records)
	}
	if m.Source == "" || m.Fetched == "" {
		t.Errorf("Metadata is missing provenance: %+v", m)
	}
}

// The scanner looks up one vendor per responding host from a worker pool.
func TestConcurrentLookup(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := Lookup("00:0C:29:12:34:56"); !ok {
				t.Error("concurrent lookup missed a known vendor")
			}
			Metadata()
		}()
	}
	wg.Wait()
}

func BenchmarkLookup(b *testing.B) {
	Metadata()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Lookup("00:0C:29:12:34:56")
	}
}
