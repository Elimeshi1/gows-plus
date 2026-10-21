package noop

import (
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
)

type GroupMemberLabelStorage struct{}

var _ storage.GroupMemberLabelStorage = (*GroupMemberLabelStorage)(nil)

func NewGroupMemberLabelStorage() *GroupMemberLabelStorage {
	return &GroupMemberLabelStorage{}
}

func (s GroupMemberLabelStorage) UpsertGroupMemberLabel(label *storage.GroupMemberLabel) error {
	return nil
}

func (s GroupMemberLabelStorage) UpsertGroupMemberLabels(labels []*storage.GroupMemberLabel) error {
	return nil
}

func (s GroupMemberLabelStorage) GetGroupMemberLabels(group types.JID) ([]*storage.GroupMemberLabel, error) {
	return []*storage.GroupMemberLabel{}, nil
}

func (s GroupMemberLabelStorage) GetGroupMemberLabel(group types.JID, member types.JID) (*storage.GroupMemberLabel, error) {
	return nil, nil
}

func (s GroupMemberLabelStorage) DeleteGroupMemberLabel(group types.JID, member types.JID) error {
	return nil
}

func (s GroupMemberLabelStorage) DeleteGroupMemberLabels(group types.JID) error {
	return nil
}
