package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Agent-to-agent compute. A MetaMax agent does not have to run everything on
// the provider it picked: it can hand a self-contained subtask to another
// agent over Gensyn AXL and wait for that agent's signed result. The remote
// agent is paid through the same escrow contract, so delegation is a payment
// relationship, not just a message.
//
// Wire format on topic comput3.session.<id>:
//
//	{"type":"task.assigned","task_id":…,"session_id":…,"command":…,"image":…}
//	{"type":"task.complete","task_id":…,"output":…,"action_hash":…,"error":…}

// AXLClient is the pub/sub transport used for agent-to-agent messages.
// integrations/axl.Client satisfies it; NoopClient makes delegation a no-op.
type AXLClient interface {
	Publish(ctx context.Context, topic string, msg []byte) error
	Subscribe(ctx context.Context, topic string, handler func(msg []byte)) error
}

// DelegationTimeout bounds how long the agent waits for a peer agent's result
// before giving up and running the subtask itself.
const DelegationTimeout = 90 * time.Second

type delegatedTask struct {
	Type      string `json:"type"`
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	Command   string `json:"command"`
	Image     string `json:"image,omitempty"`
	Payer     string `json:"payer,omitempty"` // agent wallet that will settle
}

type delegatedResult struct {
	Type       string `json:"type"`
	TaskID     string `json:"task_id"`
	Output     string `json:"output"`
	ActionHash string `json:"action_hash"`
	Error      string `json:"error,omitempty"`
	Worker     string `json:"worker,omitempty"` // peer agent's wallet
}

// sessionTopic mirrors axl.SessionTopic without importing the integration,
// keeping this package free of transport dependencies.
func sessionTopic(sessionID string) string { return "comput3.session." + sessionID }

// delegateCompute publishes a subtask to the session topic and waits for a
// peer agent to return a result. It returns an error when no peer answers in
// time, which lets the caller fall back to local execution.
func (s *Session) delegateCompute(ctx context.Context, command, image string) (map[string]any, error) {
	if s.axl == nil {
		return nil, fmt.Errorf("agent-to-agent delegation is not configured on this node")
	}

	taskID := fmt.Sprintf("%s-%d", s.ID, len(s.Actions))
	topic := sessionTopic(s.ID)

	payer := ""
	if s.agentWallet != "" {
		payer = s.agentWallet
	}

	task := delegatedTask{
		Type:      "task.assigned",
		TaskID:    taskID,
		SessionID: s.ID,
		Command:   command,
		Image:     image,
		Payer:     payer,
	}
	body, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}

	// Listen before publishing so a fast peer cannot answer into the void.
	results := make(chan delegatedResult, 4)
	subCtx, cancel := context.WithTimeout(ctx, DelegationTimeout)
	defer cancel()

	if err := s.axl.Subscribe(subCtx, topic, func(msg []byte) {
		var res delegatedResult
		if err := json.Unmarshal(msg, &res); err != nil {
			return
		}
		if res.Type != "task.complete" || res.TaskID != taskID {
			return
		}
		select {
		case results <- res:
		default:
		}
	}); err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", topic, err)
	}

	s.emit(Event{Type: "message", Message: fmt.Sprintf("Delegating subtask %s to a peer agent over AXL…", taskID)})
	if err := s.axl.Publish(ctx, topic, body); err != nil {
		return nil, fmt.Errorf("publish %s: %w", topic, err)
	}

	select {
	case res := <-results:
		if res.Error != "" {
			return nil, fmt.Errorf("peer agent failed the subtask: %s", res.Error)
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Peer agent %s returned subtask %s", shortAddr(res.Worker), taskID)})
		return map[string]any{
			"task_id":     res.TaskID,
			"output":      res.Output,
			"action_hash": res.ActionHash,
			"worker":      res.Worker,
			"delegated":   true,
		}, nil
	case <-subCtx.Done():
		log.Printf("[session %s] delegation %s timed out", s.ID, taskID)
		return nil, fmt.Errorf("no peer agent answered within %s", DelegationTimeout)
	}
}
