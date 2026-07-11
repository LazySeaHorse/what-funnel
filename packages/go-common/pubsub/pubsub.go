package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps the redis.Client to support pub/sub via Redis Streams.
type Client struct {
	rdb *redis.Client
}

// NewClient creates a new pub/sub Client connected to Redis.
func NewClient(addr string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect to redis at %s: %w", addr, err)
	}
	return &Client{rdb: rdb}, nil
}

// Close closes the underlying Redis client connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Publish serializes payload to JSON and appends it to the specified stream.
func (c *Client) Publish(ctx context.Context, stream string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	id, err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"payload": data},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd to stream %s: %w", stream, err)
	}
	return id, nil
}

// EnsureGroup creates a consumer group on the stream if it does not already exist.
func (c *Client) EnsureGroup(ctx context.Context, stream, group string) error {
	err := c.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil {
		// Ignore BUSYGROUP error (group already exists)
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return fmt.Errorf("create consumer group %s for stream %s: %w", group, stream, err)
		}
	}
	return nil
}

// Consume starts a blocking loop that reads from a stream using consumer groups.
// Messages are passed to the handler, and acknowledged (XACK) if the handler returns nil.
func (c *Client) Consume(ctx context.Context, stream, group, consumer string, handler func(ctx context.Context, id string, payload []byte) error) error {
	if err := c.EnsureGroup(ctx, stream, group); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read messages from the stream
		// Block for 1s to allow periodic context check
		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    1,
			Block:    1 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Sleep on other errors to avoid a tight error loop
			time.Sleep(1 * time.Second)
			continue
		}

		for _, s := range streams {
			for _, message := range s.Messages {
				payloadStr, ok := message.Values["payload"].(string)
				if !ok {
					// Invalid payload field, ack to clear it
					_ = c.rdb.XAck(ctx, stream, group, message.ID).Err()
					continue
				}

				err := handler(ctx, message.ID, []byte(payloadStr))
				if err == nil {
					_ = c.rdb.XAck(ctx, stream, group, message.ID).Err()
				}
			}
		}
	}
}

// RawClient returns the underlying redis.Client.
func (c *Client) RawClient() *redis.Client {
	return c.rdb
}
