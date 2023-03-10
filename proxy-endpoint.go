package main

import (
	"fmt"
	"strings"
)

type EndpointAuthorizer = func(ep Endpoint) (ok bool, err error)

func NewBasicEndpointAuthorizer(epr []string) EndpointAuthorizer {
	noop := func(ep Endpoint) (bool, error) {
		return true, nil
	}

	eps := map[string]struct{}{}
	whitelist := func(ep Endpoint) (bool, error) {
		_, ok := eps[ep.remote]
		return ok, nil
	}

	for _, r := range epr {
		if r == "*" {
			return noop
		}

		eps[r] = struct{}{}
	}

	return whitelist
}

type Endpoint struct {
	local  string
	remote string
}

func EndpointFromString(s string) (ep Endpoint, err error) {
	a := strings.Split(s, ":")
	if len(a) != 4 {
		return ep, fmt.Errorf("invalid forward endpoint: %s", s)
	}

	ep = Endpoint{
		local:  a[0] + ":" + a[1],
		remote: a[2] + ":" + a[3],
	}
	return
}

func (ep Endpoint) String() string {
	return ep.local + ":" + ep.remote
}
