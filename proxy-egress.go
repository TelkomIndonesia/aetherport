package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/xtaci/smux"
)

type EgressProxy struct {
	signal        SignalEgress
	signalTimeout time.Duration

	peer      *webrtc.PeerConnection
	endpoints []Endpoint
}

func (egp *EgressProxy) Start(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	egp.peer.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		switch pcs {
		case webrtc.PeerConnectionStateFailed:
			err = fmt.Errorf("peer connection failed")
			egp.Stop()
		case webrtc.PeerConnectionStateDisconnected:
			egp.Stop()
		case webrtc.PeerConnectionStateClosed:
			cancel()
		}
	})

	var sctx = ctx
	if egp.signalTimeout > 0 {
		var scancel context.CancelFunc
		sctx, scancel = context.WithTimeout(ctx, egp.signalTimeout)
		defer scancel()
	}

	if err := egp.createDummyDataChannel(); err != nil {
		return err
	}
	offer, err := egp.peer.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create webrtc offer failed: %w", err)
	}
	if err = egp.peer.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description failed: %w", err)
	}

	iceDone := gatherICE(sctx, egp.peer, egp.signal)

	if err := egp.signal.SendOffer(sctx, egp.peer.LocalDescription().SDP); err != nil {
		return fmt.Errorf("send offer failed: %w", err)
	}

	answer, err := egp.signal.RecvAnswer(sctx)
	if err != nil {
		return fmt.Errorf("receive answer failed: %w", err)
	}
	err = egp.peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(answer),
	})
	if err != nil {
		return fmt.Errorf("set remote description failed: %w", err)
	}

	go func() {
		defer cancel()
		if errt := egp.startTunnels(sctx); errt != nil {
			err = fmt.Errorf("start tunnels failed: %w", errt)
		}
	}()

	chanRecv(sctx, iceDone)
	egp.signal.Close()
	<-ctx.Done()
	egp.Stop()

	return
}

// createDummyDataChannel create auto-closed data channel only to populate SDP
func (egp *EgressProxy) createDummyDataChannel() (err error) {
	dc, err := egp.peer.CreateDataChannel("", nil)
	if err != nil {
		return fmt.Errorf("create dummy data channel failed: %w", err)
	}
	dc.OnOpen(func() { dc.Close() })
	dc.OnError(func(error) { dc.Close() })

	return
}

func (egp *EgressProxy) startTunnels(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	for _, ep := range egp.endpoints {
		if err = ctx.Err(); err != nil {
			return
		}

		dc, err := egp.peer.CreateDataChannel(ep.String(), nil)
		if err != nil {
			cancel()
			return fmt.Errorf("create data channel failed: %w", err)
		}

		wg.Add(1)
		ep := ep
		go func() {
			defer wg.Done()
			if err := egp.startTunnel(ctx, dc, ep); err != nil {
				log.Println("egress: create tunnel failed: ", err)
			}
		}()
	}
	return
}

func (egp *EgressProxy) startTunnel(ctx context.Context, dc *webrtc.DataChannel, ep Endpoint) (err error) {
	defer dc.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dcc, err := NewDataChannelConn(ctx, dc)
	if err != nil {
		return fmt.Errorf("create new datachannel connection failed: %w", err)
	}
	defer dcc.Close()

	session, err := smux.Client(dcc, nil)
	if err != nil {
		return fmt.Errorf("create client session failed: %w", err)
	}
	defer session.Close()

	listener, err := net.Listen("tcp", ep.local)
	if err != nil {
		return fmt.Errorf("listen to local socket failed: %w", err)
	}
	defer listener.Close()

	go func() {
		chanRecv(ctx, session.CloseChan())
		cancel()
		listener.Close()
	}()

	for {
		if ctx.Err() != nil {
			break
		}

		conn, err := listener.Accept()
		if err != nil {
			log.Println("egress: accept connection error:", err)
			continue
		}
		log.Println("egress: new connection: ", conn.RemoteAddr())

		stream, err := session.OpenStream()
		if err != nil {
			log.Println("egress: open stream error:", err)
			continue
		}

		go func() {
			err = relay(conn, stream)
			if err != nil {
				log.Println("egress: relay error:", err)
			}
		}()
	}

	return
}

func (egp *EgressProxy) Stop() (err error) {
	return egp.peer.Close()
}
