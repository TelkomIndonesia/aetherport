package main

import (
	"context"
	"fmt"
)

func (c *CliProxy) runTTY(ctx context.Context) (err error) {
	peer, err := c.newWebRTCPeerConnection()
	if err != nil {
		return fmt.Errorf("create peer connection failed: %w", err)
	}

	switch {
	case len(c.Allows) > 0:
		i := &IngressProxy{
			signal: NewSignalTTY(),
			peer:   peer,
			epAuth: NewBasicEndpointAuthorizer(c.Allows),
		}
		if err := i.Start(ctx); err != nil {
			return fmt.Errorf("start ingress proxy errored: %w", err)
		}

	case len(c.Forwards) > 0:
		var eps []Endpoint
		for _, pair := range c.Forwards {
			ep, err := EndpointFromString(pair)
			if err != nil {
				return err
			}
			eps = append(eps, ep)
		}

		i := &EgressProxy{
			signal:    NewSignalTTY(),
			peer:      peer,
			endpoints: eps,
		}
		if err := i.Start(ctx); err != nil {
			return fmt.Errorf("start egress proxy errored: %w", err)
		}

	default:
		return fmt.Errorf("either specify --forward or --allow")
	}
	return
}
