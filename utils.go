package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/go-multierror"
)

var ErrChannelClosed = fmt.Errorf("channel closed")

func chanSend[T any](ctx context.Context, c chan<- T, t T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from panic: %+v", r)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case c <- t:
		return
	}
}

func chanRecv[T any](ctx context.Context, c <-chan T) (t T, err error) {
	select {
	case <-ctx.Done():
		return t, ctx.Err()

	case t, ok := <-c:
		if !ok {
			err = ErrChannelClosed
		}
		return t, err
	}
}

func chanClose[T any](c chan T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from panic: %+v", r)
		}
	}()
	close(c)

	return
}

func relay(a io.ReadWriteCloser, b io.ReadWriteCloser) (err error) {
	var errA, errB error

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer b.Close()
		_, errA = io.Copy(b, a)
	}()
	go func() {
		defer wg.Done()
		defer a.Close()
		_, errB = io.Copy(a, b)
	}()
	wg.Wait()

	switch {
	default:
		return multierror.Append(errA, errB)

	case errA == nil || errB == nil:
	case errors.Is(io.EOF, errA) || errors.Is(io.EOF, errB):
	}
	return
}
