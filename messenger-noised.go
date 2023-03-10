package main

import (
	"context"
	"fmt"

	"github.com/flynn/noise"
)

var _ Messenger = &NoisedMessenger{}

type NoiseIdentity interface {
	DHKey() noise.DHKey
	PresharedKeys() [][]byte
	Payload() []byte
	ValidatePeer(payload []byte) error
}

type NoisedMessenger struct {
	Messenger
	dec *noise.CipherState
	enc *noise.CipherState
	hsk *noise.HandshakeState
}

func NewNoisedMessengerI(ctx context.Context, ioc Messenger, id NoiseIdentity) (i *NoisedMessenger, err error) {
	ek, err := noise.DH25519.GenerateKeypair(nil)
	if err != nil {
		return i, fmt.Errorf("generate ephemeral key failed: %w", err)
	}

	psk := []byte{}
	if psks := id.PresharedKeys(); len(psks) > 0 {
		psk = psks[0]
	}
	nhsk, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:           noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256),
		Pattern:               noise.HandshakeIX,
		PresharedKey:          psk,
		PresharedKeyPlacement: 0,
		Initiator:             true,
		StaticKeypair:         id.DHKey(),
		EphemeralKeypair:      ek,
	})
	if err != nil {
		return i, fmt.Errorf("create noise handshake failed: %w", err)
	}

	// write
	msg, _, _, err := nhsk.WriteMessage(nil, id.Payload())
	if err != nil {
		return i, fmt.Errorf("write handshake message failed: %w", err)
	}
	err = ioc.Write(ctx, msg)
	if err != nil {
		return i, fmt.Errorf("write handshake message to io failed: %w", err)
	}

	// read
	msg, err = ioc.Read(ctx)
	if err != nil {
		return i, fmt.Errorf("read handshake message from io failed: %w", err)
	}
	payload, ki, kr, err := nhsk.ReadMessage(nil, msg)
	if err != nil {
		return i, fmt.Errorf("read handshake message failed: %w", err)
	}
	if ki == nil || kr == nil {
		return i, fmt.Errorf("no keypair created at the end of handshake")
	}
	if err := id.ValidatePeer(payload); err != nil {
		return i, fmt.Errorf("peer validation failed: %w", err)
	}

	return &NoisedMessenger{
		Messenger: ioc,
		hsk:       nhsk,
		enc:       ki,
		dec:       kr,
	}, nil
}

func NewNoisedMessengerR(ctx context.Context, ioc Messenger, id NoiseIdentity) (i *NoisedMessenger, err error) {
	ek, err := noise.DH25519.GenerateKeypair(nil)
	if err != nil {
		return i, fmt.Errorf("generate ephemeral key failed: %w", err)
	}

	psks := id.PresharedKeys()
	if len(psks) < 0 {
		psks = append(psks, []byte{})
	}
	var nhsk *noise.HandshakeState
	var payload []byte

	// read
	msg, err := ioc.Read(ctx)
	if err != nil {
		return i, fmt.Errorf("read handshake message from io failed: %w", err)
	}
	for _, psk := range psks {
		hsk, err := noise.NewHandshakeState(noise.Config{
			CipherSuite:           noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256),
			Pattern:               noise.HandshakeIX,
			PresharedKey:          psk,
			PresharedKeyPlacement: 0,
			Initiator:             false,
			StaticKeypair:         id.DHKey(),
			EphemeralKeypair:      ek,
		})
		if err != nil {
			return i, fmt.Errorf("create noise handshake failed: %w", err)
		}

		payload, _, _, err = hsk.ReadMessage(nil, msg)
		if err != nil {
			continue
		}

		nhsk = hsk
	}
	if nhsk == nil {
		return i, fmt.Errorf("reading initiator message failed, possibly due to unmatching PSK")
	}
	if err = id.ValidatePeer(payload); err != nil {
		return i, fmt.Errorf("cannot validate initiator payload: %w", err)
	}

	// write
	msg, ki, kr, err := nhsk.WriteMessage(nil, id.Payload())
	if err != nil {
		return i, fmt.Errorf("write handshake message failed: %w", err)
	}
	if ki == nil || kr == nil {
		return i, fmt.Errorf("no keypair created at the end of handshake")
	}
	err = ioc.Write(ctx, msg)
	if err != nil {
		return i, fmt.Errorf("write handshake message to io failed: %w", err)
	}

	return &NoisedMessenger{
		Messenger: ioc,
		hsk:       nhsk,
		enc:       kr,
		dec:       ki,
	}, nil
}

func (c *NoisedMessenger) Read(ctx context.Context) (b []byte, err error) {
	b, err = c.Messenger.Read(ctx)
	if err != nil {
		return
	}

	return c.dec.Decrypt(b[:0], nil, b)
}

func (c *NoisedMessenger) Write(ctx context.Context, b []byte) (err error) {
	b, err = c.enc.Encrypt(b[:0], nil, b)
	if err != nil {
		return
	}

	return c.Messenger.Write(ctx, b)
}

func (c *NoisedMessenger) Close() error {
	return c.Messenger.Close()
}
