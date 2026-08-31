package gows

import (
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	GroupJoinRequestCreated = "created"
	GroupJoinRequestRevoked = "revoked"
)

// whatsmeow doesn't parse the membership request group changes, so they arrive in UnknownChanges
func parseGroupJoinRequestEvents(evt *events.GroupInfo) []*GroupJoinRequestEvent {
	var result []*GroupJoinRequestEvent
	for _, node := range evt.UnknownChanges {
		var action string
		switch node.Tag {
		case "created_membership_requests":
			action = GroupJoinRequestCreated
		case "revoked_membership_requests":
			action = GroupJoinRequestRevoked
		default:
			continue
		}
		requests := parseGroupJoinRequests(evt, node)
		if len(requests) == 0 {
			continue
		}
		result = append(result, &GroupJoinRequestEvent{
			JID:       evt.JID,
			Sender:    evt.Sender,
			SenderPN:  evt.SenderPN,
			Timestamp: evt.Timestamp,
			Action:    action,
			Requests:  requests,
		})
	}
	return result
}

func parseGroupJoinRequests(evt *events.GroupInfo, node *waBinary.Node) []GroupJoinRequest {
	requestMethod := node.AttrGetter().OptionalString("request_method")
	var requests []GroupJoinRequest
	// non_admin_add carries the requesters in <requested_user> children (the Sender is the adder),
	// revoked ones in <participant> children
	for _, child := range node.GetChildren() {
		if child.Tag != "requested_user" && child.Tag != "participant" {
			continue
		}
		ag := child.AttrGetter()
		jid := ag.OptionalJIDOrEmpty("jid")
		if jid.IsEmpty() {
			continue
		}
		requests = append(requests, GroupJoinRequest{
			JID:           jid,
			PhoneNumber:   ag.OptionalJIDOrEmpty("phone_number"),
			RequestMethod: requestMethod,
		})
	}
	// Self-requests (e.g. via invite link) carry no children at all -
	// the requester is the notification-level participant (Sender)
	if len(requests) == 0 && evt.Sender != nil && !evt.Sender.IsEmpty() {
		request := GroupJoinRequest{
			JID:           *evt.Sender,
			RequestMethod: requestMethod,
		}
		if evt.SenderPN != nil {
			request.PhoneNumber = *evt.SenderPN
		}
		requests = append(requests, request)
	}
	return requests
}
