package main

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v3"
)

var _ net.Conn = &DataChannelConn{}

type dataChannelAddr struct{}

func (dataChannelAddr) Network() string {
	return "sctp"
}
func (dataChannelAddr) String() string {
	return "host"
}

type DataChannelConn struct {
	*datachannel.DataChannel

	r bufio.Reader

	wCtx            contextExec
	wBuffMax        uint64
	wBuffLow        uint64
	wBuffWaiter     int32
	wBuffWaiterChan chan struct{}
}

func NewDataChannelConn(ctx context.Context, wdc *webrtc.DataChannel) (dcc *DataChannelConn, err error) {
	dc, err := detach(ctx, wdc)
	if err != nil {
		return nil, fmt.Errorf("detach datachannel failed: %w", err)
	}
	dcc = &DataChannelConn{
		DataChannel: dc,

		r: *bufio.NewReaderSize(dc, math.MaxUint16),

		wCtx:            newContextExec(),
		wBuffMax:        1024 * 1024, //TODO: pass as options
		wBuffLow:        512 * 1024,  //TODO: pass as options
		wBuffWaiter:     0,
		wBuffWaiterChan: make(chan struct{}),
	}

	dcc.SetBufferedAmountLowThreshold(dcc.wBuffLow)
	dcc.OnBufferedAmountLow(func() {
		for {
			if dcc.wBuffWaiter == 0 {
				break
			}
			dcc.wBuffWaiterChan <- struct{}{}
			atomic.AddInt32(&dcc.wBuffWaiter, -1)
		}
	})

	return
}

func detach(ctx context.Context, wdc *webrtc.DataChannel) (*datachannel.DataChannel, error) {
	if wdc.ReadyState() != webrtc.DataChannelStateOpen {
		errc := make(chan error)
		once := sync.Once{}
		wdc.OnOpen(func() {
			once.Do(func() { chanSend(ctx, errc, nil) })
		})
		wdc.OnError(func(err error) {
			once.Do(func() { chanSend(ctx, errc, err) })
		})
		wdc.OnClose(func() {
			once.Do(func() { chanSend(ctx, errc, fmt.Errorf("closed before openned")) })
		})

		err, rerr := chanRecv(ctx, errc)
		if rerr != nil {
			wdc.Close()
			return nil, fmt.Errorf("wait for readiness canceled: %w", rerr)
		}
		if err != nil {
			return nil, err
		}
	}

	rwc, err := wdc.Detach()
	if err != nil {
		return nil, err
	}

	dc, ok := rwc.(*datachannel.DataChannel)
	if !ok {
		return nil, fmt.Errorf("unexpected data channel concrete type: %t", rwc)
	}

	return dc, err
}

func (dcc *DataChannelConn) Read(b []byte) (n int, err error) {
	return dcc.r.Read(b)
}

func (dcc *DataChannelConn) Write(b []byte) (n int, err error) {
	errX := dcc.wCtx.exec(func() {
		if dcc.BufferedAmount() > dcc.wBuffMax {
			atomic.AddInt32(&dcc.wBuffWaiter, 1)
			<-dcc.wBuffWaiterChan
		}

		n, err = dcc.DataChannel.Write(b)
		if err != nil {
			return
		}
	})
	if errX != nil {
		return n, errX
	}
	return
}

func (dcc *DataChannelConn) SetDeadline(t time.Time) error {
	if err := dcc.SetReadDeadline(t); err != nil {
		return err
	}
	if err := dcc.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (dcc *DataChannelConn) SetWriteDeadline(t time.Time) error {
	return dcc.wCtx.setDeadline(t)
}

func (dcc *DataChannelConn) LocalAddr() net.Addr {
	return dataChannelAddr{}
}

func (dcc *DataChannelConn) RemoteAddr() net.Addr {
	return dataChannelAddr{}
}

var noctx, nocancel = context.Background(), func() {}

type contextExec struct {
	context context.Context
	cancel  context.CancelFunc
}

func newContextExec() contextExec {
	return contextExec{
		context: noctx,
		cancel:  nocancel,
	}
}

func (ce contextExec) exec(fn func()) (err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	for {
		select {
		case <-done:
			return

		case <-ce.context.Done():
			if ce.context.Err() == context.Canceled {
				continue
			}
			return ce.context.Err()
		}
	}
}

func (ce contextExec) setDeadline(t time.Time) (err error) {
	ce.cancel()
	if t.IsZero() {
		ce.context, ce.cancel = noctx, nocancel
		return
	}

	ce.context, ce.cancel = context.WithDeadline(noctx, t)
	return
}
