package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
)

func main() {
	secID := flag.String("secid", "", "MOEX security identifier")
	flag.Parse()
	if flag.NArg() != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := moex.NewClient(os.Getenv("MOEX_BASE_URL"), 30*time.Second)
	schedule, err := client.GetBondization(ctx, *secID)
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(schedule)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
