package dbaccess

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "tenant.db.test"}, DNSNames: []string{"tenant.db.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// This backend requires PostgreSQL SSLRequest, negotiates real TLS and echoes
// application bytes. WebSocket renewal frames must never enter the TLS stream.
func testTransport(t *testing.T, cert tls.Certificate) (*httptest.Server, *websocket.Dialer, *atomic.Int32, <-chan string) {
	t.Helper()
	var accepted atomic.Int32
	renewals := make(chan string, 32)
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer first-permit" {
			http.Error(w, "forbidden", 403)
			return
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		// A net.Pipe separates the opaque bridge from the TLS endpoint, as in the gateway.
		relay, backend := net.Pipe()
		defer relay.Close()
		defer backend.Close()
		go func() {
			defer backend.Close()
			ssl := make([]byte, 8)
			if _, err := io.ReadFull(backend, ssl); err != nil || !bytes.Equal(ssl, []byte{0, 0, 0, 8, 4, 210, 22, 47}) {
				return
			}
			backend.Write([]byte("S"))
			secure := tls.Server(backend, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
			if secure.Handshake() != nil {
				return
			}
			accepted.Add(1)
			io.Copy(secure, secure)
		}()
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer relay.Close()
			defer ws.Close()
			buf := make([]byte, 32<<10)
			for {
				n, err := relay.Read(buf)
				if n > 0 && ws.WriteMessage(websocket.BinaryMessage, buf[:n]) != nil {
					return
				}
				if err != nil {
					return
				}
			}
		}()
		for {
			kind, body, err := ws.ReadMessage()
			if err != nil {
				break
			}
			if kind == websocket.TextMessage {
				var v struct{ Type, Token string }
				if json.Unmarshal(body, &v) != nil || v.Type != "renew" {
					break
				}
				renewals <- v.Token
			} else if kind == websocket.BinaryMessage {
				if _, err := relay.Write(body); err != nil {
					break
				}
			} else {
				break
			}
		}
		relay.Close()
		ws.Close()
		<-done
	}))
	t.Cleanup(s.Close)
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	return s, &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: roots}, HandshakeTimeout: time.Second}, &accepted, renewals
}

func permitFor(s *httptest.Server, ca string) *Permit {
	return &Permit{Endpoint: s.URL + "/connect/test-1/00000000-0000-0000-0000-000000000001", Token: "first-permit", ExpiresAt: time.Now().Add(2 * time.Second), TLSServerName: "tenant.db.test", TLSCACert: ca}
}
func startForward(t *testing.T, p Provider, d *websocket.Dialer) (net.Conn, <-chan error) {
	t.Helper()
	local, cli := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { defer cli.Close(); done <- forwardWithDialer(ctx, cli, p, d, nil) }()
	t.Cleanup(func() { cancel(); local.Close() })
	local.SetDeadline(time.Now().Add(8 * time.Second))
	return local, done
}
func exchange(t *testing.T, c net.Conn, b []byte) {
	t.Helper()
	if _, err := c.Write(b); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(b))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, b) {
		t.Fatal("stream changed")
	}
}
func TestForwardRenewsWithoutReconnect(t *testing.T) {
	cert, ca := testCertificate(t)
	s, d, accepted, renewals := testTransport(t, cert)
	var calls atomic.Int32
	initial := permitFor(s, ca)
	local, done := startForward(t, func(ctx context.Context) (*Permit, error) {
		p := *initial
		if calls.Add(1) > 1 {
			p.Token = "renewed-permit"
			p.ExpiresAt = time.Now().Add(5 * time.Second)
		}
		return &p, nil
	}, d)
	// Default libpq SSL negotiation is local only; CLI separately requires verified TLS.
	if _, err := local.Write([]byte{0, 0, 0, 8, 4, 210, 22, 47}); err != nil {
		t.Fatal(err)
	}
	answer := make([]byte, 1)
	if _, err := io.ReadFull(local, answer); err != nil || answer[0] != 'N' {
		t.Fatalf("local SSL response: %v", err)
	}
	exchange(t, local, []byte{0, 0, 0, 16, 0, 3, 0, 0})
	select {
	case token := <-renewals:
		if token != "renewed-permit" {
			t.Fatal("unexpected renewal")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no renewal")
	}
	time.Sleep(max(time.Until(initial.ExpiresAt)+100*time.Millisecond, 0))
	exchange(t, local, []byte("same-session-after-original-expiry"))
	if accepted.Load() != 1 {
		t.Fatal("SQL session was reconnected")
	}
	local.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("forwarder did not stop")
	}
}
func TestForwardClosesAtExpiryWhenRenewalDenied(t *testing.T) {
	cert, ca := testCertificate(t)
	s, d, _, _ := testTransport(t, cert)
	initial := permitFor(s, ca)
	var calls atomic.Int32
	local, done := startForward(t, func(ctx context.Context) (*Permit, error) {
		if calls.Add(1) > 1 {
			return nil, errors.New("revoked")
		}
		return initial, nil
	}, d)
	exchange(t, local, []byte{0, 0, 0, 16, 0, 3, 0, 0})
	select {
	case <-done:
		if time.Now().Before(initial.ExpiresAt.Add(-100 * time.Millisecond)) {
			t.Fatal("closed before expiry")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("revoked session stayed open")
	}
}
func TestForwardRejectsWrongTenantTLS(t *testing.T) {
	cert, ca := testCertificate(t)
	s, d, accepted, _ := testTransport(t, cert)
	p := permitFor(s, ca)
	p.TLSServerName = "another-tenant.db.test"
	local, done := startForward(t, func(context.Context) (*Permit, error) { return p, nil }, d)
	local.Write([]byte{0, 0, 0, 16, 0, 3, 0, 0})
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "TLS verification") {
			t.Fatalf("got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TLS rejection hung")
	}
	if accepted.Load() != 0 {
		t.Fatal("accepted wrong database TLS identity")
	}
}
func TestPermitValidation(t *testing.T) {
	_, ca := testCertificate(t)
	base := Permit{Endpoint: "https://edge.example/connect/test-1/00000000-0000-0000-0000-000000000001", Token: "test", ExpiresAt: time.Now().Add(time.Minute), TLSServerName: "tenant.db.test", TLSCACert: ca}
	for _, tc := range []struct {
		name   string
		change func(*Permit)
	}{
		{"plain HTTP", func(p *Permit) { p.Endpoint = strings.Replace(p.Endpoint, "https:", "http:", 1) }},
		{"query destination", func(p *Permit) { p.Endpoint += "?target=other" }},
		{"missing CA", func(p *Permit) { p.TLSCACert = "" }},
		{"missing tenant identity", func(p *Permit) { p.TLSServerName = "" }},
		{"expired", func(p *Permit) { p.ExpiresAt = time.Now().Add(-time.Second) }},
		{"oversized lifetime", func(p *Permit) { p.ExpiresAt = time.Now().Add(time.Hour) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.change(&p)
			if p.Validate() == nil {
				t.Fatal("accepted invalid permit")
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
}
