package sqlstorage

import (
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
)

type SqlGroupMemberLabelStore struct {
	*EntityRepository[storage.GroupMemberLabel]
}

var _ storage.GroupMemberLabelStorage = (*SqlGroupMemberLabelStore)(nil)

func (gc *GContainer) NewGroupMemberLabelStorage() *SqlGroupMemberLabelStore {
	repo := NewEntityRepository[storage.GroupMemberLabel](
		gc.db,
		GroupMemberLabelsTable,
		groupMemberLabelMapper,
	)
	return &SqlGroupMemberLabelStore{
		repo,
	}
}

func (s SqlGroupMemberLabelStore) UpsertGroupMemberLabel(label *storage.GroupMemberLabel) error {
	if label.MemberJID.IsEmpty() {
		return storage.ErrEmptyGroupMember
	}
	return s.UpsertOne(label)
}

func (s SqlGroupMemberLabelStore) UpsertGroupMemberLabels(labels []*storage.GroupMemberLabel) error {
	valid := make([]*storage.GroupMemberLabel, 0, len(labels))
	for _, label := range labels {
		if label.MemberJID.IsEmpty() {
			continue
		}
		valid = append(valid, label)
	}
	return s.UpsertMany(valid)
}

func (s SqlGroupMemberLabelStore) GetGroupMemberLabels(group types.JID) ([]*storage.GroupMemberLabel, error) {
	conditions := []sq.Sqlizer{
		sq.Eq{"group_jid": group.String()},
	}
	labels, err := s.AllBy(conditions)
	if err != nil {
		return nil, err
	}
	// Removed tags stay as tombstones for the last-write-wins check
	result := make([]*storage.GroupMemberLabel, 0, len(labels))
	for _, label := range labels {
		if label.Label == "" {
			continue
		}
		result = append(result, label)
	}
	return result, nil
}

func (s SqlGroupMemberLabelStore) GetGroupMemberLabel(group types.JID, member types.JID) (*storage.GroupMemberLabel, error) {
	conditions := []sq.Sqlizer{
		sq.Eq{"group_jid": group.String()},
		sq.Eq{"member_jid": member.String()},
	}
	label, err := s.GetBy(conditions)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if label.Label == "" {
		return nil, nil
	}
	return label, nil
}

func (s SqlGroupMemberLabelStore) DeleteGroupMemberLabel(group types.JID, member types.JID) error {
	conditions := []sq.Sqlizer{
		sq.Eq{"group_jid": group.String()},
		sq.Eq{"member_jid": member.String()},
	}
	return s.DeleteBy(conditions)
}

func (s SqlGroupMemberLabelStore) DeleteGroupMemberLabels(group types.JID) error {
	conditions := []sq.Sqlizer{
		sq.Eq{"group_jid": group.String()},
	}
	return s.DeleteBy(conditions)
}
