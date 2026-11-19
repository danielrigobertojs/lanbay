// SPDX-License-Identifier: Apache-2.0
package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

func EnsureCertificate(certFile, keyFile string) (string, error) {
	if _, e := os.Stat(certFile); e == nil {
		if _, e = os.Stat(keyFile); e == nil {
			return certFile, nil
		}
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return "", e
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		return "", e
	}
	host, _ := os.Hostname()
	notBefore := time.Now().Add(-time.Minute)
	template := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Lanbay"}, NotBefore: notBefore, NotAfter: notBefore.Add(824 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{host, host + ".local", "localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}}
	template.IPAddresses = append(template.IPAddresses, interfaces()...)
	der, e := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if e != nil {
		return "", fmt.Errorf("create certificate: %w", e)
	}
	certOut, e := os.OpenFile(certFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		return "", e
	}
	defer certOut.Close()
	if e = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); e != nil {
		return "", e
	}
	keyDER, e := x509.MarshalECPrivateKey(key)
	if e != nil {
		return "", e
	}
	keyOut, e := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		return "", e
	}
	defer keyOut.Close()
	if e = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); e != nil {
		return "", e
	}
	return certFile, nil
}
func interfaces() []net.IP {
	addrs, e := net.InterfaceAddrs()
	if e != nil {
		return nil
	}
	var ips []net.IP
	for _, a := range addrs {
		if ip, _, e := net.ParseCIDR(a.String()); e == nil && !ip.IsLoopback() {
			ips = append(ips, ip)
		}
	}
	return ips
}
