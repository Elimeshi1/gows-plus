package storage

import (
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Storage struct {
	Messages             MessageStorage
	Contacts             ContactStorage
	Chats                ChatStorage
	Groups               GroupStorage
	ChatEphemeralSetting ChatEphemeralSettingStorage
	Labels               LabelStorage
	LabelAssociations    LabelAssociationStorage
	Lidmap               LidmapStorage
	StatusAcks           StatusAckStorage
}

// StatusAckStorage keeps who received and who viewed each status we sent.
type StatusAckStorage interface {
	// UpsertMany records a batch of observations, keeping the highest ack ever
	// seen for a (message, participant) pair.
	UpsertMany(records []StatusAckRecord) error
	// GetSummary returns the counts for one status, and the viewer lists when
	// participants is set.
	GetSummary(messageID string, participants bool) (*StatusAckSummary, error)
	// DeleteBefore drops rows last updated before the given time.
	DeleteBefore(t time.Time) error
	// MergeDeviceParticipants folds rows stored per device into one row per
	// person, keeping the highest ack. Returns how many per-device rows it
	// rewrote, which is more than the number of rows that disappear: a person
	// seen only from a linked device gets rewritten without being a duplicate.
	MergeDeviceParticipants() (int, error)
	// DeleteSelfViews drops rows whose viewer is the account itself, given the
	// user parts of its own JIDs. Those rows come from viewing somebody else's
	// status, which says nothing about the reach of our own. Returns how many
	// rows it removed.
	DeleteSelfViews(users []string) (int, error)
}

type MessageStorage interface {
	UpsertOneMessage(msg *StoredMessage) error
	GetLastMessagesInChats(filter ChatFilter, sortBy Sort, pagination Pagination, merge bool) ([]*StoredMessage, error)
	GetAllMessages(filters MessageFilter, sortBy Sort, pagination Pagination, merge bool) ([]*StoredMessage, error)
	GetChatMessages(jid types.JID, filters MessageFilter, pagination Pagination, merge bool) ([]*StoredMessage, error)
	// GetMessage fetches a message by ID without retries.
	GetMessage(id types.MessageID) (*StoredMessage, error)
	// GetMessageWithRetries fetches a message by ID with retries.
	GetMessageWithRetries(id types.MessageID) (*StoredMessage, error)
	DeleteChatMessages(jid types.JID, deleteBefore time.Time) error
	DeleteMessage(id types.MessageID) error
}

type GroupStorage interface {
	FetchGroups(force bool) error
	UpdateGroup(update *events.GroupInfo) error
	UpsertOneGroup(group *types.GroupInfo) error
	GetAllGroups(sort Sort, pagination Pagination) ([]*types.GroupInfo, error)
	GetGroup(jid types.JID) (*types.GroupInfo, error)
	DeleteGroup(jid types.JID) error
	DeleteGroups() error
}

type ContactStorage interface {
	GetContact(user types.JID) (*StoredContact, error)
	GetAllContacts(sortBy Sort, pagination Pagination) ([]*StoredContact, error)
}

type ChatStorage interface {
	GetChats(filter ChatFilter, sortBy Sort, pagination Pagination, merge bool) ([]*StoredChat, error)
}

type ChatEphemeralSettingStorage interface {
	GetChatEphemeralSetting(id types.JID) (*StoredChatEphemeralSetting, error)
	UpdateChatEphemeralSetting(setting *StoredChatEphemeralSetting) error
	DeleteChatEphemeralSetting(id types.JID, deleteBefore time.Time) error
}

type LabelStorage interface {
	GetAllLabels() ([]*Label, error)
	GetLabelById(id string) (*Label, error)
	UpsertLabel(label *Label) error
	DeleteLabel(id string) error
}

type LabelAssociationStorage interface {
	GetJIDsByLabelID(labelID string) ([]types.JID, error)
	GetLabelIDsByJID(jid types.JID) ([]string, error)
	AddAssociation(jid types.JID, labelID string) error
	RemoveAssociation(jid types.JID, labelID string) error
}

type LidmapStorage interface {
	// GetAllLidMap returns all lid/pn pairs from the database as an array of LidmapEntry
	GetAllLidMap() ([]LidmapEntry, error)
	// GetLidCount returns the count of lids in the database
	GetLidCount() (int, error)
}
