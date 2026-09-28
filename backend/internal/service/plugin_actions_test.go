package service

import (
	"context"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type actionClientStub struct {
	pluginv1.TransportPluginClient
	request  []byte
	deadline time.Time
}

func (s *actionClientStub) RunAction(ctx context.Context, req *pluginv1.RunActionRequest, _ ...grpc.CallOption) (*pluginv1.RunActionResponse, error) {
	s.request = append([]byte(nil), req.ActionJson...)
	s.deadline, _ = ctx.Deadline()
	return &pluginv1.RunActionResponse{Accepted: true}, nil
}

func TestPluginManagerRunActionRequiresRunningRuntime(t *testing.T) {
	m := &PluginManager{repo: &statusStubRepository{}}
	_, err := m.RunAction(context.Background(), 7, []byte(`{"action":"test"}`))
	require.Error(t, err)
	require.Empty(t, m.runtimes)
}

func TestPluginManagerRunActionLimitsAndForwards(t *testing.T) {
	api := &actionClientStub{}
	m := &PluginManager{repo: &statusStubRepository{}, runtimes: map[int64]*pluginRuntime{7: {api: api}}}
	_, err := m.RunAction(context.Background(), 7, make([]byte, 32769))
	require.Error(t, err)
	require.Empty(t, api.request)
	raw := []byte(`{"action":"test","request_id":"local-fixture"}`)
	result, err := m.RunAction(context.Background(), 7, raw)
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Equal(t, raw, api.request)
	require.WithinDuration(t, time.Now().Add(10*time.Second), api.deadline, time.Second)
}
