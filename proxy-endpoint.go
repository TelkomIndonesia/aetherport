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
	proto  string
}

func EndpointFromString(s string) (ep Endpoint, err error) {
	ss := strings.Split(s, ";")
	if len(ss) < 1 {
		return ep, fmt.Errorf("invalid endpoint: %s", s)
	}

	a := strings.Split(ss[0], ":")
	if len(a) != 4 {
		return ep, fmt.Errorf("invalid endpoint: %s", s)
	}

	ep = Endpoint{
		local:  a[0] + ":" + a[1],
		remote: a[2] + ":" + a[3],
	}

	if len(ss) == 1 {
		return
	}

	for _, s := range ss[1:] {
		l, err := labelFromString(s)
		if err != nil {
			return ep, fmt.Errorf("cannot parse label (%s) : %w", s, err)
		}

		switch l.key {
		case "proto":
			ep.proto = l.value
		default:
			return ep, fmt.Errorf("invalid label: %s", l.value)
		}
	}
	return
}

func (ep Endpoint) String() string {
	buff := strings.Builder{}
	buff.WriteString(ep.local + ":" + ep.remote)
	if ep.proto != "" {
		buff.WriteString(";")
		buff.WriteString(newLabel("proto", ep.proto).String())
	}
	return buff.String()
}
