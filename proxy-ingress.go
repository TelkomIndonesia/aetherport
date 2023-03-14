package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/xtaci/smux"
)

type IngressProxy struct {
	signal        SignalIngress
	signalTimeout time.Duration
	peer          *webrtc.PeerConnection
	epAuth        EndpointAuthorizer
}

func (igp *IngressProxy) Start(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)

	igp.peer.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		switch pcs {
		case webrtc.PeerConnectionStateFailed:
			err = fmt.Errorf("peer connection failed")
			igp.Stop()
		case webrtc.PeerConnectionStateDisconnected:
			igp.Stop()
		case webrtc.PeerConnectionStateClosed:
			cancel()
		}
	})

	sctx := ctx
	if igp.signalTimeout > 0 {
		var scancel context.CancelFunc
		sctx, scancel = context.WithTimeout(ctx, igp.signalTimeout)
		defer scancel()
	}

	igp.createTunnelsListener(sctx)

	offer, err := igp.signal.RecvOffer(sctx)
	if err != nil {
		return fmt.Errorf("receive offer failed: %w", err)
	}
	err = igp.peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer,
	})
	if err != nil {
		return fmt.Errorf("set remote description failed: %w", err)
	}

	answer, err := igp.peer.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer failed: %w", err)
	}
	if err = igp.peer.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local description failed: %w", err)
	}

	iceDone := gatherICE(sctx, igp.peer, igp.signal)

	if err := igp.signal.SendAnswer(sctx, igp.peer.LocalDescription().SDP); err != nil {
		return fmt.Errorf("send answer failed: %w", err)
	}

	chanRecv(sctx, iceDone)
	igp.signal.Close()
	<-ctx.Done()
	igp.Stop()

	return
}

func (igp *IngressProxy) createTunnelsListener(ctx context.Context) {
	igp.peer.OnDataChannel(func(dc *webrtc.DataChannel) {
		if ctx.Err() != nil {
			return
		}

		if dc.Label() == "" {
			return
		}

		ep, err := EndpointFromString(dc.Label())
		if err != nil {
			log.Printf("got invalid endpoint: %s: %s\n", dc.Label(), err)
			dc.Close()
			return
		}

		if igp.epAuth != nil {
			if ok, err := igp.epAuth(ep); !ok || err != nil {
				if err != nil {
					log.Printf("error when authorizing endpoint: %s: %s", dc.Label(), err)
				} else {
					log.Printf("unallowed endpoint: %s", dc.Label())
				}
				dc.Close()
				return
			}
		}

		log.Println("got data channel: ", dc.Label())
		go func() {
			err := igp.createTunnel(ctx, dc, ep)
			if err != nil {
				log.Println("create tunnel failed: ", err)
			}
		}()
	})
}

func (igp *IngressProxy) createTunnel(ctx context.Context, dc *webrtc.DataChannel, ep Endpoint) (err error) {
	defer dc.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dcc, err := NewDataChannelConn(ctx, dc)
	if err != nil {
		return fmt.Errorf("open datachannel error: %w", err)
	}
	defer dcc.Close()

	session, err := smux.Server(dcc, nil)
	if err != nil {
		return fmt.Errorf("open session error: %w", err)
	}
	defer session.Close()

	for {
		if ctx.Err() != nil {
			return
		}

		stream, err := session.AcceptStream()
		if err != nil {
			log.Println("ingress: accept stream error: ", err)
			continue
		}

		conn, err := net.Dial("tcp", ep.remote)
		if err != nil {
			log.Println("ingress: dial error: ", err)
			continue
		}
		log.Println("ingress: dial success:", dc.Label())

		go func() {
			err = relay(conn, stream)
			if err != nil {
				log.Println("ingress: relay error:", err)
			}
		}()
	}
}

func (igp *IngressProxy) Stop() (err error) {
	return igp.peer.Close()
}
