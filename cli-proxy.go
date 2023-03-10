package main

import (
	"context"

	"github.com/pion/webrtc/v3"
)

type CliProxy struct {
	Forwards []string `name:"forward" short:"f" placeholder:"<local-ip>:<local-port>:<remote-ip>:<remote-port>" help:"List of local to remote endpoint mapping."`
	Allows   []string `name:"allow" short:"w" placeholder:"<ip>:<port>" help:"List of remote endpoints the egress is allowed to connect to."`

	SignalType string `name:"signal-type" short:"t" default:"tty" enum:"tty,aetherlight" help:"Type of signaling. Available options are 'tty' or 'aetherlight'"`

	AetherlightBaseURL    string `name:"aetherlight-base-url" help:"URL to connect to aetherlight as ingress."`
	AetherlightIngressURL string `name:"aetherlight-ingress-url" help:"URL to connect to ingress connected to aetherlight."`

	KeyFile    string `name:"key"  help:"Path to key file. Not used in tty signaling."`
	CertFile   string `name:"cert" help:"Path to certificate file. Not used in tty signaling."`
	CaCertFile string `name:"cacert" help:"Path to file containing one or more trusted CA certificate. It must contain CA certificate that is used to sign the certificate specified in '--cert' flag. Not used in tty signaling."`

	ICEServers []string `name:"ice-server" help:"List of ICE servers to use for discovering addresses." placeholder:"[stun|stuns|turn|turns]://<host>:<port>"`
}

func (c *CliProxy) Run(ctx context.Context) (err error) {
	switch c.SignalType {
	case "tty":
		return c.runTTY(ctx)
	case "aetherlight":
		return c.runAetherlight(ctx)
	}
	return
}

func (c *CliProxy) newWebRTCPeerConnection() (peer *webrtc.PeerConnection, err error) {
	urls := c.ICEServers
	if len(urls) == 0 {
		urls = defaultSTUNs
	}

	s := webrtc.SettingEngine{}
	s.DetachDataChannels()
	a := webrtc.NewAPI(webrtc.WithSettingEngine(s))
	peer, err = a.NewPeerConnection(
		webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: urls},
			},
		},
	)
	return
}
