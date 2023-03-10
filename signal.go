package main

import (
	"context"

	"github.com/pion/webrtc/v3"
)

type SignalEgress interface {
	SendOffer(ctx context.Context, offer string) (err error)
	RecvAnswer(ctx context.Context) (answer string, err error)
	Close() (err error)
}

type SignalIngress interface {
	RecvOffer(ctx context.Context) (offer string, err error)
	SendAnswer(ctx context.Context, answer string) (err error)
	Close() (err error)
}

type SignalICE interface {
	SendICECandidate(ctx context.Context, ic *webrtc.ICECandidate) (err error)
	RecvICECandidate(ctx context.Context) (ic *webrtc.ICECandidateInit, err error)
	Close() (err error)
}
