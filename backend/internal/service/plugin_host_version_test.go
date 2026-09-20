package service

import (
	"context"
	"testing"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type hostVersionClient struct {
	pluginv1.TransportPluginClient
	message  string
	versions []uint32
}

func (c *hostVersionClient) InitHostServices(_ context.Context, req *pluginv1.InitHostServicesRequest, _ ...grpc.CallOption) (*pluginv1.InitHostServicesResponse, error) {
	c.versions = append(c.versions, req.HostServiceApiVersion)
	if req.HostServiceApiVersion > 1 && c.message != "" {
		return &pluginv1.InitHostServicesResponse{Message: c.message}, nil
	}
	return &pluginv1.InitHostServicesResponse{Ready: true}, nil
}

func TestPluginHostVersionNegotiation(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		versions      []uint32
		ready         bool
	}{
		{"modern", "", []uint32{2}, true},
		{"statekit-v1", "unsupported host service API", []uint32{2, 1}, true},
		{"unrelated-failure", "host broker unavailable", []uint32{2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &hostVersionClient{message: tc.message}
			response, err := initializePluginHostServices(context.Background(), api, 7)
			require.NoError(t, err)
			require.Equal(t, tc.ready, response.Ready)
			require.Equal(t, tc.versions, api.versions)
		})
	}
}
