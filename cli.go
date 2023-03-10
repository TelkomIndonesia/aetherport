package main

import (
	"github.com/alecthomas/kong"
)

type Cli struct {
	Proxy       CliProxy       `cmd:"" default:"withargs" name:"proxy" help:"start aetherport"`
	Signal      CliSignal      `cmd:"" name:"signal" help:"start aetherlight signalling server"`
	Certificate CliCertificate `cmd:"" name:"cert" help:""`
}

func newCLI() (*Cli, *kong.Context) {
	c := &Cli{}
	ctx := kong.Parse(c)
	return c, ctx
}
