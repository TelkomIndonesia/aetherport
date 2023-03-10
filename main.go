package main

import (
	"context"
	"log"
)

func main() {
	ctx := context.Background()
	_, kctx := newCLI()

	kctx.BindTo(ctx, (*context.Context)(nil))
	if err := kctx.Run(); err != nil {
		log.Fatalln("failed to run command:", err)
	}
}
