package sqlstorage

import (
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/devlikeapro/gows/storage"
	"github.com/jmoiron/sqlx"
)

// StatusAcksTable holds one row per (status message, viewer) pair.
const StatusAcksTable = "gows_status_acks"

type SqlStatusAckStore struct {
	db *sqlx.DB
}

var _ storage.StatusAckStorage = (*SqlStatusAckStore)(nil)

func (gc *GContainer) NewStatusAckStorage() *SqlStatusAckStore {
	return &SqlStatusAckStore{db: gc.db}
}

// UpsertMany writes a whole batch in one statement, keeping the highest ack ever
// seen for a pair. Records addressing the same pair are collapsed first:
// ON CONFLICT DO UPDATE refuses to touch the same row twice in one statement.
func (s *SqlStatusAckStore) UpsertMany(records []storage.StatusAckRecord) error {
	records = dedupeStatusAcks(records)
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
	_, err = s.db.Exec(query, args...)
	return err
}

// GetSummary counts the viewers of one status. Without participants the counting
// happens in SQL, so the thousands of rows a popular status collects never reach
// Go at all.
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

	query, args, err := sq.Select("participant", "ack").
		From(StatusAcksTable).
		Where(sq.Eq{"message_id": messageID}).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows := []struct {
		Participant string `db:"participant"`
		Ack         int    `db:"ack"`
	}{}
	if err = s.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Ack >= storage.StatusAckDelivered {
			summary.Received++
			summary.ReceivedParticipants = append(summary.ReceivedParticipants, row.Participant)
		}
		if row.Ack >= storage.StatusAckRead {
			summary.Read++
			summary.ReadParticipants = append(summary.ReadParticipants, row.Participant)
		}
	}
	return summary, nil
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
