package main

import "context"

type Messenger interface {
	Read(ctx context.Context) (b []byte, err error)
	Write(ctx context.Context, b []byte) (err error)
	Close() error
}
