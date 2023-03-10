package main

import (
	"context"
	"log"
	"net/http"
)

type CliSignal struct {
	ListenAddr string `name:"listen-addr" short:"l" default:":8080" help:""`
}

func (c *CliSignal) Run(ctx context.Context) (err error) {
	wsServer, err := NewAetherlightHandler()
	if err != nil {
		log.Fatalln("instantiate ws handler failed", err)
	}

	log.Println("listening on:", c.ListenAddr)
	return http.ListenAndServe(c.ListenAddr, wsServer)
}
