package gows

import (
	"sync"
	"time"

	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

const (
	// statusAckQueueSize is how many pending records we hold before a producer
	// blocks. Blocking is deliberate: losing a view is exactly the bug this
	// writer exists to prevent, and every event is already handled in its own
	// goroutine, so back-pressure here never stalls whatsmeow's event loop.
	statusAckQueueSize = 100_000
	// statusAckFlushSize and statusAckFlushInterval decide when a batch is
	// committed. A status sent to thousands of contacts arrives as one long
	// burst of receipts, and batching turns it into a handful of transactions.
	statusAckFlushSize     = 500
	statusAckFlushInterval = 200 * time.Millisecond
	// statusAckRetention - how far back the counters stay answerable. A status
	// itself expires after 24h, but the numbers are still worth reading long
	// after that, so keep a month and prune the rest.
	statusAckRetention     = 30 * 24 * time.Hour
	statusAckPruneInterval = 6 * time.Hour
)

// StatusAckWriter batches status@broadcast receipts into the session database.
//
// It is fed from the storage branch of the event fan-out rather than from the
// gRPC listener, so a slow or reconnecting client cannot make us miss a view.
type StatusAckWriter struct {
	storage  storage.StatusAckStorage
	log      waLog.Logger
	queue    chan storage.StatusAckRecord
	stopping chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewStatusAckWriter starts the writer. A nil storage disables the counting and
// returns nil, which every method below tolerates.
func NewStatusAckWriter(st storage.StatusAckStorage, log waLog.Logger) *StatusAckWriter {
	if st == nil {
		return nil
	}
	writer := &StatusAckWriter{
		storage:  st,
		log:      log,
		queue:    make(chan storage.StatusAckRecord, statusAckQueueSize),
		stopping: make(chan struct{}),
		done:     make(chan struct{}),
	}
	writer.prune()
	writer.mergeDeviceParticipants()
	go writer.run()
	return writer
}

// Record queues every (message, viewer) pair carried by a status@broadcast
// receipt. A single receipt names one viewer and a batch of message ids.
func (w *StatusAckWriter) Record(receipt *events.Receipt) {
	if w == nil || receipt == nil {
		return
	}
	ack := statusAckLevel(receipt.Type)
	if ack == 0 {
		return
	}
	if receipt.Sender.IsEmpty() {
		return
	}
	// ToNonAD drops the device: a contact with WhatsApp linked on a desktop sends
	// a receipt from every device, and we want one row per person, not per device.
	participant := receipt.Sender.ToNonAD().String()
	now := time.Now()
	for _, id := range receipt.MessageIDs {
		if id == "" {
			continue
		}
		record := storage.StatusAckRecord{
			MessageID:   id,
			Participant: participant,
			Ack:         ack,
			UpdatedAt:   now,
		}
		select {
		case w.queue <- record:
		case <-w.stopping:
			return
		}
	}
}

// Stop drains whatever is queued, commits it and waits for the writer to finish.
// It must run before the database is closed.
func (w *StatusAckWriter) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stopping) })
	<-w.done
}

func (w *StatusAckWriter) run() {
	defer close(w.done)

	flushTicker := time.NewTicker(statusAckFlushInterval)
	defer flushTicker.Stop()
	pruneTicker := time.NewTicker(statusAckPruneInterval)
	defer pruneTicker.Stop()

	batch := make([]storage.StatusAckRecord, 0, statusAckFlushSize)
	for {
		select {
		case record := <-w.queue:
			batch = append(batch, record)
			if len(batch) >= statusAckFlushSize {
				batch = w.flush(batch)
			}
		case <-flushTicker.C:
			batch = w.flush(batch)
		case <-pruneTicker.C:
			batch = w.flush(batch)
			w.prune()
		case <-w.stopping:
			w.drain(batch)
			return
		}
	}
}

// drain commits everything still sitting in the queue at shutdown.
func (w *StatusAckWriter) drain(batch []storage.StatusAckRecord) {
	for {
		select {
		case record := <-w.queue:
			batch = append(batch, record)
			if len(batch) >= statusAckFlushSize {
				batch = w.flush(batch)
			}
		default:
			w.flush(batch)
			return
		}
	}
}

// flush commits a batch and hands back the emptied slice.
func (w *StatusAckWriter) flush(batch []storage.StatusAckRecord) []storage.StatusAckRecord {
	if len(batch) == 0 {
		return batch
	}
	if err := w.storage.UpsertMany(batch); err != nil {
		w.log.Errorf("Error writing %d status acks: %v", len(batch), err)
	}
	return batch[:0]
}

// mergeDeviceParticipants repairs rows written before receipts were keyed by
// person instead of by device. It runs once at startup and finds nothing after.
func (w *StatusAckWriter) mergeDeviceParticipants() {
	merged, err := w.storage.MergeDeviceParticipants()
	if err != nil {
		w.log.Warnf("Error merging per-device status acks: %v", err)
		return
	}
	if merged > 0 {
		w.log.Infof("Merged %d per-device status ack rows into one row per viewer", merged)
	}
}

func (w *StatusAckWriter) prune() {
	if err := w.storage.DeleteBefore(time.Now().Add(-statusAckRetention)); err != nil {
		w.log.Warnf("Error pruning status acks: %v", err)
	}
}

// statusAckLevel maps a whatsmeow receipt type to the ack level we store.
// An empty type is WhatsApp's "delivered to the device" receipt.
func statusAckLevel(receiptType types.ReceiptType) int {
	switch receiptType {
	case types.ReceiptTypeDelivered:
		return storage.StatusAckDelivered
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf, types.ReceiptTypePlayed:
		return storage.StatusAckRead
	default:
		return 0
	}
}
