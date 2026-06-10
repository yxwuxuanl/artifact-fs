package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudflare/artifact-fs/internal/csi"
)

func main() {
	csiEndpoint := flag.String("csi-address", "unix:///csi/csi.sock", "CSI gRPC endpoint")
	root := flag.String("root", "/var/lib/artifact-fs-csi", "data root directory")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	driver, err := csi.NewDriver(*root, os.Stderr)
	if err != nil {
		log.Fatalf("failed to create CSI driver: %v", err)
	}
	if err := driver.Run(ctx, *csiEndpoint); err != nil {
		log.Fatalf("CSI driver exited: %v", err)
	}
}
