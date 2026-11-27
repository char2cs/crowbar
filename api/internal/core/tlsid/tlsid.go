package tlsid

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Identity struct {
	Certificate tls.Certificate
	Fingerprint string
}

const (
	certName = "cert.pem"
	keyName  = "key.pem"
	validFor = 10 * 365 * 24 * time.Hour
)

func Load(dir string) (*Identity, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("tlsid: create dir: %w", err)
	}
	//nolint:gosec // the TLS identity directory must not be group/world readable
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("tlsid: restrict dir: %w", err)
	}
	certPath := filepath.Join(dir, certName)
	keyPath := filepath.Join(dir, keyName)

	if missing(certPath) || missing(keyPath) {
		if err := generate(certPath, keyPath); err != nil {
			return nil, err
		}
	}

	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("tlsid: this host's TLS identity cannot be loaded from %s (%w). It is not "+
			"regenerated, because paired clients pin it: restore %s and %s, or delete both to create a new "+
			"identity, which every paired client will see as an identity change and must re-pair",
			dir, err, certName, keyName)
	}
	if len(pair.Certificate) == 0 {
		return nil, fmt.Errorf("tlsid: keypair carries no certificate")
	}
	return &Identity{Certificate: pair, Fingerprint: FingerprintOf(pair.Certificate[0])}, nil
}

func FingerprintOf(der []byte) string {
	sum := sha256.Sum256(der)
	full := hex.EncodeToString(sum[:])
	groups := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		groups = append(groups, full[i*4:(i+1)*4])
	}
	return "SHA256:" + strings.Join(groups, " ")
}

func generate(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("tlsid: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("tlsid: serial: %w", err)
	}

	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "crowbar"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"crowbar", "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("tlsid: create certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("tlsid: marshal key: %w", err)
	}

	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return err
	}
	return writePEM(certPath, "CERTIFICATE", der, 0o600)
}

func missing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) (err error) {
	name := filepath.Base(path)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+name+".tmp-*")
	if err != nil {
		return fmt.Errorf("tlsid: write %s: %w", name, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmp.Name()))
		}
	}()

	buf := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	_, err = tmp.Write(buf)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("tlsid: write %s: %w", name, err)
	}
	return nil
}
