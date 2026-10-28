package gows

import (
	"errors"
	"runtime/debug"
	"time"

	"github.com/avast/retry-go"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// StorageEventHandler handles events from WhatsApp and stores them in the database.
// It can be configured to ignore events from certain types of JIDs based on their server type.
type StorageEventHandler struct {
	gows    *GoWS
	log     waLog.Logger
	storage *storage.Storage
	// ignoreJids specifies which types of JIDs should be ignored when processing events.
	ignoreJids *IgnoreJidsConfig
	// statusAcks counts who received and who viewed the statuses we sent. It is
	// fed from here, in the storage branch of the event fan-out, so the counters
	// survive a gRPC listener that falls behind or reconnects. nil disables it.
	statusAcks *StatusAckWriter
}

func (st *StorageEventHandler) shouldIgnoreJID(jid types.JID) bool {
	if jid.IsEmpty() {
		return false
	}

	if st.ignoreJids == nil {
		return false
	}

	// Check if the JID should be ignored based on its server type
	switch jid.Server {
	case types.BroadcastServer:
		// Do not filter status@broadcast when broadcast-ignore is enabled:
		// it is controlled by the dedicated Status flag.
		if jid.User == "status" {
			return st.ignoreJids.Status
		}
		return st.ignoreJids.Broadcast
	case types.GroupServer:
		return st.ignoreJids.Groups
	case types.NewsletterServer:
		return st.ignoreJids.Newsletters
	default:
		return false
	}
}

// isStatusBroadcast reports whether the JID is the status@broadcast JID.
func isStatusBroadcast(jid types.JID) bool {
	return jid.Server == types.StatusBroadcastJID.Server &&
		jid.User == types.StatusBroadcastJID.User
}

// isOwnStatusReceipt reports whether a status@broadcast receipt is about a status
// we posted. Viewing somebody else's status raises a receipt on the very same
// chat, and storing those would fill the table with rows about statuses whose
// reach is none of our business.
func (st *StorageEventHandler) isOwnStatusReceipt(event *events.Receipt) bool {
	// A receipt we sent ourselves is our own view of someone else's status. It is
	// also how a linked device reports that we opened our own, which is not a
	// viewer either.
	if event.IsFromMe {
		return false
	}
	// MessageSender carries the author of the status being acknowledged, and
	// whatsmeow leaves it empty when that author is us.
	if event.MessageSender.IsEmpty() {
		return true
	}
	return st.isOwnJID(event.MessageSender)
}

// isOwnJID reports whether a JID addresses this account, under either its phone
// number or its @lid.
func (st *StorageEventHandler) isOwnJID(jid types.JID) bool {
	if jid.IsEmpty() || st.gows == nil || st.gows.Store == nil {
		return false
	}
	for _, own := range []types.JID{st.gows.Store.GetJID(), st.gows.Store.GetLID()} {
		if !own.IsEmpty() && own.User == jid.User {
			return true
		}
	}
	return false
}

// keepOwnStatus reports whether an event must be kept even when its JID would
// otherwise be ignored: our own (fromMe) status@broadcast messages. This lets
// statuses we sent be stored and read back from the DB later even when statuses
// are configured to be ignored. Statuses from others stay filtered.
func keepOwnStatus(jid types.JID, isFromMe bool) bool {
	return isFromMe && isStatusBroadcast(jid)
}

func (st *StorageEventHandler) GetMessageForRetry(requester, to types.JID, id types.MessageID) *waE2E.Message {
	msg, err := st.storage.Messages.GetMessageWithRetries(id)
	if errors.Is(err, storage.ErrNotFound) {
		st.log.Warnf("Message not found in storage for retry - requester %v, to %v, id %v", requester, to, id)
		return nil
	}
	if err != nil {
		st.log.Errorf("Error getting message for retry - requester %v, to %v, id %v: %v", requester, to, id, err)
		return nil
	}
	if msg == nil || msg.Message == nil {
		st.log.Warnf("No message for retry - requester %v, to %v, id %v", requester, to, id)
		return nil
	}
	return msg.Message.RawMessage
}

func isRealMessage(event *events.Message) bool {
	if event.Message == nil {
		return false
	}
	msg := event.Message
	switch {
	case msg.Conversation != nil:
		return true
	case msg.ExtendedTextMessage != nil:
		return true
	case msg.ImageMessage != nil:
		return true
	case msg.ContactMessage != nil:
		return true
	case msg.LocationMessage != nil:
		return true
	case msg.VideoMessage != nil:
		return true
	case msg.PtvMessage != nil:
		return true
	case msg.AudioMessage != nil:
		return true
	case msg.DocumentMessage != nil:
		return true
	case msg.DocumentWithCaptionMessage != nil:
		return true
	case msg.StickerMessage != nil:
		return true
	case msg.ContactsArrayMessage != nil:
		return true
	case msg.TemplateMessage != nil:
		return true
	case msg.ListMessage != nil:
		return true
	case msg.RichResponseMessage != nil:
		return true
	case msg.PollCreationMessage != nil:
		return true
	case msg.PollCreationMessageV2 != nil:
		return true
	case msg.PollCreationMessageV3 != nil:
		return true
	case msg.PollCreationMessageV4 != nil:
		return true
	case msg.PollCreationMessageV5 != nil:
		return true
	case msg.ButtonsResponseMessage != nil:
		return true
	default:
		return false
	}
}

func (st *StorageEventHandler) handleEvent(event interface{}) {
	switch event.(type) {
	case *events.LabelEdit, *events.LabelAssociationChat:
		// Keep whatsmeow's order - the last change per label/chat wins
		// In case of "add" then "delete" and similar events for 1 entity
		st.processEvent(event)
	default:
		go st.processEvent(event)
	}
}

func (st *StorageEventHandler) processEvent(event interface{}) {
	// Handle all panic and log error + stack
	defer func() {
		if err := recover(); err != nil {
			stack := debug.Stack()
			st.log.Errorf("Panic happened in handle event: %v. Stack: %s. Event: %v", err, stack, event)
		}
	}()

	switch event.(type) {
	case *events.Message:
		msg := event.(*events.Message)
		if st.shouldIgnoreJID(msg.Info.Chat) &&
			!keepOwnStatus(msg.Info.Chat, msg.Info.IsFromMe) {
			return
		}
		var status storage.Status
		if msg.Info.IsFromMe {
			status = storage.StatusServerAck
		} else {
			status = storage.StatusDeliveryAck
		}
		st.handleSaveMessage(msg, &status)
		st.handleMessageEvent(msg)
	case *events.Receipt:
		receipt := event.(*events.Receipt)
		// status@broadcast receipts are always for our own statuses; keep them so
		// the stored status keeps its delivery/read status updated even when
		// statuses are otherwise ignored.
		if st.shouldIgnoreJID(receipt.Chat) && !isStatusBroadcast(receipt.Chat) {
			return
		}
		st.handleReceipt(receipt)
	case *events.UndecryptableMessage:
		msg := event.(*events.UndecryptableMessage)
		if st.shouldIgnoreJID(msg.Info.Chat) {
			return
		}
		st.log.Errorf(
			"Undecryptable message %s in %s from %s (unavailable: %v, type: %q, failMode: %q) - content lost unless the sender answers the retry receipt",
			msg.Info.ID, msg.Info.Chat, msg.Info.Sender,
			msg.IsUnavailable, msg.UnavailableType, msg.DecryptFailMode,
		)
	case *events.HistorySync:
		st.handleHistorySync(event.(*events.HistorySync))
	// Groups
	case *events.JoinedGroup:
		group := event.(*events.JoinedGroup)
		if st.shouldIgnoreJID(group.JID) {
			return
		}
		st.handleMeJoinedGroup(group)
	case *events.GroupInfo:
		info := event.(*events.GroupInfo)
		if st.shouldIgnoreJID(info.JID) {
			return
		}
		left := st.handleMeLeftGroup(info)
		if left {
			return
		}
		st.handleGroupInfo(info)
		st.handleGroupMembersLeft(info)
	case *events.DeleteChat:
		deleteChat := event.(*events.DeleteChat)
		if st.shouldIgnoreJID(deleteChat.JID) {
			return
		}
		st.handleDeleteChat(deleteChat)
	case *events.Contact:
		contact := event.(*events.Contact)
		if st.shouldIgnoreJID(contact.JID) {
			return
		}
		st.handleContact(contact)
	// Labels
	case *events.LabelEdit:
		st.handleLabelEdit(event.(*events.LabelEdit))
	case *events.LabelAssociationChat:
		labelAssoc := event.(*events.LabelAssociationChat)
		if st.shouldIgnoreJID(labelAssoc.JID) {
			return
		}
		st.handleLabelAssociationChat(labelAssoc)
	}
}

func (st *StorageEventHandler) handleSaveMessage(event *events.Message, status *storage.Status) {
	messageToStore := &storage.StoredMessage{
		Message: event,
		Status:  status,
		IsReal:  isRealMessage(event),
	}

	err := st.storage.Messages.UpsertOneMessage(messageToStore)
	if err != nil {
		st.log.Errorf("Error storing message %v(%v): %v", event.Info.Chat, event.Info.ID, err)
	}
}

func (st *StorageEventHandler) handleMessageEvent(event *events.Message) {
	if event.Message == nil {
		return
	}
	protocolMessage := event.Message.GetProtocolMessage()

	// Revoked message
	// NOTE: ProtocolMessage_REVOKE is the zero value, always check the message itself first
	isRevoked := protocolMessage != nil && protocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE
	if isRevoked {
		id := protocolMessage.GetKey().GetID()
		if id == "" {
			st.log.Warnf("Revoke message %v without a message id, ignoring", event.Info.ID)
			return
		}
		err := st.storage.Messages.DeleteMessage(id)
		if err != nil {
			st.log.Errorf("Error deleting message %v: %v", id, err)
		}
		return
	}

	// Group member tag - changed
	memberLabel := protocolMessage.GetMemberLabel()
	isMemberLabel := protocolMessage != nil &&
		protocolMessage.GetType() == waE2E.ProtocolMessage_GROUP_MEMBER_LABEL_CHANGE &&
		memberLabel != nil &&
		event.Info.Chat.Server == types.GroupServer
	if isMemberLabel {
		label := &storage.GroupMemberLabel{
			GroupJID:  event.Info.Chat,
			MemberJID: event.Info.Sender.ToNonAD(),
			Label:     memberLabel.GetLabel(),
			Timestamp: event.Info.Timestamp,
		}
		if memberLabel.GetLabelTimestamp() > 0 {
			label.Timestamp = time.Unix(memberLabel.GetLabelTimestamp(), 0)
		}
		err := st.storage.GroupMemberLabels.UpsertGroupMemberLabel(label)
		if err != nil {
			st.log.Errorf("Error saving member tag for %v in %v: %v", label.MemberJID, label.GroupJID, err)
		}
		return
	}

	// Chat ephemeral settings - changed
	if protocolMessage != nil {
		setting := ExtractEphemeralSettingsFromProtocolMessage(event.Info, protocolMessage)
		if setting != nil {
			err := st.storage.ChatEphemeralSetting.UpdateChatEphemeralSetting(setting)
			if err != nil {
				st.log.Errorf("Error updating chat ephemeral setting %v: %v", setting.ID, err)
			}
			st.log.Debugf("Changed chat ephemeral setting %v (enabled: %v)", setting.ID, setting.IsEphemeral)
			return
		}
	}

	// Chat ephemeral settings - from message
	setting := ExtractEphemeralSettingsFromMsg(event)
	if setting != nil {
		err := st.storage.ChatEphemeralSetting.UpdateChatEphemeralSetting(setting)
		if err != nil {
			st.log.Errorf("Error updating chat ephemeral setting %v: %v", setting.ID, err)
		}
		st.log.Debugf("Initial chat ephemeral setting %v (enabled: %v)", setting.ID, setting.IsEphemeral)
		// Do not return - we still need to handle the message
		// return
	}
}

func (st *StorageEventHandler) handleReceipt(event *events.Receipt) {
	// Count status viewers before anything else. This branch of the fan-out
	// writes straight to the database, so a slow gRPC listener can no longer cost
	// us a view, and it also covers the receipt types the status switch below
	// deliberately ignores (read-self).
	if isStatusBroadcast(event.Chat) && st.isOwnStatusReceipt(event) {
		st.statusAcks.Record(event)
	}

	var status storage.Status
	switch event.Type {
	case types.ReceiptTypeDelivered:
		status = storage.StatusDeliveryAck
	case types.ReceiptTypeRead:
		status = storage.StatusRead
	case types.ReceiptTypePlayed:
		status = storage.StatusPlayed
	default:
		st.log.Debugf("Unknown receipt type: %v", event.Type)
		return
	}
	for _, id := range event.MessageIDs {
		st.log.Debugf("Updating status for message %v(%v) to %v (receipt type: '%v')", event.Chat, id, status, event.Type.GoString())
		msg, err := st.storage.Messages.GetMessageWithRetries(id)
		if errors.Is(err, storage.ErrNotFound) {
			st.log.Debugf("Message %v(%v) not found", event.Chat, id)
			continue
		}
		if err != nil {
			st.log.Debugf("Error getting message - storage handle receipt %v(%v): %v", event.Chat, id, err)
			continue
		}
		if msg.Status != nil && *msg.Status >= status {
			continue
		}
		msg.Status = &status
		err = st.storage.Messages.UpsertOneMessage(msg)
		if err != nil {
			st.log.Errorf("Error updating status for message %v(%v): %v", event.Chat, id, err)
			continue
		}
		st.log.Debugf("Updated status for message %v(%v) to %v", event.Chat, id, status)
	}
}

func (st *StorageEventHandler) handleHistorySync(event *events.HistorySync) {
	conversations := event.Data.GetConversations()
	for _, conv := range conversations {
		jid, err := types.ParseJID(conv.GetID())
		if err != nil {
			st.log.Errorf("Error parsing JID: %v", err)
			continue
		}

		// Skip this conversation if the JID should be ignored
		if st.shouldIgnoreJID(jid) {
			continue
		}

		go st.saveHistoryForOneChat(conv, jid)
		if jid.Server == types.GroupServer {
			go st.saveGroupMemberLabelsFromHistory(conv, jid)
		}
	}
	st.log.Debugf("Saved history for %v chats", len(conversations))
}

func (st *StorageEventHandler) saveHistoryForOneChat(conv *waHistorySync.Conversation, chatJID types.JID) {
	historyMessages := conv.GetMessages()
	for _, historyMsg := range historyMessages {
		message := historyMsg.GetMessage()
		msg, err := st.gows.ParseWebMessage(chatJID, message)
		if err != nil {
			st.log.Errorf("Error parsing message: %v", err)
			continue
		}

		var status storage.Status
		if msg.SourceWebMsg != nil && msg.SourceWebMsg.Status != nil {
			status = storage.Status(*msg.SourceWebMsg.Status)
		}

		st.handleSaveMessage(msg, &status)
	}
	st.log.Debugf("Saved %v messages in %v", len(conv.GetMessages()), chatJID)

	setting := ExtractEphemeralSettingsFromConversation(conv, chatJID)
	if setting != nil {
		err := st.storage.ChatEphemeralSetting.UpdateChatEphemeralSetting(setting)
		if err != nil {
			st.log.Errorf("Error updating chat ephemeral setting %v: %v", setting.ID, err)
		}
		st.log.Debugf("Initial chat ephemeral setting %v (enabled: %v)", setting.ID, setting.IsEphemeral)
	}
}

func (st *StorageEventHandler) handleMeJoinedGroup(group *events.JoinedGroup) {
	err := st.storage.Groups.UpsertOneGroup(&group.GroupInfo)
	if err != nil {
		st.log.Errorf("Error storing group %v: %v", group.JID, err)
	}
	st.log.Debugf("I joined group %v", group.JID)
}

func (st *StorageEventHandler) handleMeLeftGroup(info *events.GroupInfo) bool {
	jid := st.gows.Store.ID
	if jid == nil {
		// Not logged in, we can't tell if it's us who left
		return false
	}
	for _, leave := range info.Leave {
		if leave == jid.ToNonAD() {
			st.log.Debugf("I left group %v", info.JID)
			err := st.storage.Groups.DeleteGroup(info.JID)
			if err != nil {
				st.log.Errorf("Error deleting group %v: %v", info.JID, err)
			}
			err = st.storage.GroupMemberLabels.DeleteGroupMemberLabels(info.JID)
			if err != nil {
				st.log.Errorf("Error deleting member tags of group %v: %v", info.JID, err)
			}
			return true
		}
	}
	return false
}

func (st *StorageEventHandler) handleGroupInfo(info *events.GroupInfo) {
	err := retry.Do(func() error {
		return st.storage.Groups.UpdateGroup(info)
	})
	if err != nil {
		st.log.Errorf("Error updating group %v: %v", info.JID, err)
	}
	return
}

func (st *StorageEventHandler) handleGroupMembersLeft(info *events.GroupInfo) {
	for _, member := range info.Leave {
		err := st.storage.GroupMemberLabels.DeleteGroupMemberLabel(info.JID, member.ToNonAD())
		if err != nil {
			st.log.Errorf("Error deleting member tag for %v in %v: %v", member, info.JID, err)
		}
	}
}

// The only source for tags set before pairing
func (st *StorageEventHandler) saveGroupMemberLabelsFromHistory(conv *waHistorySync.Conversation, group types.JID) {
	var labels []*storage.GroupMemberLabel
	for _, participant := range conv.GetParticipant() {
		memberLabel := participant.GetMemberLabel()
		if memberLabel == nil {
			continue
		}
		member, err := types.ParseJID(participant.GetUserJID())
		if err != nil {
			st.log.Warnf("Error parsing participant JID %q in %v: %v", participant.GetUserJID(), group, err)
			continue
		}
		labels = append(labels, &storage.GroupMemberLabel{
			GroupJID:  group,
			MemberJID: member.ToNonAD(),
			Label:     memberLabel.GetLabel(),
			Timestamp: time.Unix(memberLabel.GetLabelTimestamp(), 0),
		})
	}
	if len(labels) == 0 {
		return
	}
	err := st.storage.GroupMemberLabels.UpsertGroupMemberLabels(labels)
	if err != nil {
		st.log.Errorf("Error saving %d member tags from history for %v: %v", len(labels), group, err)
	}
}

func (st *StorageEventHandler) handleDeleteChat(event *events.DeleteChat) {
	err := st.storage.Messages.DeleteChatMessages(event.JID, event.Timestamp)
	if err != nil {
		st.log.Errorf("Error deleting chat messages %v: %v", event.JID, err)
	}
	err = st.storage.ChatEphemeralSetting.DeleteChatEphemeralSetting(event.JID, event.Timestamp)
	if err != nil {
		st.log.Errorf("Error deleting chat ephemeral setting %v: %v", event.JID, err)
	}
	st.log.Debugf("Deleted chat %v", event.JID)
}

func (st *StorageEventHandler) handleLabelEdit(event *events.LabelEdit) {
	if event.Action == nil {
		return
	}
	action := event.Action
	// Delete
	if action.GetDeleted() {
		// No FK cascade anymore, drop the associations here
		err := st.storage.LabelAssociations.RemoveAssociationsByLabelID(event.LabelID)
		if err != nil {
			st.log.Errorf("Error removing label associations for label %v: %v", event.LabelID, err)
			return
		}
		err = st.storage.Labels.DeleteLabel(event.LabelID)
		if err != nil {
			st.log.Errorf("Error deleting label %v: %v", event.LabelID, err)
			return
		}
		st.log.Debugf("Deleted label %v", event.LabelID)
		return
	}

	// Create
	label := &storage.Label{
		ID:       event.LabelID,
		Name:     action.GetName(),
		Color:    int(action.GetColor()),
		IsActive: action.IsActive,
	}
	if action.Type != nil {
		labelType := int(action.GetType())
		label.Type = &labelType
	}

	err := st.storage.Labels.UpsertLabel(label)
	if err != nil {
		st.log.Errorf("Error upserting label %v: %v", label.ID, err)
		return
	}

	st.log.Debugf("Upserted label %v (%v)", label.ID, label.Name)
}

func (st *StorageEventHandler) handleLabelAssociationChat(event *events.LabelAssociationChat) {
	// Get the association data directly from the event
	jid := event.JID
	labelID := event.LabelID
	labeled := event.Action.GetLabeled()

	if labeled {
		// The label may not be stored yet (it can come later in the sync), store the association anyway
		err := st.storage.LabelAssociations.AddAssociation(jid, labelID)
		if err != nil {
			st.log.Errorf("Error adding label association for JID %v and label %v: %v", jid, labelID, err)
			return
		}
		st.log.Debugf("Added label association for JID %v and label %v", jid, labelID)
	} else {
		err := st.storage.LabelAssociations.RemoveAssociation(jid, labelID)
		if err != nil {
			st.log.Errorf("Error removing label association for JID %v and label %v: %v", jid, labelID, err)
			return
		}
		st.log.Debugf("Removed label association for JID %v and label %v", jid, labelID)
	}
}

func (st *StorageEventHandler) handleContact(contact *events.Contact) {
	st.handleContactLidJidMapping(contact)
}

func (st *StorageEventHandler) handleContactLidJidMapping(contact *events.Contact) {
	ctx := st.gows.Context
	cli := st.gows.Client

	// Save lid to jid mapping
	if cli == nil || cli.Store == nil || cli.Store.LIDs == nil {
		return
	}

	var err error
	var lid = types.EmptyJID
	var jid = types.EmptyJID

	act := contact.Action
	switch contact.JID.Server {
	// jid => lid
	case types.DefaultUserServer:
		jid = contact.JID
		if act.GetLidJID() != "" {
			lid, err = types.ParseJID(act.GetLidJID())
			if err != nil {
				st.log.Errorf("Failed to parse LID JID: %v", err)
				return
			}
		}

	// lid => jid
	case types.HiddenUserServer:
		lid = contact.JID
		if act.GetPnJID() != "" {
			jid, err = types.ParseJID(act.GetPnJID())
			if err != nil {
				st.log.Errorf("Failed to parse PN JID: %v", err)
				return
			}
		}
	}

	// Save lid to jid mapping
	if lid == types.EmptyJID || jid == types.EmptyJID {
		return
	}
	err = cli.Store.LIDs.PutLIDMapping(ctx, lid, jid)
	if err != nil {
		st.log.Errorf("Failed to update LID mapping (%v => %v): %v", lid, jid, err)
		return
	}
}
