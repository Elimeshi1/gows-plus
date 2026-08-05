package gows

import (
	"github.com/devlikeapro/gows/storage"
	meowstorage "github.com/devlikeapro/gows/storage/meow"
	"github.com/devlikeapro/gows/storage/noop"
	"github.com/devlikeapro/gows/storage/sqlstorage"
	"github.com/devlikeapro/gows/storage/views"
	"go.mau.fi/whatsmeow/store"
)

// ApplyDeviceStorageConfig swaps disabled whatsmeow device stores for no-ops.
// The NoopStore must keep Error nil: a non-nil error aborts app state sync and status broadcasts,
// while zero values just skip persisting.
func ApplyDeviceStorageConfig(device *store.Device, cfg StorageConfig) {
	if !cfg.Contacts {
		device.Contacts = &store.NoopStore{}
	}
	if !cfg.MessageSecrets {
		device.MsgSecrets = &store.NoopStore{}
	}
}

func BuildStorage(container *sqlstorage.GContainer, gows *GoWS, cfg StorageConfig) *storage.Storage {
	st := &storage.Storage{}
	st.ChatEphemeralSetting = container.NewChatEphemeralSettingStorage()

	if cfg.Contacts {
		st.Contacts = meowstorage.NewContactStorage(gows.Store)
	} else {
		st.Contacts = noop.NewContactStorage()
	}

	if cfg.Messages {
		st.Messages = container.NewMessageStorage()
	} else {
		st.Messages = noop.NewMessageStorage()
	}

	if cfg.Groups {
		st.Groups = container.NewGroupStorage()
		st.Groups = NewGroupCacheStorage(gows, st.Groups, st.ChatEphemeralSetting)
	} else {
		st.Groups = NewGroupEphemeralOnlyStorage(gows, st.ChatEphemeralSetting)
	}

	if cfg.Chats {
		st.Chats = views.NewChatView(st.Messages, st.Contacts, st.Groups)
	} else {
		st.Chats = noop.NewChatStorage()
	}

	if cfg.Labels {
		st.Labels = container.NewLabelStorage()
		st.LabelAssociations = container.NewLabelAssociationStorage()
	} else {
		st.Labels = noop.NewLabelStorage()
		st.LabelAssociations = noop.NewLabelAssociationStorage()
	}

	st.Lidmap = container.NewLidmapStorage()
	return st
}
