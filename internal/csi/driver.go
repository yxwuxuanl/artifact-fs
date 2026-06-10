package csi

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudflare/artifact-fs/internal/gitstore"
	"github.com/cloudflare/artifact-fs/internal/logging"
	"github.com/cloudflare/artifact-fs/internal/registry"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

type Driver struct {
	srv          *grpc.Server
	is           *identityService
	cs           *controllerService
	ns           *nodeService
	logger       *slog.Logger
	mountManager *mountManager
}

func NewDriver(root string, logWriter io.Writer) (*Driver, error) {
	logger := logging.NewJSONLogger(logWriter, slog.LevelInfo)

	regDB := filepath.Join(root, "registry.sqlite")
	reg, err := registry.New(context.Background(), regDB)
	if err != nil {
		return nil, err
	}

	gs := gitstore.New(logger)
	gs.SetBatchPoolSize(4)

	mm := newMountManager(root, gs, reg, logger)

	d := &Driver{
		logger:       logger,
		mountManager: mm,
	}
	d.is = &identityService{}
	d.cs = &controllerService{mm: mm}
	d.ns = &nodeService{mm: mm, logger: logger}

	d.srv = grpc.NewServer(
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.MaxSendMsgSize(16*1024*1024),
	)
	csi.RegisterIdentityServer(d.srv, d.is)
	csi.RegisterControllerServer(d.srv, d.cs)
	csi.RegisterNodeServer(d.srv, d.ns)
	return d, nil
}

func (d *Driver) Run(ctx context.Context, endpoint string) error {
	addr := strings.TrimPrefix(endpoint, "unix://")
	if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(addr), 0o755); err != nil {
		return err
	}
	listener, err := net.Listen("unix", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		d.srv.GracefulStop()
	}()
	d.logger.Info("CSI driver listening", "endpoint", endpoint)
	return d.srv.Serve(listener)
}
