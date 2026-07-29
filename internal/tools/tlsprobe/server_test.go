package tlsprobe

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

// tlsServer starts a raw TLS listener on loopback with the given config and
// returns its host:port.
//
// httptest is deliberately not used here for two reasons: StartTLS overwrites
// NextProtos, so a server that agrees to no ALPN cannot be expressed, and its
// error log prints a line for every refused handshake, which this suite does on
// purpose many times per run.
func tlsServer(t *testing.T, tune func(*tls.Config)) string {
	return tlsServerWithKey(t, false, tune)
}

// tlsServerWithKey is tlsServer with a choice of key type. An RSA key is needed
// for the RSA-only cipher suites, which are exactly the ones Go classes as
// insecure and leaves out of its default client list.
func tlsServerWithKey(t *testing.T, rsaKey bool, tune func(*tls.Config)) string {
	t.Helper()

	cfg := &tls.Config{Certificates: []tls.Certificate{selfSigned(t, rsaKey)}}
	tune(cfg)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// The handshake is the whole test. Complete it, then hang up.
			go func() {
				_ = conn.(*tls.Conn).Handshake()
				conn.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

// selfSigned builds a throwaway certificate. It is untrusted by design, which
// is also what TestProbeIgnoresBadCertificate needs.
func selfSigned(t *testing.T, rsaKey bool) tls.Certificate {
	t.Helper()

	var (
		key crypto.Signer
		err error
	)
	if rsaKey {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "tlsprobe test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
