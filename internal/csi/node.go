package csi

import (
	"context"
	"log/slog"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type nodeService struct {
	csi.UnimplementedNodeServer
	mm     *mountManager
	logger *slog.Logger
}

func (ns *nodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	volumeID := req.VolumeId
	targetPath := req.TargetPath
	params := req.VolumeContext
	if params == nil {
		params = map[string]string{}
	}

	// Merge secrets into params so private repo tokens are available.
	for k, v := range req.Secrets {
		if _, ok := params[k]; !ok {
			params[k] = v
		}
	}

	cfg := volumeContextToRepoConfig(ns.mm.root, volumeID, params)

	if err := ns.mm.PublishVolume(ctx, volumeID, targetPath, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "publish volume: %v", err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := ns.mm.UnpublishVolume(ctx, req.TargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "unpublish volume: %v", err)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeService) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	nodeID := os.Getenv("NODE_NAME")
	if nodeID == "" {
		var err error
		nodeID, err = os.Hostname()
		if err != nil {
			nodeID = "unknown"
		}
	}
	return &csi.NodeGetInfoResponse{
		NodeId:            nodeID,
		MaxVolumesPerNode: 0,
	}, nil
}

func (ns *nodeService) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{}, nil
}
