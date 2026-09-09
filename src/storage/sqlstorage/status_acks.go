package sqlstorage

import (
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/devlikeapro/gows/storage"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow/types"
)

// StatusAcksTable holds one row per (status message, viewer).
const StatusAcksTable = "gows_status_acks"

// lidMapTable is whatsmeow's own @lid to phone number mapping. Joining against it
// lets us report viewers as phone numbers instead of opaque @lid identifiers,
// without asking the server for anything.
const lidMapTable = "whatsmeow_lid_map"

// deviceParticipantPattern matches participants that still carry a device suffix
// (user:device@server), which is what rows written before the ToNonAD fix look like.
const deviceParticipantPattern = "%:%"

// maxUpsertRows caps how many rows go into one INSERT. Every row binds four
// parameters and SQLite refuses a statement with more than 32766 of them, which a
// one-shot merge of a busy session's backlog comfortably exceeds.
const maxUpsertRows = 500

type SqlStatusAckStore struct {
	db *sqlx.DB
}

var _ storage.StatusAckStorage = (*SqlStatusAckStore)(nil)

func (gc *GContainer) NewStatusAckStorage() *SqlStatusAckStore {
	return &SqlStatusAckStore{db: gc.db}
}

// execer is the bit of sqlx both *sqlx.DB and *sqlx.Tx satisfy, so the upsert can
// run standalone or inside the merge transaction.
type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// UpsertMany writes a whole batch in one statement, keeping the highest ack ever
// seen for a pair.
func (s *SqlStatusAckStore) UpsertMany(records []storage.StatusAckRecord) error {
	return upsertStatusAcks(s.db, records)
}

// upsertStatusAcks collapses records addressing the same pair before writing:
// ON CONFLICT DO UPDATE refuses to touch the same row twice in one statement.
// The write is split into statements small enough for the parameter limit.
func upsertStatusAcks(db execer, records []storage.StatusAckRecord) error {
	records = dedupeStatusAcks(records)
	for start := 0; start < len(records); start += maxUpsertRows {
		end := start + maxUpsertRows
		if end > len(records) {
			end = len(records)
		}
		if err := upsertStatusAckChunk(db, records[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// upsertStatusAckChunk writes one statement's worth of already deduped records.
func upsertStatusAckChunk(db execer, records []storage.StatusAckRecord) error {
	if len(records) == 0 {
		return nil
	}

	insert := sq.Insert(StatusAcksTable).
		Columns("message_id", "participant", "ack", "updated_at")
	for _, record := range records {
		insert = insert.Values(
			record.MessageID,
			record.Participant,
			record.Ack,
			record.UpdatedAt.UnixMilli(),
		)
	}
	insert = insert.Suffix(fmt.Sprintf(
		"ON CONFLICT (message_id, participant) DO UPDATE SET "+
			"ack = EXCLUDED.ack, updated_at = EXCLUDED.updated_at "+
			"WHERE EXCLUDED.ack > %s.ack",
		StatusAcksTable,
	))

	query, args, err := insert.ToSql()
	if err != nil {
		return err
	}
	_, err = db.Exec(query, args...)
	return err
}

// GetSummary counts the viewers of one status. Without participants the counting
// happens in SQL, so the thousands of rows a popular status collects never reach
// Go at all.
//
// The participant list is reported as phone numbers wherever the session already
// knows the @lid behind them. The mapping is read from whatsmeow's local table,
// never from the server: a status can reach thousands of contacts, and asking
// WhatsApp to resolve each unknown one would be a request storm. Viewers we
// cannot map are returned as their @lid.
func (s *SqlStatusAckStore) GetSummary(messageID string, participants bool) (*storage.StatusAckSummary, error) {
	summary := &storage.StatusAckSummary{MessageID: messageID}
	if messageID == "" {
		return summary, nil
	}

	if !participants {
		counts := struct {
			Received int `db:"received"`
			Read     int `db:"read_count"`
		}{}
		query, args, err := sq.Select(
			fmt.Sprintf("COUNT(*) FILTER (WHERE ack >= %d) AS received", storage.StatusAckDelivered),
			fmt.Sprintf("COUNT(*) FILTER (WHERE ack >= %d) AS read_count", storage.StatusAckRead),
		).
			From(StatusAcksTable).
			Where(sq.Eq{"message_id": messageID}).
			ToSql()
		if err != nil {
			return nil, err
		}
		if err = s.db.Get(&counts, query, args...); err != nil {
			return nil, err
		}
		summary.Received = counts.Received
		summary.Read = counts.Read
		return summary, nil
	}

	// substr(participant, 1, length(participant) - 4) drops the "@lid" suffix, which
	// is how the mapping table keys its rows. Both halves are plain SQL92 so this
	// works on SQLite and Postgres alike.
	join := fmt.Sprintf(
		"%s m ON a.participant LIKE '%%@%s' AND m.lid = substr(a.participant, 1, length(a.participant) - 4)",
		lidMapTable, types.HiddenUserServer,
	)
	query, args, err := sq.Select("a.participant", "a.ack", "m.pn").
		From(StatusAcksTable + " a").
		LeftJoin(join).
		Where(sq.Eq{"a.message_id": messageID}).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows := []struct {
		Participant string         `db:"participant"`
		Ack         int            `db:"ack"`
		PN          sql.NullString `db:"pn"`
	}{}
	if err = s.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		participant := row.Participant
		if row.PN.Valid && row.PN.String != "" {
			participant = row.PN.String + "@" + types.DefaultUserServer
		}
		if row.Ack >= storage.StatusAckDelivered {
			summary.Received++
			summary.ReceivedParticipants = append(summary.ReceivedParticipants, participant)
		}
		if row.Ack >= storage.StatusAckRead {
			summary.Read++
			summary.ReadParticipants = append(summary.ReadParticipants, participant)
		}
	}
	return summary, nil
}

// MergeDeviceParticipants folds rows whose participant still carries a device
// suffix into the device-less participant, keeping the highest ack.
//
// A contact with WhatsApp linked on a desktop sends a delivery receipt from every
// device, and each used to be stored as its own participant, so one person counted
// two or three times. Writes are device-less now; this repairs what was already
// stored. It returns how many per-device rows it rewrote - more than the number of
// rows that disappear, since a person seen only from a linked device is rewritten
// without having been a duplicate - and is a no-op once there is nothing left.
func (s *SqlStatusAckStore) MergeDeviceParticipants() (int, error) {
	query, args, err := sq.Select("message_id", "participant", "ack", "updated_at").
		From(StatusAcksTable).
		Where(sq.Like{"participant": deviceParticipantPattern}).
		ToSql()
	if err != nil {
		return 0, err
	}
	rows := []struct {
		MessageID   string `db:"message_id"`
		Participant string `db:"participant"`
		Ack         int    `db:"ack"`
		UpdatedAt   int64  `db:"updated_at"`
	}{}
	if err = s.db.Select(&rows, query, args...); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	merged := make([]storage.StatusAckRecord, 0, len(rows))
	for _, row := range rows {
		jid, parseErr := types.ParseJID(row.Participant)
		if parseErr != nil {
			continue
		}
		merged = append(merged, storage.StatusAckRecord{
			MessageID:   row.MessageID,
			Participant: jid.ToNonAD().String(),
			Ack:         row.Ack,
			UpdatedAt:   time.UnixMilli(row.UpdatedAt),
		})
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once the commit below succeeded

	if err = upsertStatusAcks(tx, merged); err != nil {
		return 0, err
	}
	deleteQuery, deleteArgs, err := sq.Delete(StatusAcksTable).
		Where(sq.Like{"participant": deviceParticipantPattern}).
		ToSql()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(deleteQuery, deleteArgs...); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// DeleteSelfViews drops every row whose viewer is the account itself.
//
// Viewing somebody else's status produces a receipt on status@broadcast just like
// a viewer of ours does, and those used to be stored as well. They are always
// keyed by our own JID, because WhatsApp only forwards a status author the
// receipts for their own posts, so matching on the participant is enough to tell
// the two apart after the fact. users holds the bare user parts of the account's
// phone-number and @lid JIDs.
func (s *SqlStatusAckStore) DeleteSelfViews(users []string) (int, error) {
	own := sq.Or{}
	for _, user := range users {
		if user == "" {
			continue
		}
		// Two patterns per JID: the device-less form we write today, and the
		// user:device form that rows written before the merge still carry.
		own = append(own,
			sq.Like{"participant": user + "@%"},
			sq.Like{"participant": user + ":%@%"},
		)
	}
	if len(own) == 0 {
		return 0, nil
	}

	query, args, err := sq.Delete(StatusAcksTable).Where(own).ToSql()
	if err != nil {
		return 0, err
	}
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
}

// DeleteBefore drops rows last updated before t. WhatsApp statuses expire after
// 24h, so anything older than the retention window is dead weight.
func (s *SqlStatusAckStore) DeleteBefore(t time.Time) error {
	query, args, err := sq.Delete(StatusAcksTable).
		Where(sq.Lt{"updated_at": t.UnixMilli()}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = s.db.Exec(query, args...)
	return err
}

// dedupeStatusAcks collapses records that address the same (message, participant)
// pair, keeping the highest ack and the latest timestamp seen for it.
func dedupeStatusAcks(records []storage.StatusAckRecord) []storage.StatusAckRecord {
	type pair struct {
		messageID   string
		participant string
	}
	index := make(map[pair]int, len(records))
	deduped := make([]storage.StatusAckRecord, 0, len(records))
	for _, record := range records {
		if record.MessageID == "" || record.Participant == "" || record.Ack == 0 {
			continue
		}
		key := pair{record.MessageID, record.Participant}
		position, seen := index[key]
		if !seen {
			index[key] = len(deduped)
			deduped = append(deduped, record)
			continue
		}
		if record.Ack > deduped[position].Ack {
			deduped[position].Ack = record.Ack
		}
		if record.UpdatedAt.After(deduped[position].UpdatedAt) {
			deduped[position].UpdatedAt = record.UpdatedAt
		}
	}
	return deduped
}
