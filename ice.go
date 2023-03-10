package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/pion/webrtc/v3"
)

func gatherICE(ctx context.Context, peer *webrtc.PeerConnection, signal interface{}) <-chan struct{} {
	done := make(chan struct{})

	switch si, ok := signal.(SignalICE); ok {
	case true:
		go func() {
			defer close(done)
			trickleICEWLog(ctx, peer, si)
		}()

	case false:
		defer close(done)
		chanRecv(ctx, webrtc.GatheringCompletePromise(peer))
	}

	return done
}

func trickleICEWLog(ctx context.Context, peer *webrtc.PeerConnection, s SignalICE) {
	for {
		if ctx.Err() != nil {
			break
		}

		err, ok := <-trickleICE(ctx, peer, s)
		if !ok {
			break
		}
		if err != nil {
			log.Println("trickle ICE failed:", err)
		}
	}
}

func trickleICE(ctx context.Context, peer *webrtc.PeerConnection, s SignalICE) <-chan error {
	errc := make(chan error)
	once := sync.Once{}

	ctx, cancel := context.WithCancel(ctx)
	peer.OnICEGatheringStateChange(func(is webrtc.ICEGathererState) {
		if is == webrtc.ICEGathererStateNew || is == webrtc.ICEGathererStateGathering {
			return
		}

		cancel()
		once.Do(func() { close(errc) })
	})

	peer.OnICECandidate(func(i *webrtc.ICECandidate) {
		if ctx.Err() != nil {
			return
		}

		err := s.SendICECandidate(ctx, i)
		if err != nil {
			chanSend(ctx, errc, fmt.Errorf("send ICE candidate failed: %w", err))
		}
	})

	go func() {
		deffered := make([]*webrtc.ICECandidateInit, 0, 10)
		for {
			if ctx.Err() != nil {
				break
			}

			if peer.RemoteDescription() != nil && len(deffered) > 0 {
				for _, can := range deffered {
					if err := peer.AddICECandidate(*can); err != nil {
						chanSend(ctx, errc, fmt.Errorf("add ICE candidate failed: %w", err))
					}
				}
				deffered = deffered[:0]
			}

			can, err := s.RecvICECandidate(ctx)
			if err != nil {
				break
			}

			if peer.RemoteDescription() == nil {
				deffered = append(deffered, can)
				continue
			}

			if err = peer.AddICECandidate(*can); err != nil {
				chanSend(ctx, errc, fmt.Errorf("add ICE candidate failed: %w", err))
			}
		}
	}()

	return errc
}
