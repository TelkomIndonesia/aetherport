//go:generate protoc --go_out=. --go_opt=paths=source_relative certificate.proto

// MIT License

// Copyright (c) 2018-2019 Slack Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:

// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
// LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
// OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
// WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
package main

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/net/idna"

	"golang.org/x/crypto/curve25519"
	"google.golang.org/protobuf/proto"
)

const x25519KeyLen = 32

type AetherportCertificate struct {
	Details   AetherportCertificateDetails
	Signature []byte
}

type AetherportCertificateDetails struct {
	Name      string
	Labels    []Label
	NotBefore time.Time
	NotAfter  time.Time
	PublicKey []byte
	IsCA      bool
	Issuer    string
}

func UnmarshalAetherportCertificate(b []byte) (ac *AetherportCertificate, err error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("nil byte")
	}

	var acr AetherportCertificateRaw
	if err := proto.Unmarshal(b, &acr); err != nil {
		return nil, fmt.Errorf("invalid certificate: %w", err)
	}

	if acr.Details == nil {
		return nil, fmt.Errorf("the certificate does not contain any details")
	}

	ac = &AetherportCertificate{
		Details: AetherportCertificateDetails{
			Name:      acr.Details.Name,
			Labels:    make([]Label, len(acr.Details.Labels)),
			NotBefore: time.Unix(0, acr.Details.NotBefore),
			NotAfter:  time.Unix(0, acr.Details.NotAfter),
			PublicKey: make([]byte, len(acr.Details.PublicKey)),
			IsCA:      acr.Details.IsCA,
		},
		Signature: make([]byte, len(acr.Signature)),
	}
	copy(ac.Signature, acr.Signature)
	ac.Details.Issuer = hex.EncodeToString(acr.Details.Issuer)

	for _, s := range acr.Details.Labels {
		l, err := labelFromString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid labels was found (%s): %w", s, err)
		}
		ac.Details.Labels = append(ac.Details.Labels, l)
	}

	if len(acr.Details.PublicKey) < x25519KeyLen {
		return nil, fmt.Errorf("public key was fewer than %d bytes; %v", x25519KeyLen, len(acr.Details.PublicKey))
	}
	copy(ac.Details.PublicKey, acr.Details.PublicKey)

	if _, err := idna.Lookup.ToASCII(ac.Details.Name); err != nil {
		return nil, fmt.Errorf("certificate name does not comply with IDNA2018: %w", err)
	}
	return
}

func (ac *AetherportCertificate) Marshal() ([]byte, error) {
	dr, err := ac.getDetailsRaw()
	if err != nil {
		return nil, err
	}

	cr := &AetherportCertificateRaw{
		Details:   dr,
		Signature: ac.Signature,
	}
	return proto.Marshal(cr)
}

func (ac *AetherportCertificate) getDetailsRaw() (dr *AetherportCertificateDetailsRaw, err error) {
	dr = &AetherportCertificateDetailsRaw{
		Name:      ac.Details.Name,
		Labels:    make([]string, len(ac.Details.Labels)),
		NotBefore: ac.Details.NotBefore.UnixNano(),
		NotAfter:  ac.Details.NotAfter.UnixNano(),
		PublicKey: make([]byte, len(ac.Details.PublicKey)),
		IsCA:      ac.Details.IsCA,
	}
	copy(dr.PublicKey, ac.Details.PublicKey[:])
	for _, l := range ac.Details.Labels {
		dr.Labels = append(dr.Labels, l.String())
	}
	if dr.Issuer, err = hex.DecodeString(ac.Details.Issuer); err != nil {
		return nil, fmt.Errorf("invalid issuer (%s): %w", ac.Details.Issuer, err)
	}
	if _, err := idna.Lookup.ToASCII(dr.Name); err != nil {
		return nil, fmt.Errorf("certificate name does not comply with IDNA2018: %w", err)
	}

	return
}

func (ac *AetherportCertificate) VerifyPrivateKey(key []byte) (err error) {
	switch ac.Details.IsCA {
	case true:
		if len(key) != ed25519.PrivateKeySize {
			return fmt.Errorf("key was not 64 bytes, is invalid ed25519 private key")
		}

		if !ed25519.PublicKey(ac.Details.PublicKey).Equal(ed25519.PrivateKey(key).Public()) {
			return fmt.Errorf("public key in cert and private key supplied don't match")
		}

	case false:
		pub, err := curve25519.X25519(key, curve25519.Basepoint)
		if err != nil {
			return err
		}
		if !bytes.Equal(pub, ac.Details.PublicKey) {
			return fmt.Errorf("public key in cert and private key supplied don't match")
		}
	}

	return
}

func (ac *AetherportCertificate) Sign(key ed25519.PrivateKey, cert *AetherportCertificate) (err error) {
	if cert != nil {
		ac.Details.Issuer, err = cert.Sha256Sum()
		if err != nil {
			return
		}
	}

	r, err := ac.getDetailsRaw()
	if err != nil {
		return
	}
	b, err := proto.Marshal(r)
	if err != nil {
		return err
	}

	sig, err := key.Sign(rand.Reader, b, crypto.Hash(0))
	if err != nil {
		return err
	}
	ac.Signature = sig
	return nil
}

func (ac *AetherportCertificate) Verify(t time.Time, acp *AetherportCAPool) (bool, error) {
	if acp.IsBlocklisted(ac) {
		return false, fmt.Errorf("certificate has been blocked")
	}

	signer, err := acp.GetCAForCert(ac)
	if err != nil {
		return false, err
	}

	if signer.Expired(t) {
		return false, fmt.Errorf("root certificate is expired")
	}

	if ac.Expired(t) {
		return false, fmt.Errorf("certificate is expired")
	}

	if !ac.CheckSignature(signer.Details.PublicKey) {
		return false, fmt.Errorf("certificate signature did not match")
	}

	if err := ac.CheckRootConstrains(signer); err != nil {
		return false, err
	}

	return true, nil
}

func (ac *AetherportCertificate) CheckSignature(key ed25519.PublicKey) bool {
	r, err := ac.getDetailsRaw()
	if err != nil {
		return false
	}

	b, err := proto.Marshal(r)
	if err != nil {
		return false
	}

	return ed25519.Verify(key, b, ac.Signature)
}

func (nc *AetherportCertificate) CheckRootConstrains(signer *AetherportCertificate) (err error) {
	if signer.Details.NotAfter.Before(nc.Details.NotAfter) {
		return fmt.Errorf("certificate expires after signing certificate")
	}

	if signer.Details.NotBefore.After(nc.Details.NotBefore) {
		return fmt.Errorf("certificate is valid before the signing certificate")
	}

	return
}

func (ac *AetherportCertificate) Sha256Sum() (string, error) {
	b, err := ac.Marshal()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (ac *AetherportCertificate) Expired(t time.Time) bool {
	return ac.Details.NotBefore.After(t) || ac.Details.NotAfter.Before(t)
}
