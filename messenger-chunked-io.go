package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
)

var _ Messenger = chunkedIOMessenger{}

type chunkedIOMessenger struct {
	io    io.ReadWriteCloser
	rbuff bufio.Reader
}

func NewChunkedIOMessenger(io io.ReadWriteCloser) chunkedIOMessenger {
	return chunkedIOMessenger{
		io:    io,
		rbuff: *bufio.NewReader(io),
	}
}

func (i chunkedIOMessenger) Write(ctx context.Context, b []byte) (err error) {
	done := make(chan struct{})
	go func() {
		defer chanSend(ctx, done, struct{}{})
		err = i.write(b)
	}()

	if _, errr := chanRecv(ctx, done); errr != nil {
		err = errr
	}
	return
}

func (i chunkedIOMessenger) write(b []byte) (err error) {
	nbl := 8
	switch n := uint64(len(b)); {
	case n < 1<<8:
		nbl = 1
	case n < 1<<16:
		nbl = 2
	case n < 1<<24:
		nbl = 3
	case n < 1<<32:
		nbl = 4
	case n < 1<<40:
		nbl = 5
	case n < 1<<48:
		nbl = 6
	case n < 1<<56:
		nbl = 7
	}

	b, n := append(b[:nbl+(1-nbl/8)], b...), len(b)
	if nbl < 8 {
		b[nbl] = '\n'
	}
	func() {
		if b[0] = byte(n); nbl == 1 {
			return
		}
		if b[1] = byte(n >> 8); nbl == 2 {
			return
		}
		if b[2] = byte(n >> 16); nbl == 3 {
			return
		}
		if b[3] = byte(n >> 24); nbl == 4 {
			return
		}
		if b[4] = byte(n >> 32); nbl == 5 {
			return
		}
		if b[5] = byte(n >> 40); nbl == 6 {
			return
		}
		if b[6] = byte(n >> 48); nbl == 7 {
			return
		}
		b[7] = byte(n >> 56)
	}()

	n, err = i.io.Write(b)
	if n < len(b) && err == nil {
		return fmt.Errorf("incomplete write: (%d) out of (%d)", n, len(b))
	}
	return
}

func (i chunkedIOMessenger) Read(ctx context.Context) (b []byte, err error) {
	done := make(chan struct{})
	go func() {
		defer chanSend(ctx, done, struct{}{})
		b, err = i.read()
	}()

	if _, errr := chanRecv(ctx, done); errr != nil {
		err = errr
	}
	return
}

func (i chunkedIOMessenger) read() (b []byte, err error) {
	p, err := i.rbuff.Peek(8)
	if err != nil && len(p) < 2 { // at minimum accept 2 byte, "0x00" & "0x0a" ('\n')
		return
	}

	nbl, n := 0, uint64(0)
	func() {
		if nbl, n = 1, uint64(p[0]); len(p) == 1 || p[1] == '\n' {
			return
		}
		if nbl, n = 2, n|uint64(p[1])<<8; len(p) == 2 || p[2] == '\n' {
			return
		}
		if nbl, n = 3, n|uint64(p[2])<<16; len(p) == 3 || p[3] == '\n' {
			return
		}
		if nbl, n = 4, n|uint64(p[3])<<24; len(p) == 4 || p[4] == '\n' {
			return
		}
		if nbl, n = 5, n|uint64(p[4])<<32; len(p) == 5 || p[5] == '\n' {
			return
		}
		if nbl, n = 6, n|uint64(p[5])<<40; len(p) == 6 || p[6] == '\n' {
			return
		}
		if nbl, n = 7, n|uint64(p[6])<<48; len(p) == 7 || p[7] == '\n' {
			return
		}
		nbl, n = 8, n|uint64(p[7])<<56
	}()

	b = make([]byte, n+uint64(nbl+(1-nbl/8)))
	_, err = io.ReadFull(&i.rbuff, b)

	return b[nbl+(1-nbl/8):], err
}

func (i chunkedIOMessenger) Close() error {
	return i.io.Close()
}
