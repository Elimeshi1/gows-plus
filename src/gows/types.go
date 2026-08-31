package gows

import (
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type ConnectedEventData struct {
	ID       *types.JID
	LID      *types.JID
	PushName string
}

type EventMessageResponse struct {
	*events.Message
	EventResponse *waE2E.EventResponseMessage
}

type PollVoteEvent struct {
	*events.Message
	Votes *[]string
}

type GroupJoinRequest struct {
	JID           types.JID
	PhoneNumber   types.JID // zero value if not provided
	RequestMethod string    // invite_link | linked_group_join | non_admin_add
}

type GroupJoinRequestEvent struct {
	JID       types.JID // the group JID
	Sender    *types.JID
	SenderPN  *types.JID
	Timestamp time.Time
	Action    string // created | revoked
	Requests  []GroupJoinRequest
}
