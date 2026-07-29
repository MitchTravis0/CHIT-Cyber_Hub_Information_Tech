package wol

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"chit/internal/core"
	"chit/internal/netscan"
)

// checkPorts are the ports a desktop answers on once it is back. TCP only, so
// the check needs no privileges and no extra dependency.
var checkPorts = []int{445, 3389, 22, 80, 443, 135}

const (
	portTimeout   = 700 * time.Millisecond
	checkDeadline = 2 * time.Second
)

type Awake struct {
	IP    string `json:"ip"`
	Alive bool   `json:"alive"`
	// Via is "port 445", "port 3389" and so on, empty when nothing answered.
	Via       string `json:"via"`
	LatencyMS int64  `json:"latencyMs"`
}

// CheckAwake reports whether an address answers a TCP connection yet, so the UI
// can watch a machine come back after a wake-up.
func CheckAwake(ctx context.Context, ip string) (Awake, error) {
	return checkPortsAt(ctx, ip, checkPorts)
}

func checkPortsAt(ctx context.Context, ip string, ports []int) (Awake, error) {
	if net.ParseIP(ip) == nil {
		return Awake{}, core.Errorf(core.CodeInvalidInput,
			"\"%s\" is not an IP address, so there is nothing to check.", ip)
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, checkDeadline)
	defer cancel()

	probe := func(c context.Context, port int) (int, bool) {
		return port, answered(c, ip, port)
	}
	for port := range core.Pool(ctx, ports, len(ports), probe) {
		return Awake{
			IP:        ip,
			Alive:     true,
			Via:       fmt.Sprintf("port %d", port),
			LatencyMS: time.Since(started).Milliseconds(),
		}, nil
	}
	return Awake{IP: ip, LatencyMS: time.Since(started).Milliseconds()}, nil
}

func answered(ctx context.Context, ip string, port int) bool {
	ctx, cancel := context.WithTimeout(ctx, portTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err == nil {
		conn.Close()
		return true
	}
	// A refused connection proves the machine is up even though nothing is
	// listening on that port.
	return netscan.IsRefused(err)
}
