// Package dbaccess connects local PostgreSQL clients to a tenant's PgCat.
// TLS runs from this CLI to PgCat, across the opaque WebSocket transport.
package dbaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Permit struct {
	Endpoint      string    `json:"endpoint"`
	Token         string    `json:"token"`
	ExpiresAt     time.Time `json:"expires_at"`
	TLSServerName string    `json:"tls_server_name"`
	TLSCACert     string    `json:"tls_ca_certificate"`
}

type Provider func(context.Context) (*Permit, error)

var pathPattern = regexp.MustCompile(`^/connect/[a-z0-9-]{1,64}/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// Validate rejects missing transport or database TLS configuration.
func (p *Permit) Validate() error { _, _, err := p.validate(); return err }

func (p *Permit) validate() (*url.URL, *tls.Config, error) {
	if p == nil {
		return nil, nil, errors.New("missing database access permit")
	}
	u, err := url.Parse(p.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || !pathPattern.MatchString(u.Path) ||
		p.Token == "" || len(p.Token) > 2048 || strings.ContainsAny(p.Token, "\r\n") ||
		time.Until(p.ExpiresAt) <= 0 || time.Until(p.ExpiresAt) > 10*time.Minute || p.TLSServerName == "" {
		return nil, nil, errors.New("invalid database access permit")
	}
	// The authenticated API supplies the tenant name and public CA. No system
	// trust fallback or insecure TLS mode is permitted for the database stream.
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(p.TLSCACert)) {
		return nil, nil, errors.New("invalid database TLS CA")
	}
	u.Scheme = "wss"
	return u, &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: p.TLSServerName}, nil
}

// Serve keeps the caller's loopback listener until cancellation. Each DB
// session gets its own authenticated stream and renews without reconnecting SQL.
func Serve(ctx context.Context, listener net.Listener, provider Provider, localTLS *tls.Config, report func(error)) error {
	if localTLS == nil || len(localTLS.Certificates) == 0 {
		return errors.New("verified local database TLS is required")
	}
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("database listener must use a loopback address")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	var sessions sync.WaitGroup
	defer sessions.Wait()
	for {
		local, err := listener.Accept()
		if err != nil {
			cancel()
			if ctx.Err() != nil && errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		sessions.Add(1)
		go func() {
			defer sessions.Done()
			defer local.Close()
			if err := forwardWithDialer(ctx, local, provider, &websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}, localTLS); err != nil && ctx.Err() == nil && report != nil {
				report(err)
			}
		}()
	}
}

func forwardWithDialer(ctx context.Context, local net.Conn, provider Provider, dialer *websocket.Dialer, localTLS *tls.Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rawLocal := local
	stopLocal := context.AfterFunc(ctx, func() { rawLocal.Close() })
	defer stopLocal()
	// Require PostgreSQL's SSLRequest and authenticate this loopback server
	// before reading startup credentials. Plain clients fail before any API call.
	local.SetDeadline(time.Now().Add(10 * time.Second))
	header := make([]byte, 8)
	encrypted := false
	for attempt := 0; ; attempt++ {
		if _, err := io.ReadFull(local, header); err != nil {
			return nil
		}
		code := binary.BigEndian.Uint32(header[4:])
		if binary.BigEndian.Uint32(header[:4]) != 8 || (code != 80877103 && code != 80877104) {
			break
		}
		if attempt >= 2 || encrypted {
			return errors.New("too many local encryption requests")
		}
		if code == 80877103 && localTLS != nil {
			if _, err := local.Write([]byte("S")); err != nil {
				return err
			}
			secure := tls.Server(local, localTLS)
			if err := secure.HandshakeContext(ctx); err != nil {
				return errors.New("local database TLS handshake failed")
			}
			local = secure
			encrypted = true
		} else {
			if _, err := local.Write([]byte("N")); err != nil {
				return err
			}
		}
	}
	if localTLS != nil && !encrypted {
		return errors.New("local database connection requires TLS; configure verify-full with db-local-ca.pem")
	}
	local.SetDeadline(time.Time{})
	local.SetReadDeadline(time.Time{})
	permit, err := provider(ctx)
	if err != nil {
		return errors.New("database access permit unavailable")
	}
	endpoint, tlsConfig, err := permit.validate()
	if err != nil {
		return err
	}
	ws, response, err := dialer.DialContext(ctx, endpoint.String(), http.Header{"Authorization": {"Bearer " + permit.Token}})
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		return errors.New("database tunnel connection rejected or unavailable")
	}
	defer ws.Close()
	ws.SetReadLimit(64 << 10)
	stream := &websocketStream{ws: ws}
	stopStream := context.AfterFunc(ctx, func() { ws.Close() })
	defer stopStream()
	// PostgreSQL's SSLRequest must precede the TLS handshake.
	stream.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := stream.Write([]byte{0, 0, 0, 8, 4, 210, 22, 47}); err != nil {
		return err
	}
	var answer [1]byte
	if _, err := io.ReadFull(stream, answer[:]); err != nil || answer[0] != 'S' {
		return errors.New("database does not support TLS")
	}
	upstream := tls.Client(stream, tlsConfig)
	if err := upstream.HandshakeContext(ctx); err != nil {
		return errors.New("database TLS verification failed")
	}
	stream.SetDeadline(time.Time{})
	if _, err := upstream.Write(header); err != nil {
		return err
	}
	go renew(ctx, cancel, stream, permit, provider)
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, local); done <- struct{}{} }()
	go func() { io.Copy(local, upstream); done <- struct{}{} }()
	<-done
	cancel()
	local.Close()
	ws.Close()
	<-done
	return nil
}

func renew(ctx context.Context, cancel context.CancelFunc, stream *websocketStream, current *Permit, provider Provider) {
	for {
		remaining := time.Until(current.ExpiresAt)
		if remaining <= 0 {
			cancel()
			return
		}
		timer := time.NewTimer(remaining * 4 / 5)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		requestCtx, stop := context.WithDeadline(ctx, current.ExpiresAt)
		next, err := provider(requestCtx)
		stop()
		if err == nil {
			_, _, err = next.validate()
		}
		if err != nil || next.Endpoint != current.Endpoint || next.TLSServerName != current.TLSServerName ||
			next.TLSCACert != current.TLSCACert || !next.ExpiresAt.After(current.ExpiresAt) {
			// No unbounded use of a revoked permit; the gateway also enforces
			// this deadline independently of a cooperating client.
			timer.Reset(max(time.Until(current.ExpiresAt), 0))
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
				cancel()
			}
			return
		}
		body, _ := json.Marshal(struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}{"renew", next.Token})
		if stream.writeMessage(websocket.TextMessage, body) != nil {
			cancel()
			return
		}
		current = next
	}
}

type websocketStream struct {
	ws      *websocket.Conn
	reader  io.Reader
	writeMu sync.Mutex
}

func (s *websocketStream) Read(p []byte) (int, error) {
	for {
		if s.reader == nil {
			kind, reader, err := s.ws.NextReader()
			if err != nil {
				return 0, err
			}
			if kind != websocket.BinaryMessage {
				return 0, errors.New("unexpected tunnel message")
			}
			s.reader = reader
		}
		n, err := s.reader.Read(p)
		if err == io.EOF {
			s.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}
func (s *websocketStream) Write(p []byte) (int, error) {
	if err := s.writeMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
func (s *websocketStream) writeMessage(kind int, body []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return s.ws.WriteMessage(kind, body)
}
func (s *websocketStream) Close() error         { return s.ws.Close() }
func (s *websocketStream) LocalAddr() net.Addr  { return s.ws.UnderlyingConn().LocalAddr() }
func (s *websocketStream) RemoteAddr() net.Addr { return s.ws.UnderlyingConn().RemoteAddr() }
func (s *websocketStream) SetDeadline(t time.Time) error {
	s.SetWriteDeadline(t)
	return s.SetReadDeadline(t)
}
func (s *websocketStream) SetReadDeadline(t time.Time) error { return s.ws.SetReadDeadline(t) }
func (s *websocketStream) SetWriteDeadline(t time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.ws.SetWriteDeadline(t)
}
