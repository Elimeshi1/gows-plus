package sqlstorage

import (
	"encoding/json"
	"github.com/devlikeapro/gows/storage"
)

type GroupMemberLabelMapper struct {
}

var _ Mapper[storage.GroupMemberLabel] = (*GroupMemberLabelMapper)(nil)
var groupMemberLabelMapper = &GroupMemberLabelMapper{}

func (f *GroupMemberLabelMapper) ToFields(entity *storage.GroupMemberLabel) map[string]interface{} {
	return map[string]interface{}{
		"group_jid":  entity.GroupJID.String(),
		"member_jid": entity.MemberJID.String(),
		"timestamp":  entity.Timestamp.Unix(),
	}
}

func (f *GroupMemberLabelMapper) Marshal(label *storage.GroupMemberLabel) ([]byte, error) {
	return json.Marshal(label)
}

func (f *GroupMemberLabelMapper) Unmarshal(data []byte, label *storage.GroupMemberLabel) error {
	return json.Unmarshal(data, label)
}
