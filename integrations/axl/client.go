// Package axl provides an interface and implementations for AXL (Gensyn)
// pub/sub messaging.  Topic naming follows the comput3 convention:
// comput3.session.<sessionID>.
//
// Message type values:
//   task.assigned  — a new deployment session was started
//   task.status    — an intermediate agent event
//   task.complete  — the deployment finished
package axl

import (
	"context"
)

// Client is the interface used by the agent to publish/subscribe events.
type Client interface {
	// Publish sends msg to the given topic.
	Publish(ctx context.Context, topic string, msg any) error
	// Subscribe registers handler for messages on topic.  handler is called
	// in a separate goroutine for each inbound message.
	Subscribe(ctx context.Context, topic string, handler func(msg []byte)) error
}

// NoopClient is a Client that silently discards all messages.
type NoopClient struct{}

func (NoopClient) Publish(_ context.Context, _ string, _ any) error           { return nil }
func (NoopClient) Subscribe(_ context.Context, _ string, _ func([]byte)) error { return nil }

// New returns an AXL client if credentials are provided, otherwise a NoopClient.
func New(endpoint, privateKey string) (Client, error) {
	if endpoint == "" || privateKey == "" {
		return NoopClient{}, nil
	}
	// TODO: implement real AXL pub/sub client.
	return NoopClient{}, nil
}

// SessionTopic returns the canonical topic name for a session.
func SessionTopic(sessionID string) string {
	return "comput3.session." + sessionID
}
