-- Numbered 010: upstream took 008. Databases that already ran it as 008 get
-- their version set back to 7 once, so upstream 008/009 apply and this is a no-op.

-- Per-viewer delivery/read tracking for the status@broadcast messages we send.
--
-- Rows are written from the storage branch of the whatsmeow event fan-out, so
-- the counters do not depend on a gRPC listener keeping up with the event
-- stream: a status sent to thousands of contacts produces a burst of receipts
-- that used to overflow the listener buffer and lose views for good.
CREATE TABLE IF NOT EXISTS gows_status_acks
(
    -- Short status message id, e.g. 3EB0C767D097E9ECFE8B
    message_id  VARCHAR(100) NOT NULL,
    -- The viewer, as the raw JID whatsmeow reports (usually a @lid). A @lid is a
    -- stable per-contact identifier, so distinct @lid == distinct viewer and no
    -- phone-number resolution is needed just to count.
    participant VARCHAR(100) NOT NULL,
    -- 2 = delivered to device, 3 = read/played. Only ever raised, never lowered.
    ack         INTEGER      NOT NULL,
    -- Unix milliseconds, used for retention pruning.
    updated_at  BIGINT       NOT NULL,
    PRIMARY KEY (message_id, participant)
);

-- Summary lookups are always by message id.
CREATE INDEX IF NOT EXISTS gows_status_acks_message_id_idx ON gows_status_acks (message_id);

-- Retention pruning scans by age.
CREATE INDEX IF NOT EXISTS gows_status_acks_updated_at_idx ON gows_status_acks (updated_at);
