-- Member tags participants set for themselves in groups, fed by protocol messages and history sync
CREATE TABLE gows_group_member_labels
(
    -- Group JID
    group_jid VARCHAR(100) NOT NULL,
    -- Member JID (LID when known)
    member_jid VARCHAR(100) NOT NULL,
    -- Last change, unix seconds
    timestamp BIGINT NOT NULL,
    -- Label data (JSON)
    data TEXT NOT NULL,
    PRIMARY KEY (group_jid, member_jid)
);
