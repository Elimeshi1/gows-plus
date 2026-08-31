package noop

import (
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
)

type ContactStorage struct{}

var _ storage.ContactStorage = (*ContactStorage)(nil)

func NewContactStorage() *ContactStorage {
	return &ContactStorage{}
}

func (s ContactStorage) GetContact(user types.JID) (*storage.StoredContact, error) {
	return nil, storage.StorageDisabled("contacts")
}

func (s ContactStorage) GetAllContacts(sortBy storage.Sort, pagination storage.Pagination) ([]*storage.StoredContact, error) {
	return []*storage.StoredContact{}, nil
}
