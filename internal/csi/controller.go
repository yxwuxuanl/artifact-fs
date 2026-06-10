package csi

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type controllerService struct {
	csi.UnimplementedControllerServer
	mm *mountManager
}

func (cs *controllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.Parameters
	if params == nil {
		params = map[string]string{}
	}
	remoteURL := params["remoteURL"]
	if remoteURL == "" {
		return nil, status.Error(codes.InvalidArgument, "remoteURL parameter is required")
	}

	name := req.Name
	cfg := paramsToRepoConfig(cs.mm.root, name, params)

	if err := cs.mm.PrepareRepo(ctx, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to prepare repo: %v", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      cfg.Name,
			CapacityBytes: 1 * 1024 * 1024 * 1024,
			VolumeContext: paramsToVolumeContext(cfg),
		},
	}, nil
}

func (cs *controllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.mm.DeleteRepo(ctx, req.VolumeId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete volume: %v", err)
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerService) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (cs *controllerService) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.AccessMode.Mode != csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			return &csi.ValidateVolumeCapabilitiesResponse{
				Message: "only MULTI_NODE_READER_ONLY access mode is supported",
			}, nil
		}
		if cap.AccessType != nil {
			if _, ok := cap.AccessType.(*csi.VolumeCapability_Mount); !ok {
				return &csi.ValidateVolumeCapabilitiesResponse{
					Message: "only mount access type is supported",
				}, nil
			}
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}
