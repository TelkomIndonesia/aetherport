package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	"github.com/flynn/noise"
	"golang.org/x/crypto/hkdf"
)

var _ NoiseIdentity = &Identity{}

type Identity struct {
	key     []byte
	cert    *AetherportCertificate
	caPool  *AetherportCAPool
	psk     [][]byte
	payload []byte

	peerCert *AetherportCertificate
}

func NewIdentity(key []byte, cert *AetherportCertificate, caPool *AetherportCAPool) (n *Identity, err error) {
	n = &Identity{
		key:    key,
		cert:   cert,
		caPool: caPool,
	}

	if err := n.computePresharedKeys(); err != nil {
		return nil, err
	}
	if err := n.computePayload(); err != nil {
		return nil, err
	}

	return
}

func (i *Identity) DHKey() noise.DHKey {
	return noise.DHKey{
		Private: i.key[:],
		Public:  i.cert.Details.PublicKey,
	}
}

func (i *Identity) computePresharedKeys() (err error) {
	pskGen := func(ca *AetherportCertificate) (psk []byte, err error) {
		psk = make([]byte, 32)
		_, err = io.ReadFull(hkdf.New(sha256.New, ca.Signature, nil, nil), psk)
		if err != nil {
			return nil, fmt.Errorf("cannot derive psk from ca signature: %w", err)
		}
		return
	}

	ica, err := i.caPool.GetCAForCert(i.cert)
	if err != nil {
		return fmt.Errorf("cannot get ca of the node certificate: %w", err)
	}

	i.psk = make([][]byte, 0, len(i.caPool.caMap))
	for _, ca := range i.caPool.caMap {
		psk, err := pskGen(ica)
		if err != nil {
			return fmt.Errorf("cannot derive psk from ca certificate: %w", err)
		}

		if ca == ica && len(i.psk) > 1 {
			i.psk[0], i.psk = psk, append(i.psk[:1], i.psk...)
		} else {
			i.psk = append(i.psk, psk)
		}
	}
	return
}

func (i *Identity) computePayload() (err error) {
	i.payload, err = i.cert.Marshal()
	if err != nil {
		return fmt.Errorf("can not marshal certificate: %w", err)
	}
	return
}

func (i *Identity) PresharedKeys() (keys [][]byte) {
	return i.psk
}

func (i *Identity) Payload() []byte {
	return i.payload
}

func (i *Identity) ValidatePeer(payload []byte) (err error) {
	c, err := UnmarshalAetherportCertificate(payload)
	if err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}

	ok, err := c.Verify(time.Now(), i.caPool)
	if err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}
	if !ok {
		return fmt.Errorf("untrusted certificate")
	}

	i.peerCert = c
	return
}
