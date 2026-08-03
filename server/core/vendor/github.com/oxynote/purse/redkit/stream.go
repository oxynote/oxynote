package redkit

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

// _streamPayloadField is the single field name used inside each
// stream entry. Keeping it constant lets the Stream encode/decode
// without caring about per-event schemas beyond the JSON payload.
const _streamPayloadField = "payload"

// StreamEntry is one entry consumed from a Stream. ID is the
// Redis-assigned `1234-0`-style identifier the caller passes back to
// Ack; Data is the decoded V payload.
type StreamEntry[V any] struct {
	ID   string
	Data V
}

// Stream is a generic wrapper over a single Redis stream + consumer
// group. The value type V is the payload shape; it is JSON-encoded
// on Publish and decoded on Read.
type Stream[V any] struct {
	pool          *redis.Pool
	name          string
	consumerGroup string
}

// NewStream creates a Stream over the named Redis stream, scoped to
// the given consumer group. Call EnsureGroup once before consuming.
func NewStream[V any](
	pool *redis.Pool,
	name string,
	consumerGroup string,
) *Stream[V] {
	return &Stream[V]{
		pool:          pool,
		name:          name,
		consumerGroup: consumerGroup,
	}
}

// Name returns the underlying Redis stream name.
func (s *Stream[V]) Name() string {
	return s.name
}

// ConsumerGroup returns the consumer-group name.
func (s *Stream[V]) ConsumerGroup() string {
	return s.consumerGroup
}

// Publish appends value to the stream. The encoded payload is a
// single JSON blob under a fixed field name.
func (s *Stream[V]) Publish(ctx context.Context, value V) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("XADD", s.name, "*", _streamPayloadField, payload)
	return err
}

// EnsureGroup creates the consumer group if it doesn't already
// exist. The MKSTREAM flag also creates the stream on first call.
// A `BUSYGROUP` reply from Redis (group already exists) is treated
// as success.
func (s *Stream[V]) EnsureGroup(ctx context.Context) error {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("XGROUP", "CREATE", s.name, s.consumerGroup, "$", "MKSTREAM")
	if err != nil && !isBusyGroup(err) {
		return err
	}

	return nil
}

// Read calls XREADGROUP with the given consumer name and blocks for
// up to block waiting for at most count new entries. A nil/empty
// return with no error means the call timed out without new entries.
func (s *Stream[V]) Read(
	ctx context.Context,
	consumer string,
	count int,
	block time.Duration,
) ([]StreamEntry[V], error) {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reply, err := conn.Do("XREADGROUP",
		"GROUP", s.consumerGroup, consumer,
		"COUNT", count,
		"BLOCK", int(block.Milliseconds()),
		"STREAMS", s.name, ">",
	)
	if err != nil {
		return nil, err
	}

	if reply == nil {
		return nil, nil
	}

	return parseStreamReply[V](reply)
}

// Ack acknowledges entryID, removing it from the consumer group's
// pending-entries list.
func (s *Stream[V]) Ack(ctx context.Context, entryID string) error {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("XACK", s.name, s.consumerGroup, entryID)
	return err
}

// ClaimPending transfers ownership of any pending entries that have
// been idle for at least minIdle to the named consumer, returning
// up to count of them ready to process. Idle entries are entries
// that were delivered to some consumer in this group and never
// acknowledged — typically because that consumer crashed.
//
// Without periodic claiming, a crashed worker's pending entries are
// orphaned forever: XREADGROUP with ">" only returns new (never-
// delivered) entries. Pair with Read on a regular schedule to keep
// the pending-entries list bounded.
//
// Implementation note: this wraps XAUTOCLAIM, which requires Redis
// 6.2+ (and Valkey, which heimdall uses). The returned entries are
// already owned by `consumer` once this call succeeds; the caller
// processes them and acks as it would any other entry.
func (s *Stream[V]) ClaimPending(
	ctx context.Context,
	consumer string,
	minIdle time.Duration,
	count int,
) ([]StreamEntry[V], error) {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reply, err := conn.Do("XAUTOCLAIM",
		s.name,
		s.consumerGroup,
		consumer,
		int(minIdle.Milliseconds()),
		"0-0",
		"COUNT", count,
	)
	if err != nil {
		return nil, err
	}

	return parseClaimReply[V](reply)
}

// isBusyGroup reports whether err is the Redis BUSYGROUP error
// raised by XGROUP CREATE when the group already exists.
func isBusyGroup(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "BUSYGROUP")
}

// parseClaimReply decodes the XAUTOCLAIM response into a flat
// []StreamEntry[V]. The Redis reply shape is:
//
//	[ next-cursor-id, [ [id, [field, value, ...]], ... ] (, [deleted-ids]) ]
//
// We only care about the entry array; the cursor is informational
// (callers iterate by calling ClaimPending repeatedly), and the
// deleted-ids list (Redis 7+) is ignored.
func parseClaimReply[V any](reply any) ([]StreamEntry[V], error) {
	parts, err := redis.Values(reply, nil)
	if err != nil {
		return nil, err
	}

	if len(parts) < 2 {
		return nil, nil
	}

	entriesRaw, err := redis.Values(parts[1], nil)
	if err != nil {
		return nil, err
	}

	var entries []StreamEntry[V]

	for _, e := range entriesRaw {
		entry, ok := decodeStreamEntry[V](e)
		if !ok {
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// parseStreamReply decodes the nested array XREADGROUP response into
// flat []StreamEntry[V]. Entries whose payload field is missing or
// fails to decode are skipped silently; the caller can spot lossy
// entries by comparing reply ID counts to returned len.
//
// Reply shape (redigo, simplified):
//
//	[ [ stream_name, [ [id, [field, value, ...]], ... ] ], ... ]
func parseStreamReply[V any](reply any) ([]StreamEntry[V], error) {
	streams, err := redis.Values(reply, nil)
	if err != nil {
		return nil, err
	}

	var entries []StreamEntry[V]

	for _, s := range streams {
		streamPair, err := redis.Values(s, nil)
		if err != nil || len(streamPair) < 2 {
			continue
		}

		entriesRaw, err := redis.Values(streamPair[1], nil)
		if err != nil {
			continue
		}

		for _, e := range entriesRaw {
			entry, ok := decodeStreamEntry[V](e)
			if !ok {
				continue
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// decodeStreamEntry decodes one `[id, [field, value, ...]]` pair.
// Returns ok=false for malformed entries or entries whose payload
// field is missing/invalid.
func decodeStreamEntry[V any](e any) (StreamEntry[V], bool) {
	pair, err := redis.Values(e, nil)
	if err != nil || len(pair) < 2 {
		return StreamEntry[V]{}, false
	}

	id, err := redis.String(pair[0], nil)
	if err != nil {
		return StreamEntry[V]{}, false
	}

	fields, err := redis.Values(pair[1], nil)
	if err != nil {
		return StreamEntry[V]{}, false
	}

	payload := findPayload(fields)
	if len(payload) == 0 {
		return StreamEntry[V]{}, false
	}

	var data V
	if err := json.Unmarshal(payload, &data); err != nil {
		return StreamEntry[V]{}, false
	}

	return StreamEntry[V]{ID: id, Data: data}, true
}

// findPayload pulls the payload bytes out of a flat [k1, v1, k2, v2, …]
// XREADGROUP field list. Returns nil if the payload field is missing.
func findPayload(fields []any) []byte {
	for i := 0; i+1 < len(fields); i += 2 {
		key, err := redis.String(fields[i], nil)
		if err != nil {
			continue
		}
		if key != _streamPayloadField {
			continue
		}

		v, err := redis.Bytes(fields[i+1], nil)
		if err != nil {
			continue
		}

		return v
	}

	return nil
}
