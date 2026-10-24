// Package localtls authenticates the loopback PostgreSQL listener independently
// of daemon PIDs and ports. Its private identity never leaves the config directory.
package localtls

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/kamui-project/kamui-cli/internal/config"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Identity struct {
	ServerConfig    *tls.Config
	CertificateFile string
}

// LoadOrCreate uses a cross-process lock and one atomic identity file, so a
// concurrent CLI cannot observe mismatched keys and certificates or rotate trust.
func LoadOrCreate(ctx context.Context) (*Identity, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("database config directory must be private (chmod 700) and must not be a symlink")
	}
	lock := config.NewManagerWithPath(filepath.Join(dir, "db-local-tls"))
	release, err := lock.LockAuth(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	path := filepath.Join(dir, "db-local-identity.pem")
	body, err := readPrivateFile(path)
	if errors.Is(err, os.ErrNotExist) {
		body, err = newIdentity()
		if err != nil {
			return nil, err
		}
		if err = atomicWrite(path, body); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(body, body)
	if err != nil || len(pair.Certificate) != 1 {
		return nil, errors.New("invalid local database TLS identity")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, errors.New("invalid local database TLS certificate")
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || cert.CheckSignatureFrom(cert) != nil || cert.VerifyHostname("localhost") != nil || cert.VerifyHostname("127.0.0.1") != nil {
		return nil, errors.New("local database TLS identity is expired or invalid; restore a valid identity before connecting")
	}
	public := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	publicPath := filepath.Join(dir, "db-local-ca.pem")
	previous, err := readPrivateFile(publicPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !bytes.Equal(previous, public) {
		if err = atomicWrite(publicPath, public); err != nil {
			return nil, err
		}
	}
	return &Identity{ServerConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}, CertificateFile: publicPath}, nil
}

func newIdentity() ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Kamui local database listener"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(2, 0, 0), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	result := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return append(result, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})...), nil
}

func readPrivateFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("local TLS file %s must be a regular file accessible only to its owner", path)
	}
	return os.ReadFile(path)
}

func atomicWrite(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".db-local-tls-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
