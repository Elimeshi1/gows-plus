package gows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var requesterLid, _ = types.ParseJID("111111111111111@lid")
var requesterPn, _ = types.ParseJID("12345678901@s.whatsapp.net")
var groupJid, _ = types.ParseJID("123456789012345678@g.us")

// Self-request via invite link - no children, requester is the notification-level Sender
func TestParseGroupJoinRequestEvents_CreatedNoParticipants(t *testing.T) {
	evt := &events.GroupInfo{
		JID:      groupJid,
		Sender:   &requesterLid,
		SenderPN: &requesterPn,
		UnknownChanges: []*waBinary.Node{
			{
				Tag:   "created_membership_requests",
				Attrs: waBinary.Attrs{"request_method": "invite_link"},
			},
		},
	}

	result := parseGroupJoinRequestEvents(evt)
	assert.Len(t, result, 1)
	assert.Equal(t, GroupJoinRequestCreated, result[0].Action)
	assert.Len(t, result[0].Requests, 1)
	assert.Equal(t, requesterLid, result[0].Requests[0].JID)
	assert.Equal(t, requesterPn, result[0].Requests[0].PhoneNumber)
	assert.Equal(t, "invite_link", result[0].Requests[0].RequestMethod)
}

// non_admin_add - the Sender is the adder, the requester is in <requested_user> children
func TestParseGroupJoinRequestEvents_CreatedNonAdminAdd(t *testing.T) {
	adderLid, _ := types.ParseJID("222222222222222@lid")
	adderPn, _ := types.ParseJID("12345678902@s.whatsapp.net")
	requestedLid, _ := types.ParseJID("333333333333333@lid")
	requestedPn, _ := types.ParseJID("12345678903@s.whatsapp.net")
	evt := &events.GroupInfo{
		JID:      groupJid,
		Sender:   &adderLid,
		SenderPN: &adderPn,
		UnknownChanges: []*waBinary.Node{
			{
				Tag:   "created_membership_requests",
				Attrs: waBinary.Attrs{"request_method": "non_admin_add"},
				Content: []waBinary.Node{
					{
						Tag: "requested_user",
						Attrs: waBinary.Attrs{
							"jid":          requestedLid,
							"phone_number": requestedPn,
						},
					},
				},
			},
		},
	}

	result := parseGroupJoinRequestEvents(evt)
	assert.Len(t, result, 1)
	assert.Equal(t, GroupJoinRequestCreated, result[0].Action)
	assert.Len(t, result[0].Requests, 1)
	assert.Equal(t, requestedLid, result[0].Requests[0].JID)
	assert.Equal(t, requestedPn, result[0].Requests[0].PhoneNumber)
	assert.Equal(t, "non_admin_add", result[0].Requests[0].RequestMethod)
}

func TestParseGroupJoinRequestEvents_RevokedWithParticipants(t *testing.T) {
	evt := &events.GroupInfo{
		JID: groupJid,
		UnknownChanges: []*waBinary.Node{
			{
				Tag: "revoked_membership_requests",
				Content: []waBinary.Node{
					{
						Tag: "participant",
						Attrs: waBinary.Attrs{
							"jid":          requesterLid,
							"phone_number": requesterPn,
						},
					},
				},
			},
		},
	}

	result := parseGroupJoinRequestEvents(evt)
	assert.Len(t, result, 1)
	assert.Equal(t, GroupJoinRequestRevoked, result[0].Action)
	assert.Len(t, result[0].Requests, 1)
	assert.Equal(t, requesterLid, result[0].Requests[0].JID)
	assert.Equal(t, requesterPn, result[0].Requests[0].PhoneNumber)
}

func TestParseGroupJoinRequestEvents_OtherChangesIgnored(t *testing.T) {
	evt := &events.GroupInfo{
		JID:    groupJid,
		Sender: &requesterLid,
		UnknownChanges: []*waBinary.Node{
			{Tag: "some_other_change"},
		},
	}

	result := parseGroupJoinRequestEvents(evt)
	assert.Empty(t, result)
}
