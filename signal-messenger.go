package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"

	"github.com/pion/webrtc/v3"
)

var _ SignalICE = &SignalMessenger{}
var _ SignalIngress = &SignalMessenger{}
var _ SignalEgress = &SignalMessenger{}

type SignalMessenger struct {
	offer  chan string
	answer chan string
	ican   chan *webrtc.ICECandidateInit

	io Messenger
}

func NewSignalMessenger(ctx context.Context, io Messenger) (s *SignalMessenger) {
	s = &SignalMessenger{
		offer:  make(chan string),
		answer: make(chan string),
		ican:   make(chan *webrtc.ICECandidateInit),
		io:     io,
	}
	go s.readLoop(ctx)
	return
}

func (s *SignalMessenger) readLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			break
		}

		b, err := s.io.Read(ctx)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			break
		}
		if err != nil {
			log.Println("signal io: read failed:", err)
			break
		}

		msg := &SignalMessengerMessage{}
		if err := json.Unmarshal(b, msg); err != nil {
			log.Println("signalio: received unknwon message:", string(b))
			continue
		}

		switch msg.Type {
		case signalMessengerOffer:
			chanSend(ctx, s.offer, msg.Data)

		case signalMessengerAnswer:
			chanSend(ctx, s.answer, msg.Data)

		case signalMessengerIceCandidate:
			chanSend(ctx, s.ican, msg.ICECandidateInit())
		}
	}
}

func (s *SignalMessenger) SendOffer(ctx context.Context, offer string) (err error) {
	msg := SignalMessengerMessage{
		Type: signalMessengerOffer,
		Data: offer,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}

	return s.io.Write(ctx, b)
}

func (s *SignalMessenger) RecvAnswer(ctx context.Context) (answer string, err error) {
	return chanRecv(ctx, s.answer)
}

func (s *SignalMessenger) RecvOffer(ctx context.Context) (offer string, err error) {
	return chanRecv(ctx, s.offer)
}

func (s *SignalMessenger) SendAnswer(ctx context.Context, answer string) (err error) {
	msg := SignalMessengerMessage{
		Type: signalMessengerAnswer,
		Data: answer,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}

	return s.io.Write(ctx, b)
}

func (s *SignalMessenger) SendICECandidate(ctx context.Context, ic *webrtc.ICECandidate) (err error) {
	if ic == nil {
		return
	}

	b, err := json.Marshal(ic.ToJSON())
	if err != nil {
		return err
	}

	msg := SignalMessengerMessage{
		Type: signalMessengerIceCandidate,
		Data: string(b),
	}
	b, err = json.Marshal(msg)
	if err != nil {
		return
	}

	return s.io.Write(ctx, b)
}

func (s *SignalMessenger) RecvICECandidate(ctx context.Context) (ic *webrtc.ICECandidateInit, err error) {
	return chanRecv(ctx, s.ican)
}

func (s *SignalMessenger) Close() (err error) {
	chanClose(s.answer)
	chanClose(s.offer)
	chanClose(s.ican)
	return s.io.Close()
}

const (
	signalMessengerOffer        = "OFFER"
	signalMessengerAnswer       = "ANSWER"
	signalMessengerIceCandidate = "ICE_CANDIDATE"
)

type SignalMessengerMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

func (s *SignalMessengerMessage) ICECandidateInit() (ican *webrtc.ICECandidateInit) {
	ican = &webrtc.ICECandidateInit{}
	json.Unmarshal([]byte(s.Data), ican)
	return
}
