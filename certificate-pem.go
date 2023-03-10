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
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
)

const (
	AetherportCertBanner    = "AETHERPORT CERTIFICATE"
	X25519PrivateKeyBanner  = "X25519 PRIVATE KEY"
	X25519PublicKeyBanner   = "X25519 PUBLIC KEY"
	Ed25519PrivateKeyBanner = "ED25519 PRIVATE KEY"
	Ed25519PublicKeyBanner  = "ED25519 PUBLIC KEY"
)

// Aetherport certificate
func UnmarshalAetherportCertificateFromPEM(b []byte) (*AetherportCertificate, []byte, error) {
	p, r := pem.Decode(b)
	if p == nil {
		return nil, r, fmt.Errorf("input did not contain a valid PEM encoded block")
	}
	if p.Type != AetherportCertBanner {
		return nil, r, fmt.Errorf("bytes did not contain a proper aetherport certificate banner: %s", AetherportCertBanner)
	}
	ac, err := UnmarshalAetherportCertificate(p.Bytes)
	return ac, r, err
}

func MarshalAetherportCertificateToPEM(ac *AetherportCertificate) ([]byte, error) {
	b, err := ac.Marshal()
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: AetherportCertBanner, Bytes: b}), nil
}

// ed25519
func MarshalEd25519PrivateKey(key ed25519.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: Ed25519PrivateKeyBanner, Bytes: key})
}
func UnmarshalEd25519PrivateKey(b []byte) (ed25519.PrivateKey, []byte, error) {
	return unmarshalPEM(b, Ed25519PrivateKeyBanner, ed25519.PrivateKeySize)
}

func MarshalEd25519PublicKey(key ed25519.PublicKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: Ed25519PublicKeyBanner, Bytes: key})
}
func UnmarshalEd25519PublicKey(b []byte) (ed25519.PublicKey, []byte, error) {
	return unmarshalPEM(b, Ed25519PublicKeyBanner, ed25519.PublicKeySize)
}

// x25519
func MarshalX25519PrivateKey(b []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: X25519PrivateKeyBanner, Bytes: b})
}
func UnmarshalX25519PrivateKey(b []byte) ([]byte, []byte, error) {
	return unmarshalPEM(b, X25519PrivateKeyBanner, x25519KeyLen)
}

func MarshalX25519PublicKey(b []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: X25519PublicKeyBanner, Bytes: b})
}
func UnmarshalX25519PublicKey(b []byte) ([]byte, []byte, error) {
	return unmarshalPEM(b, X25519PublicKeyBanner, x25519KeyLen)
}

func unmarshalPEM(b []byte, ty string, l int) ([]byte, []byte, error) {
	k, r := pem.Decode(b)
	if k == nil {
		return nil, r, fmt.Errorf("input did not contain a valid PEM encoded block")
	}
	if k.Type != ty {
		return nil, r, fmt.Errorf("bytes did not contain a proper banner: %s", ty)
	}
	if len(k.Bytes) != l {
		return nil, r, fmt.Errorf("bytes length is not equal proper length (%d of %d)", len(k.Bytes), l)
	}

	return k.Bytes, r, nil
}
