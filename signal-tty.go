package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
)

var _ SignalEgress = SignalTTY{}
var _ SignalIngress = SignalTTY{}

type SignalTTY struct {
	in  bufio.Reader
	out io.Writer
}

func NewSignalTTY() SignalTTY {
	return SignalTTY{
		in:  *bufio.NewReaderSize(os.Stdin, 10*1024),
		out: os.Stdout,
	}
}

func (s SignalTTY) SendOffer(ctx context.Context, offer string) (err error) {
	s.out.Write([]byte("Offer:\n"))
	return s.write(ctx, offer)
}

func (s SignalTTY) SendAnswer(ctx context.Context, answer string) (err error) {
	s.out.Write([]byte("Answer:\n"))
	return s.write(ctx, answer)
}

func (s SignalTTY) write(ctx context.Context, str string) (err error) {
	done := make(chan struct{})
	go func() {
		defer chanSend(ctx, done, struct{}{})

		var bu bytes.Buffer
		w := zlib.NewWriter(&bu)
		w.Write([]byte(str))
		w.Close()

		var i big.Int
		b := []byte(i.SetBytes(bu.Bytes()).Text(62))

		n, errw := s.out.Write(append(b, '\n'))
		if errw != nil {
			err = errw
			return
		}
		if n < len(b) {
			err = fmt.Errorf("incomplete write (%d) out of (%d)", n, len(b))
		}
	}()

	if _, errr := chanRecv(ctx, done); errr != nil {
		err = errr
	}
	return
}

func (s SignalTTY) RecvAnswer(ctx context.Context) (answer string, err error) {
	s.out.Write([]byte("Peer's Answer?\n"))
	return s.readLine(ctx)
}

func (s SignalTTY) RecvOffer(ctx context.Context) (offer string, err error) {
	s.out.Write([]byte("Peer's Offer?\n"))
	return s.readLine(ctx)
}

func (s SignalTTY) readLine(ctx context.Context) (line string, err error) {
	done := make(chan struct{})
	go func() {
		defer chanSend(ctx, done, struct{}{})

		for {
			if ctx.Err() != nil {
				return
			}

			b, errr := s.in.ReadBytes('\n')
			if err != nil {
				err = errr
				return
			}

			var i big.Int
			if _, ok := i.SetString(string(b[:len(b)-1]), 62); !ok {
				err = fmt.Errorf("invalid base62 string: %v", string(b))
				return
			}

			var bu bytes.Buffer
			bu.Write(i.Bytes())
			r, errw := zlib.NewReader(&bu)
			if errw != nil {
				err = errw
				return
			}

			b, err = io.ReadAll(r)
			if err != nil {
				return
			}
			line = string(b)
			if len(line) > 0 {
				break
			}
		}
	}()

	if _, errr := chanRecv(ctx, done); errr != nil {
		err = errr
	}
	return
}

func (s SignalTTY) Close() (err error) {
	return
}
