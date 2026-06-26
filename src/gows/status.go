package gows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/samber/lo"
	"github.com/samber/lo/mutable"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// [WAHA] Defaults tuned for large status@broadcast audiences (#2080 / #2096).
// All values are overridable via environment variables so they can be tuned
// in production without rebuilding the image.
const (
	// Keep large batches: fewer stanzas => far less likely to hit the WhatsApp
	// 429 rate-limit. The cost (the server is slower to ack a big batch) is
	// absorbed by the longer per-batch timeout below.
	defaultStatusParticipantsBatchSize = 5_000
	// 75s (whatsmeow default) is too short for a large status batch and produces
	// false "timed out waiting for message send response" errors. Give the server
	// room to ack.
	defaultStatusBatchTimeoutSec = 180
	// Small gap between batches so we behave like a steady client, not a burst.
	defaultStatusBatchDelayMs = 1500
	// On 429 / timeout, back off and retry the same batch instead of aborting.
	defaultStatusBatchMaxRetries      = 3
	defaultStatusBatchRetryBackoffSec = 5
)

// statusEnvInt reads a non-negative integer from the environment, falling back
// to def when unset or invalid. [WAHA]
func statusEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// isRetryableStatusErr reports whether a failed status batch is worth retrying.
// Transient server conditions (429 rate-limit, 5xx) and ack timeouts are
// retried; anything else (e.g. a malformed request) is treated as permanent. [WAHA]
func isRetryableStatusErr(err error) bool {
	return errors.Is(err, whatsmeow.ErrMessageTimedOut) ||
		errors.Is(err, whatsmeow.ErrServerReturnedError)
}

// SendStatusMessage sends a status message to a Broadcast list.
//
// [WAHA] Behaviour vs. the previous tight loop (#2080 / #2096), made closer to
// how the official WhatsApp client delivers a status to a large audience:
//   - large batches (default 5000) to minimise the number of stanzas and avoid 429;
//   - a long per-batch ack timeout (default 180s) so big batches aren't false-timed-out;
//   - a short delay between batches for a steady, non-bursty cadence;
//   - exponential backoff + retry on 429 / timeout instead of giving up;
//   - best-effort: a partially delivered status is reported as success (the status
//     is already live), with the undelivered participants logged so they can be
//     re-targeted instead of re-sending to everyone and causing duplicates.
func (gows *GoWS) SendStatusMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extra whatsmeow.SendRequestExtra) (*whatsmeow.SendResponse, error) {
	var err error

	allParticipants := extra.Participants
	if len(allParticipants) == 0 {
		// No participants provided, fetch them
		allParticipants, err = gows.int.GetBroadcastListParticipants(ctx, to)
		if err != nil {
			return nil, err
		}
		// so we have ownId first
		mutable.Reverse(allParticipants)
	}

	// Filter out only the right participants
	validParticipants := lo.Filter(allParticipants, func(p types.JID, _ int) bool {
		return p.Server == types.DefaultUserServer
	})

	// [WAHA] Always batch by the configured size (default 5000), regardless of
	// whether the caller supplied the participant list. Previously an explicit
	// list was sent as a single batch, which timed out for large audiences.
	participantsBatchSize := statusEnvInt("WAHA_GOWS_STATUS_PARTICIPANTS_BATCH_SIZE", defaultStatusParticipantsBatchSize)
	if participantsBatchSize <= 0 {
		participantsBatchSize = defaultStatusParticipantsBatchSize
	}
	batchTimeout := time.Duration(statusEnvInt("WAHA_GOWS_STATUS_BATCH_TIMEOUT_SEC", defaultStatusBatchTimeoutSec)) * time.Second
	batchDelay := time.Duration(statusEnvInt("WAHA_GOWS_STATUS_BATCH_DELAY_MS", defaultStatusBatchDelayMs)) * time.Millisecond
	maxRetries := statusEnvInt("WAHA_GOWS_STATUS_BATCH_MAX_RETRIES", defaultStatusBatchMaxRetries)
	retryBackoff := time.Duration(statusEnvInt("WAHA_GOWS_STATUS_BATCH_RETRY_BACKOFF_SEC", defaultStatusBatchRetryBackoffSec)) * time.Second

	batches := lo.Chunk(validParticipants, participantsBatchSize)
	if extra.ID == "" {
		extra.ID = gows.Client.GenerateMessageID()
	}

	errs := make([]error, 0)
	succeeded := 0
	failedParticipants := make([]types.JID, 0)
	ignored := len(allParticipants) - len(validParticipants)
	gows.Log.Infof(
		"Sending status message (%s) in %d batches - %d participants in total, %d per batch, %d ignored (timeout=%s, delay=%s, retries=%d)",
		extra.ID,
		len(batches),
		len(validParticipants),
		participantsBatchSize,
		ignored,
		batchTimeout,
		batchDelay,
		maxRetries,
	)
	for index, participants := range batches {
		// [WAHA] Steady cadence: pause between batches (not before the first one).
		if index > 0 && batchDelay > 0 {
			select {
			case <-time.After(batchDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		batchExtra := extra
		batchExtra.Participants = participants
		// [WAHA] Give the server enough time to ack a large batch.
		if batchTimeout > 0 {
			batchExtra.Timeout = batchTimeout
		}

		batchErr := gows.sendStatusBatchWithRetry(ctx, to, msg, batchExtra, maxRetries, retryBackoff, index+1, len(batches))
		if batchErr != nil {
			gows.Log.Errorf("Failed to send message (%s) to (batch %d/%d): %v", extra.ID, index+1, len(batches), batchErr)
			errs = append(errs, fmt.Errorf("batch %d: %w", index+1, batchErr))
			failedParticipants = append(failedParticipants, participants...)
		} else {
			succeeded++
			gows.Log.Infof("Sending status message (%s) to %d participants (batch %d/%d) - success", extra.ID, len(participants), index+1, len(batches))
		}
	}

	// [WAHA] Best-effort delivery: only fail the whole call if nothing got through.
	// A status that reached at least one batch is already live on WhatsApp, so
	// returning an error here would only push the caller to re-send everything.
	if succeeded == 0 && len(errs) > 0 {
		err = errors.Join(errs...)
		gows.Log.Errorf("Failed to send status message (%s): %v", extra.ID, err)
		return nil, err
	}
	if len(errs) > 0 {
		gows.Log.Warnf(
			"Status message (%s) partially delivered: %d/%d batches ok, %d participants not delivered",
			extra.ID, succeeded, len(batches), len(failedParticipants),
		)
	} else {
		gows.Log.Infof("Sending status message (%s) - success", extra.ID)
	}

	result := &whatsmeow.SendResponse{
		ID:        extra.ID,
		Timestamp: time.Now(),
	}
	return result, nil
}

// sendStatusBatchWithRetry sends a single status batch, retrying transient
// failures (429 rate-limit / ack timeout) with exponential backoff. A permanent
// error returns immediately without wasting the retry budget. [WAHA] #2080 / #2096.
func (gows *GoWS) sendStatusBatchWithRetry(
	ctx context.Context,
	to types.JID,
	msg *waE2E.Message,
	batchExtra whatsmeow.SendRequestExtra,
	maxRetries int,
	backoffBase time.Duration,
	batchNum, batchTotal int,
) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: base, 3x, 9x, ... (e.g. 5s, 15s, 45s).
			backoff := backoffBase * time.Duration(pow3(attempt-1))
			gows.Log.Warnf(
				"Retrying status batch %d/%d (attempt %d/%d) after %s - previous error: %v",
				batchNum, batchTotal, attempt, maxRetries, backoff, lastErr,
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		_, err := gows.Client.SendMessage(ctx, to, msg, batchExtra)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableStatusErr(err) {
			// Permanent error - don't retry.
			return err
		}
	}
	return lastErr
}

// pow3 returns 3^n for small non-negative n. [WAHA]
func pow3(n int) int {
	r := 1
	for i := 0; i < n; i++ {
		r *= 3
	}
	return r
}
