package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	sync "sync"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/xtaci/smux"
	"golang.org/x/crypto/nacl/box"
	"nhooyr.io/websocket"
)

func (c *CliProxy) runAetherlight(ctx context.Context) (err error) {
	bk, err := os.ReadFile(c.KeyFile)
	if err != nil {
		return fmt.Errorf("open private key failed: %w", err)
	}
	key, _, err := UnmarshalX25519PrivateKey(bk)
	if err != nil {
		return fmt.Errorf("unmarshal private key failed: %w", err)
	}

	bc, err := os.ReadFile(c.CertFile)
	if err != nil {
		return fmt.Errorf("open certificate failed: %w", err)
	}
	cert, _, err := UnmarshalAetherportCertificateFromPEM(bc)
	if err != nil {
		return fmt.Errorf("unmarshal certificate failed: %w", err)
	}

	bca, err := os.ReadFile(c.CaCertFile)
	if err != nil {
		return fmt.Errorf("open ca certificate failed: %w", err)
	}
	capool, err := NewCAPoolFromPEM(bca)
	if err != nil {
		return fmt.Errorf("unmarshall ca certificate failed: %w", err)
	}

	id, err := NewIdentity(key, cert, capool)
	if err != nil {
		return fmt.Errorf("instantiating node failed: %w", err)
	}

	wg := sync.WaitGroup{}
	if len(c.Allows) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.runAetherlightIngress(ctx, id); err != nil {
				log.Println("run aetherlight ingress failed:", err)
			}
		}()
	}
	if len(c.Forwards) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.runAetherlightEgress(ctx, id); err != nil {
				log.Println("run aetherlight egress failed:", err)
			}
		}()
	}
	wg.Wait()
	return
}

func (c *CliProxy) runAetherlightIngress(ctx context.Context, id *Identity) (err error) {
	date := time.Now()
	token, err := c.aetherlightToken(ctx, id, date)
	if err != nil {
		return fmt.Errorf("generate aetherlight token failed: %w", err)
	}

	ingressID := base58.Encode(id.cert.Details.PublicKey)
	ws, _, err := websocket.Dial(ctx, c.AetherlightBaseURL+"/ingresses/"+ingressID, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"date":    []string{date.UTC().Format(http.TimeFormat)},
			"x-token": []string{token},
		},
	})
	if err != nil {
		return fmt.Errorf("websocket connection to aetherlight failed: %w", err)
	}
	defer func() { ws.Close(websocket.StatusNormalClosure, "closing") }()
	log.Println("connected to aetherlight at " + c.AetherlightBaseURL + "/ingresses/" + ingressID)

	cfg := smux.DefaultConfig()
	cfg.KeepAliveInterval = time.Second
	session, err := smux.Server(websocket.NetConn(ctx, ws, websocket.MessageBinary), cfg)
	if err != nil {
		return fmt.Errorf("create muxed connection failed: %w", err)
	}
	defer session.Close()

	for {
		conn, err := session.Accept()
		if err != nil {
			return fmt.Errorf("accept muxed connection failed: %w", err)
		}
		defer conn.Close()

		ioc, err := NewNoisedMessengerI(ctx, NewChunkedIOMessenger(conn), id)
		if err != nil {
			return fmt.Errorf("create noised io chunked failed: %w", err)
		}
		defer ioc.Close()

		peer, err := c.newWebRTCPeerConnection()
		if err != nil {
			return fmt.Errorf("create peer connection failed: %w: ", err)
		}
		defer peer.Close()

		ip := &IngressProxy{
			signal:        NewSignalMessenger(ctx, ioc),
			signalTimeout: time.Minute,
			peer:          peer,
			epAuth:        NewBasicEndpointAuthorizer(c.Allows),
		}
		go func() {
			if err := ip.Start(ctx); err != nil {
				log.Fatalln("ingress: start errored:", err)
			}
			log.Println("ingress done")
		}()
	}
}

func (c *CliProxy) aetherlightToken(ctx context.Context, id *Identity, date time.Time) (token string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.AetherlightBaseURL+"/public-key", nil)
	if err != nil {
		return token, fmt.Errorf("construct http request failed: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return token, fmt.Errorf("sending http request failed: %w", err)
	}
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return token, fmt.Errorf("read http response body failed: %w", err)
	}
	defer res.Body.Close()

	data, nonce, priKey, peerkey := make([]byte, 32), [24]byte{}, [32]byte{}, [32]byte{}
	io.ReadFull(rand.Reader, data)
	binary.LittleEndian.PutUint64(nonce[:], uint64(date.Unix()))
	copy(priKey[:], id.key)
	copy(peerkey[:], resBody)

	token = base64.RawURLEncoding.EncodeToString(box.Seal(nil, data, &nonce, &peerkey, &priKey))
	return
}

func (c *CliProxy) runAetherlightEgress(ctx context.Context, id *Identity) (err error) {
	ws, _, err := websocket.Dial(ctx, c.AetherlightIngressURL, nil)
	if err != nil {
		return fmt.Errorf("connectin to websocket failed: %w", err)
	}
	defer func() { ws.Close(websocket.StatusNormalClosure, "closing") }()
	conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)

	ioc, err := NewNoisedMessengerR(ctx, NewChunkedIOMessenger(conn), id)
	if err != nil {
		return fmt.Errorf("create noised io chunked failed: %w", err)
	}
	defer ioc.Close()

	peer, err := c.newWebRTCPeerConnection()
	if err != nil {
		return fmt.Errorf("create peer connection failed: %w", err)
	}
	defer peer.Close()

	var eps []Endpoint
	for _, e := range c.Forwards {
		ep, err := EndpointFromString(e)
		if err != nil {
			return fmt.Errorf("parse forward endpoint failed: %w", err)
		}
		eps = append(eps, ep)
	}
	ep := &EgressProxy{
		signal:        NewSignalMessenger(ctx, ioc),
		signalTimeout: time.Minute,
		peer:          peer,
		endpoints:     eps,
	}
	if err := ep.Start(ctx); err != nil {
		log.Fatalln("egress: start failed:", err)
	}
	log.Println("egress done")
	return
}
