package netscan

import (
	"reflect"
	"testing"
)

func TestNormalizeMAC(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF"},
		{"AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF"},
		{"aa-bb-cc-dd-ee-ff", "AA:BB:CC:DD:EE:FF"},
		{"0:11:22:33:44:5", "00:11:22:33:44:05"},
		{" 0:1:2:3:4:5 ", "00:01:02:03:04:05"},
		{"", ""},
		{"incomplete", ""},
		{"aa:bb:cc:dd:ee", ""},
		{"aa:bb:cc:dd:ee:ff:00", ""},
		{"aa:bb:cc:dd:ee:gg", ""},
		{"aaa:bb:cc:dd:ee:ff", ""},
		{"aa:bb:cc:dd:ee:", ""},
		{"192.168.1.1", ""},
		{"dynamic", ""},
	}
	for _, tc := range cases {
		if got := normalizeMAC(tc.in); got != tc.want {
			t.Errorf("normalizeMAC(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUsableMAC(t *testing.T) {
	cases := map[string]bool{
		"AA:BB:CC:DD:EE:FF": true,
		"00:11:22:33:44:55": true,
		"":                  false,
		"00:00:00:00:00:00": false,
		"FF:FF:FF:FF:FF:FF": false,
		"01:00:5E:00:00:16": false,
		"33:33:00:00:00:FB": false,
	}
	for mac, want := range cases {
		if got := usableMAC(mac); got != want {
			t.Errorf("usableMAC(%q) = %v, want %v", mac, got, want)
		}
	}
}

func TestParseProcNetARP(t *testing.T) {
	const fixture = `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         3c:37:86:11:22:33     *        wlan0
192.168.1.42     0x1         0x2         00:1a:2b:3c:4d:5e     *        wlan0
192.168.1.99     0x1         0x0         00:00:00:00:00:00     *        wlan0
192.168.1.255    0x1         0x2         ff:ff:ff:ff:ff:ff     *        wlan0
224.0.0.22       0x1         0x2         01:00:5e:00:00:16     *        wlan0
10.8.0.1         0x1         0x2         aa:bb:cc:dd:ee:ff     *        tun0
`
	want := []ARPEntry{
		{IP: "192.168.1.1", MAC: "3C:37:86:11:22:33"},
		{IP: "192.168.1.42", MAC: "00:1A:2B:3C:4D:5E"},
		{IP: "10.8.0.1", MAC: "AA:BB:CC:DD:EE:FF"},
	}
	if got := parseProcNetARP(fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("parseProcNetARP =\n%v\nwant\n%v", got, want)
	}
}

func TestParseIPNeigh(t *testing.T) {
	const fixture = `192.168.1.1 dev wlan0 lladdr 3c:37:86:11:22:33 REACHABLE
192.168.1.42 dev wlan0 lladdr 00:1a:2b:3c:4d:5e STALE
192.168.1.77 dev wlan0  FAILED
192.168.1.78 dev wlan0 lladdr 00:00:00:00:00:00 INCOMPLETE
fe80::1 dev wlan0 lladdr 3c:37:86:11:22:44 router STALE
192.168.1.90 dev wlan0 lladdr 0:1:2:3:4:5 DELAY
`
	want := []ARPEntry{
		{IP: "192.168.1.1", MAC: "3C:37:86:11:22:33"},
		{IP: "192.168.1.42", MAC: "00:1A:2B:3C:4D:5E"},
		{IP: "192.168.1.90", MAC: "00:01:02:03:04:05"},
	}
	if got := parseIPNeigh(fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("parseIPNeigh =\n%v\nwant\n%v", got, want)
	}
}

func TestParseARPCommandWindows(t *testing.T) {
	const fixture = "\r\nInterface: 192.168.1.10 --- 0x5\r\n" +
		"  Internet Address      Physical Address      Type\r\n" +
		"  192.168.1.1           3c-37-86-11-22-33     dynamic\r\n" +
		"  192.168.1.42          00-1a-2b-3c-4d-5e     dynamic\r\n" +
		"  192.168.1.255         ff-ff-ff-ff-ff-ff     static\r\n" +
		"  224.0.0.22            01-00-5e-00-00-16     static\r\n" +
		"  239.255.255.250       01-00-5e-7f-ff-fa     static\r\n"

	want := []ARPEntry{
		{IP: "192.168.1.1", MAC: "3C:37:86:11:22:33"},
		{IP: "192.168.1.42", MAC: "00:1A:2B:3C:4D:5E"},
	}
	if got := parseARPCommand(fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("parseARPCommand =\n%v\nwant\n%v", got, want)
	}
}

func TestParseARPCommandMacOS(t *testing.T) {
	const fixture = `? (192.168.1.1) at 3c:37:86:11:22:33 on en0 ifscope [ethernet]
? (192.168.1.42) at 0:1a:2b:3c:4d:5e on en0 ifscope [ethernet]
? (192.168.1.77) at (incomplete) on en0 ifscope [ethernet]
? (192.168.1.255) at ff:ff:ff:ff:ff:ff on en0 ifscope [ethernet]
? (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]
`
	want := []ARPEntry{
		{IP: "192.168.1.1", MAC: "3C:37:86:11:22:33"},
		{IP: "192.168.1.42", MAC: "00:1A:2B:3C:4D:5E"},
	}
	if got := parseARPCommand(fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("parseARPCommand =\n%v\nwant\n%v", got, want)
	}
}

func TestParseARPCommandLinuxNetTools(t *testing.T) {
	const fixture = `router.lan (192.168.1.1) at 3c:37:86:11:22:33 [ether] on eth0
? (192.168.1.42) at 00:1a:2b:3c:4d:5e [ether] on eth0
? (192.168.1.77) at <incomplete> on eth0
`
	want := []ARPEntry{
		{IP: "192.168.1.1", MAC: "3C:37:86:11:22:33"},
		{IP: "192.168.1.42", MAC: "00:1A:2B:3C:4D:5E"},
	}
	if got := parseARPCommand(fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("parseARPCommand =\n%v\nwant\n%v", got, want)
	}
}

func TestParsersIgnoreJunk(t *testing.T) {
	junk := "\n\n   \nno addresses here at all\narp: bad command\n"
	if got := parseProcNetARP(junk); len(got) != 0 {
		t.Errorf("parseProcNetARP returned %v", got)
	}
	if got := parseIPNeigh(junk); len(got) != 0 {
		t.Errorf("parseIPNeigh returned %v", got)
	}
	if got := parseARPCommand(junk); len(got) != 0 {
		t.Errorf("parseARPCommand returned %v", got)
	}
}
