package server

import (
	"context"

	__ "github.com/devlikeapro/gows/proto"
)

// FetchReachoutTimelock fetches the account's current reachout timelock state
// on demand, for callers that want a fresh value instead of the last state
// pushed through the event stream.
func (s *Server) FetchReachoutTimelock(ctx context.Context, req *__.Session) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	timelock, err := cli.FetchAccountReachoutTimelock(ctx)
	if err != nil {
		return nil, err
	}
	return toJson(timelock)
}
