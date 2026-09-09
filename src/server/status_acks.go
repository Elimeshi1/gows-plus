package server

import (
	"context"
	"fmt"

	__ "github.com/devlikeapro/gows/proto"
)

// GetStatusAck reports how many contacts received and viewed a status we sent.
//
// The rows behind it are written by the storage event handler, straight from
// whatsmeow, so the answer is complete even when the event stream had to drop
// events for a slow listener.
func (s *Server) GetStatusAck(ctx context.Context, req *__.StatusAckRequest) (*__.StatusAckResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	messageId := req.GetMessageId()
	if cli.Storage == nil || cli.Storage.StatusAcks == nil {
		return &__.StatusAckResponse{MessageId: messageId}, nil
	}
	summary, err := cli.Storage.StatusAcks.GetSummary(messageId, req.GetParticipants())
	if err != nil {
		return nil, fmt.Errorf("error getting status acks for %v: %w", messageId, err)
	}
	return &__.StatusAckResponse{
		MessageId:            summary.MessageID,
		Received:             uint64(summary.Received),
		Read:                 uint64(summary.Read),
		ReceivedParticipants: summary.ReceivedParticipants,
		ReadParticipants:     summary.ReadParticipants,
	}, nil
}
