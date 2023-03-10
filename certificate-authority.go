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
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrExpired       = errors.New("certificate is expired")
	ErrNotCA         = errors.New("certificate is not a CA")
	ErrNotSelfSigned = errors.New("certificate is not self-signed")
)

type AetherportCAPool struct {
	caMap         map[string]*AetherportCertificate
	certBlocklist map[string]struct{}
}

func NewCAPool() *AetherportCAPool {
	ca := AetherportCAPool{
		caMap:         make(map[string]*AetherportCertificate),
		certBlocklist: make(map[string]struct{}),
	}

	return &ca
}

func NewCAPoolFromPEM(caPEMs []byte) (*AetherportCAPool, error) {
	pool := NewCAPool()
	var err error
	var expired bool
	for {
		caPEMs, err = pool.AddCACertificateFromPEM(caPEMs)
		if errors.Is(err, ErrExpired) {
			expired = true
			err = nil
		}
		if err != nil {
			return nil, err
		}
		if len(caPEMs) == 0 || strings.TrimSpace(string(caPEMs)) == "" {
			break
		}
	}

	if expired {
		return pool, ErrExpired
	}

	return pool, nil
}

func (ncp *AetherportCAPool) AddCACertificateFromPEM(pemBytes []byte) ([]byte, error) {
	c, pemBytes, err := UnmarshalAetherportCertificateFromPEM(pemBytes)
	if err != nil {
		return pemBytes, err
	}

	if !c.Details.IsCA {
		return pemBytes, fmt.Errorf("%s: %w", c.Details.Name, ErrNotCA)
	}

	if !c.CheckSignature(c.Details.PublicKey) {
		return pemBytes, fmt.Errorf("%s: %w", c.Details.Name, ErrNotSelfSigned)
	}

	sum, err := c.Sha256Sum()
	if err != nil {
		return pemBytes, fmt.Errorf("could not calculate shasum for provided CA; error: %s; %s", err, c.Details.Name)
	}

	ncp.caMap[sum] = c
	if c.Expired(time.Now()) {
		return pemBytes, fmt.Errorf("%s: %w", c.Details.Name, ErrExpired)
	}

	return pemBytes, nil
}

func (acp *AetherportCAPool) BlocklistFingerprint(fp string) (err error) {
	acp.certBlocklist[fp] = struct{}{}
	return
}

func (acp *AetherportCAPool) IsBlocklisted(ac *AetherportCertificate) bool {
	h, err := ac.Sha256Sum()
	if err != nil {
		return true
	}

	if _, ok := acp.certBlocklist[h]; ok {
		return true
	}

	return false
}

func (acp *AetherportCAPool) GetCAForCert(ac *AetherportCertificate) (*AetherportCertificate, error) {
	if ac.Details.Issuer == "" {
		return nil, fmt.Errorf("no issuer in certificate")
	}

	signer, ok := acp.caMap[ac.Details.Issuer]
	if !ok {
		return nil, fmt.Errorf("could not find ca for the certificate")
	}

	return signer, nil
}
