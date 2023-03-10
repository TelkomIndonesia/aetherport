package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/go-chi/chi/v5"
	"github.com/xtaci/smux"
	"golang.org/x/crypto/nacl/box"
	"nhooyr.io/websocket"
)

type ingress struct {
	id      string
	conn    net.Conn
	session *smux.Session
}

func newIngress(id string, c net.Conn) (mst ingress, err error) {
	cfg := smux.DefaultConfig()
	cfg.KeepAliveInterval = time.Second
	cfg.KeepAliveTimeout = 3 * time.Second
	session, err := smux.Client(c, cfg)
	if err != nil {
		return mst, fmt.Errorf("create smux session failed: %w", err)
	}

	return ingress{id: id, conn: c, session: session}, nil
}

func (mst ingress) relay(ctx context.Context, egress io.ReadWriteCloser) (err error) {
	conn, err := mst.session.Open()
	if err != nil {
		return fmt.Errorf("open session failed: %w", err)
	}

	return relay(egress, conn)
}

func NewAetherlightHandler() (*chi.Mux, error) {
	r := chi.NewRouter()

	pub, key, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	r.Get("/public-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/octet-stream")
		w.Write(pub[:])
	})

	ingresses := map[string]ingress{}
	r.Get("/ingresses/{ingressID}", func(w http.ResponseWriter, r *http.Request) {
		ingressID := chi.URLParam(r, "ingressID")

		isMaster := false
		if r.Header.Get("x-token") != "" {
			if err := authenticate(r, key, ingressID); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				log.Println(err)
				return
			}

			isMaster = true
		}

		ing, ok := ingresses[ingressID]
		if !isMaster && !ok {
			w.WriteHeader(http.StatusNotFound)
			log.Printf("ingress '%s' not found\n", ingressID)
			return
		}
		if isMaster && ok {
			w.WriteHeader(http.StatusConflict)
			log.Printf("ingress '%s' exist\n", ingressID)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		sc, msg := websocket.StatusNormalClosure, "closed"
		defer func() { conn.Close(sc, msg) }()

		ctx := r.Context()
		switch isMaster {
		case true:
			ing, err := newIngress(ingressID, websocket.NetConn(ctx, conn, websocket.MessageBinary))
			if err != nil {
				log.Printf("create new ingress '%s' failed: %v\n", ingressID, err)
				return
			}

			ingresses[ingressID] = ing
			defer func() {
				if ingd := ingresses[ingressID]; ingd != ing {
					return
				}
				delete(ingresses, ingressID)
			}()
			<-ing.session.CloseChan()
			log.Printf("ingress '%s' has been disconnected", ingressID)

		case false:
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()

			err = ing.relay(ctx, websocket.NetConn(ctx, conn, websocket.MessageBinary))
		}

		switch {
		default:
			log.Println("tunnel error: ", err)
			sc, msg = websocket.CloseStatus(err), err.Error()

		case err == nil:
		case errors.Is(err, ctx.Err()):
		case errors.Is(err, io.EOF):
		case errors.Is(err, io.ErrClosedPipe):
		}
	})

	return r, nil
}

func authenticate(r *http.Request, key *[32]byte, ingressID string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("signature verification panic: %v", e)
		}
	}()

	pubkey := [32]byte{}
	copy(pubkey[:], base58.Decode(ingressID))

	date, err := parseDate(r, nil)
	if err != nil {
		return fmt.Errorf("invalid date (%s): %w", date, err)
	}
	nonce := [24]byte{}
	binary.LittleEndian.PutUint64(nonce[:], uint64(date.Unix()))

	payload, err := base64.RawURLEncoding.DecodeString(r.Header.Get("x-token"))
	if err != nil {
		return fmt.Errorf("invalid base64 payload / public key (%s): %w", payload, err)
	}

	if _, ok := box.Open(nil, payload, &nonce, &pubkey, key); !ok {
		return fmt.Errorf("decryption failed")
	}

	return
}

func parseDate(r *http.Request, skew *time.Duration) (date time.Time, err error) {
	s := r.Header.Get("date")
	if s == "" {
		s = r.Header.Get("x-date")
	}

	date, err = http.ParseTime(s)
	if err != nil {
		return date, fmt.Errorf("unparsed date (%s): %w", date, err)
	}
	sk := 5 * time.Second
	if skew != nil {
		sk = *skew
	}
	if diff := time.Now().Sub(date); diff.Abs() > sk {
		return date, fmt.Errorf("too high skew (%t): %s", date, diff)
	}
	return
}
