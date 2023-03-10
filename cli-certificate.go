package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"time"

	"github.com/flynn/noise"
)

type CliCertificate struct {
	Generate   CliCertificateGenerate   `cmd:"" name:"generate" help:"generate certificate."`
	GenerateCA CliCertificateGenerateCA `cmd:"" name:"generate-ca" help:"generate CA certificate."`
	ReSign     CliCertificateReSign     `cmd:"" name:"re-sign" help:"re-sign previously generated certificate."`
}

type CliCertificateGenerate struct {
	KeyFile    string `name:"key" help:"path to save the generated private key. If not specified, it will use <certificate name>-key.pem"`
	CertFile   string `name:"cert" help:"path to save the generated certificate. If not specified, it will use <certificate name>-cert.pem"`
	CaKeyFile  string `name:"cakey" default:"cakey.pem" help:"path to existing ca private key. If CA key and cert does not exist, it will be created using default settings."`
	CaCertFile string `name:"cacert" default:"cacert.pem" help:"path to existing ca certificate. If CA key and cert does not exist, it will be created using default settings."`

	Name     string        `name:"name" required:"" placeholder:"\"certificate name\"" help:"The name represented by this certificate"`
	Labels   []label       `name:"label" help:"List of arbritrary key=value"`
	Duration time.Duration `name:"duration" help:"The duration the certificate will be active upon its creation."`
}

func (c *CliCertificateGenerate) Run(ctx context.Context) (err error) {
	_, errk := os.Stat(c.CaKeyFile)
	_, errc := os.Stat(c.CaCertFile)
	if errk != nil && errc != nil {
		var dur time.Duration
		if c.Duration == 0 {
			dur = 8760 * time.Hour
		}
		g := &CliCertificateGenerateCA{
			CakeyFile:  c.CaKeyFile,
			CacertFile: c.CaCertFile,
			Name:       "aetherport.ca",
			Duration:   dur,
		}
		if err := g.Run(ctx); err != nil {
			return fmt.Errorf("generating default ca failed: %w", err)
		}
	}

	caKeyB, err := os.ReadFile(c.CaKeyFile)
	if err != nil {
		return fmt.Errorf("open ca key failed: %w", err)
	}
	caKey, _, err := UnmarshalEd25519PrivateKey(caKeyB)
	if err != nil {
		return fmt.Errorf("invalid ca key: %w", err)
	}
	caCertB, err := os.ReadFile(c.CaCertFile)
	if err != nil {
		return fmt.Errorf("open ca cert failed: %w", err)
	}
	caCert, _, err := UnmarshalAetherportCertificateFromPEM(caCertB)
	if err != nil {
		return fmt.Errorf("invalid ca certificate: %w", err)
	}

	sk, err := noise.DH25519.GenerateKeypair(nil)
	if err != nil {
		return fmt.Errorf("generate key failed: %w", err)
	}

	now := time.Now()
	dur := c.Duration
	if dur == 0 {
		dur = caCert.Details.NotAfter.Sub(now) - time.Second
	}
	cert := &AetherportCertificate{
		Details: AetherportCertificateDetails{
			Name:      c.Name,
			Labels:    c.Labels,
			NotBefore: now,
			NotAfter:  now.Add(dur),
			IsCA:      true,
			PublicKey: sk.Public,
		},
	}
	if err = cert.Sign(caKey, caCert); err != nil {
		return fmt.Errorf("sign ca certificate failed: %w", err)
	}

	fkn := c.KeyFile
	if fkn == "" {
		fkn = c.Name + "-key.pem"
	}
	fk, err := os.OpenFile(fkn, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("preparing file for writing private key failed: %w", err)
	}
	defer fk.Close()
	if _, err := fk.Write(MarshalX25519PrivateKey(sk.Private)); err != nil {
		return fmt.Errorf("write ca private key failed: %w", err)
	}

	fcn := c.CertFile
	if fcn == "" {
		fcn = c.Name + "-cert.pem"
	}
	fc, err := os.OpenFile(fcn, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("preparing file for writing certificate failed: %w", err)
	}
	defer fc.Close()
	b, err := MarshalAetherportCertificateToPEM(cert)
	if err != nil {
		return fmt.Errorf("marshal ca certificate failed: %w", err)
	}
	if _, err := fc.Write(b); err != nil {
		return fmt.Errorf("write ca certificate failed: %w", err)
	}
	return
}

type CliCertificateGenerateCA struct {
	CakeyFile  string `name:"cakey" default:"cakey.pem" help:"path to save the generated ca private key"`
	CacertFile string `name:"cacert" default:"cacert.pem" help:"path to save the generated ca certificate"`

	Name     string        `name:"name" required:"" help:"The name represented by this certificate"`
	Labels   []label       `name:"label" help:"List of arbritrary key=value"`
	Duration time.Duration `name:"duration" default:"8760h0m0s" help:"The duration the certificate will be active upon its creation."`
}

func (c *CliCertificateGenerateCA) Run(ctx context.Context) (err error) {
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("generate key pair failed: %w", err)
	}

	now := time.Now()
	cert := &AetherportCertificate{
		Details: AetherportCertificateDetails{
			Name:      c.Name,
			Labels:    c.Labels,
			NotBefore: now,
			NotAfter:  now.Add(c.Duration),
			IsCA:      true,
			PublicKey: pub,
		},
	}
	if err = cert.Sign(key, nil); err != nil {
		return fmt.Errorf("sign ca certificate failed: %w", err)
	}

	fk, err := os.OpenFile(c.CakeyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("preparing file for writing ca private key failed: %w", err)
	}
	defer fk.Close()
	if _, err := fk.Write(MarshalEd25519PrivateKey(key)); err != nil {
		return fmt.Errorf("write ca private key failed: %w", err)
	}

	fc, err := os.OpenFile(c.CacertFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("preparing file for writing ca certificate failed: %w", err)
	}
	defer fc.Close()
	b, err := MarshalAetherportCertificateToPEM(cert)
	if err != nil {
		return fmt.Errorf("marshal ca certificate failed: %w", err)
	}
	if _, err := fc.Write(b); err != nil {
		return fmt.Errorf("write ca certificate failed: %w", err)
	}

	return
}

type CliCertificateReSign struct {
	CertFile   string `name:"cert" default:"cert.pem" help:"path to existing certificate"`
	CaKeyFile  string `name:"cakey" default:"cakey.pem" help:"path to existing ca private key"`
	CaCertFile string `name:"cacert" default:"cacert.pem" help:"path to existing ca certificate"`

	Name     *string        `name:"name" required:"" help:"The name represented by this certificate"`
	Labels   []label        `name:"label" help:"List of arbritrary key=value"`
	Duration *time.Duration `name:"duration" help:"The duration the certificate will be active upon its creation."`
}

func (c *CliCertificateReSign) Run(ctx context.Context) (err error) {
	b, err := os.ReadFile(c.CaKeyFile)
	if err != nil {
		return fmt.Errorf("open CA key failed: %w", err)
	}
	caKey, _, err := UnmarshalEd25519PrivateKey(b)
	if err != nil {
		return fmt.Errorf("unmarshall ca private key failed: %w", err)
	}

	b, err = os.ReadFile(c.CaCertFile)
	if err != nil {
		return fmt.Errorf("open CA key failed: %w", err)
	}
	caCert, _, err := UnmarshalAetherportCertificateFromPEM(b)
	if err != nil {
		return fmt.Errorf("unmarshall ca certificate failed: %w", err)
	}

	b, err = os.ReadFile(c.CertFile)
	if err != nil {
		return fmt.Errorf("open certificate failed: %w", err)
	}
	cert, _, err := UnmarshalAetherportCertificateFromPEM(b)
	if err != nil {
		return fmt.Errorf("unmarshall certificate failed: %w", err)
	}

	if c.Name != nil {
		cert.Details.Name = *c.Name
	}
	if c.Duration == nil {
		dur := cert.Details.NotAfter.Sub(cert.Details.NotBefore)
		c.Duration = &dur
	}
	cert.Details.NotBefore = time.Now()
	cert.Details.NotAfter = cert.Details.NotBefore.Add(*c.Duration)
	if len(c.Labels) > 0 {
		cert.Details.Labels = c.Labels
	}

	if err := cert.Sign(caKey, caCert); err != nil {
		return fmt.Errorf("re-sign certificate failed: %w", err)
	}
	b, err = MarshalAetherportCertificateToPEM(cert)
	if err != nil {
		return fmt.Errorf("marshal certificate failed: %w", err)
	}
	if err = os.WriteFile(c.CertFile, b, 0644); err != nil {
		return fmt.Errorf("write new certificate failed: %w", err)
	}
	return
}
