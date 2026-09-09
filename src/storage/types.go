package storage

import (
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Status int

const (
	StatusError       Status = 0
	StatusPending     Status = 1
	StatusServerAck   Status = 2
	StatusDeliveryAck Status = 3
	StatusRead        Status = 4
	StatusPlayed      Status = 5
)

// StoredMessage contains a message and some additional data.
type StoredMessage struct {
	*events.Message
	IsReal bool
	Status *Status
}

// Ack levels tracked for status@broadcast receipts. They mirror the WAHA
// WAMessageAck values the API exposes, not the Status enum above.
const (
	// StatusAckDelivered - the status reached the viewer's device.
	StatusAckDelivered = 2
	// StatusAckRead - the viewer opened (or played) the status.
	StatusAckRead = 3
)

// StatusAckRecord is a single (status, viewer, ack) observation.
type StatusAckRecord struct {
	MessageID   string
	Participant string
	Ack         int
	UpdatedAt   time.Time
}

// StatusAckSummary aggregates every viewer of one status we sent.
// The participant lists are filled in only when they are explicitly asked for:
// a status can reach thousands of contacts, and the counts alone are far
// cheaper to compute and to send.
type StatusAckSummary struct {
	MessageID            string
	Received             int
	Read                 int
	ReceivedParticipants []string
	ReadParticipants     []string
}

type StoredContact struct {
	Jid      types.JID
	Name     string
	PushName string
}

type StoredChat struct {
	Jid                   types.JID
	Name                  string
	ConversationTimestamp time.Time
}

type EphemeralSetting struct {
	Initiator     *waE2E.DisappearingMode_Initiator
	Trigger       *waE2E.DisappearingMode_Trigger
	InitiatedByMe *bool
	Timestamp     *int64
	Expiration    uint32
}

type StoredChatEphemeralSetting struct {
	ID          types.JID
	IsEphemeral bool
	Setting     *EphemeralSetting
}

func NotEphemeral(jid types.JID) *StoredChatEphemeralSetting {
	return &StoredChatEphemeralSetting{
		ID:          jid,
		IsEphemeral: false,
	}
}

type SortOrder string

const (
	SortAsc  SortOrder = "ASC"
	SortDesc SortOrder = "DESC"
)

type Sort struct {
	Field string
	Order SortOrder
}

type Pagination struct {
	Offset uint64
	Limit  uint64
}

type MessageFilter struct {
	Jid          *types.JID
	TimestampGte *time.Time
	TimestampLte *time.Time
	FromMe       *bool
	Status       *Status
}

type ChatFilter struct {
	Jids []types.JID
}

// Label represents a label that can be associated with chats
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color int    `json:"color"`
}

// LabelAssociation represents an association between a label and a JID
type LabelAssociation struct {
	JID     types.JID `json:"jid"`
	LabelID string    `json:"label_id"`
}

// LidmapEntry represents a lid/pn pair
type LidmapEntry struct {
	Lid types.JID `json:"lid"`
	Pn  types.JID `json:"pn"`
}
