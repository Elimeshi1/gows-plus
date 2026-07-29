package gows

import (
	"context"
	"encoding/json"
	"errors"

	"go.mau.fi/util/jsontime"
)

// The same MEX (GraphQL) query WhatsApp Web runs to read the account's new-chat message capping
// (WAWebMexFetchNewChatMessageCappingInfoJobQuery -> xwa2_message_capping_info).
const queryFetchMessageCapping = "24503548349331633"

// MessageCappingTypeNewChatThread is the capping bucket WhatsApp Web reads for
// the per-cycle quota on starting new individual chats.
const MessageCappingTypeNewChatThread = "INDIVIDUAL_NEW_CHAT_THREAD"

// MessageCapping is the per-cycle quota WhatsApp applies to starting new chat
// threads - the volume counterpart of the reachout timelock. Unlike the
// timelock, WhatsApp has no whatsmeow event type for it, so it is defined here
// and fetched on demand. TotalQuota is -1 when the account has no cap.
type MessageCapping struct {
	CappingStatus string              `json:"capping_status,omitempty"`
	TotalQuota    int                 `json:"total_quota"`
	UsedQuota     int                 `json:"used_quota"`
	CycleStart    jsontime.UnixString `json:"cycle_start_timestamp,omitzero"`
	CycleEnd      jsontime.UnixString `json:"cycle_end_timestamp,omitzero"`
	ServerSent    jsontime.UnixString `json:"server_sent_timestamp,omitzero"`
	MvStatus      string              `json:"mv_status,omitempty"`
	OteStatus     string              `json:"ote_status,omitempty"`
}

type respFetchMessageCapping struct {
	Capping *MessageCapping `json:"xwa2_message_capping_info"`
}

// ErrNoCappingData is returned when the MEX response carries no capping payload. WhatsApp Web
// treats a null xwa2_message_capping_info as an error too, instead of assuming "no cap".
var ErrNoCappingData = errors.New("no message capping data in response")

// FetchMessageCapping fetches the account's current new-chat message capping
// (quota) for the given type. There is no push notification for it that
// whatsmeow handles, so this active fetch is the only way to read it.
func (gows *GoWS) FetchMessageCapping(ctx context.Context, cappingType string) (*MessageCapping, error) {
	data, err := gows.int.SendMexIQ(ctx, queryFetchMessageCapping, Map{
		"input": Map{"type": cappingType},
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNoCappingData
	}
	var respData respFetchMessageCapping
	err = json.Unmarshal(data, &respData)
	if err != nil {
		return nil, err
	}
	capping := respData.Capping
	if capping == nil {
		return nil, ErrNoCappingData
	}
	// WhatsApp Web clamps used to total when storing; skip when TotalQuota is the -1 "no cap" sentinel
	if capping.TotalQuota >= 0 && capping.UsedQuota > capping.TotalQuota {
		capping.UsedQuota = capping.TotalQuota
	}
	return capping, nil
}
