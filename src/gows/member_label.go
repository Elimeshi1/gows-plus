package gows

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// SetGroupMemberLabel sets my member tag in the group, an empty label removes it
func (gows *GoWS) SetGroupMemberLabel(ctx context.Context, group types.JID, label string) error {
	now := time.Now()
	message := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_GROUP_MEMBER_LABEL_CHANGE.Enum(),
			MemberLabel: &waE2E.MemberLabel{
				Label:          proto.String(label),
				LabelTimestamp: proto.Int64(now.Unix()),
			},
		},
	}
	tagReason := "user_update"
	if label == "" {
		tagReason = "user_delete"
	}
	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{{
			Tag:   "meta",
			Attrs: waBinary.Attrs{"appdata": "member_tag", "tag_reason": tagReason},
		}},
	}
	_, err := gows.SendMessage(ctx, group, message, extra)
	return err
}
