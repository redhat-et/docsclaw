package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// A2AClient wraps the a2aclient to send requests to A2A agents.
type A2AClient struct {
	httpClient *http.Client
	log        *slog.Logger
}

// NewA2AClient creates a new A2A client wrapper.
func NewA2AClient(httpClient *http.Client, log *slog.Logger) *A2AClient {
	return &A2AClient{
		httpClient: httpClient,
		log:        log,
	}
}

// InvokeRequest holds the parameters for invoking an A2A agent.
type InvokeRequest struct {
	AgentURL      string
	Card          *a2a.AgentCard
	DocumentID    string // Legacy: used by Go agents
	MessageText   string // New: used by gateway mode (e.g., "Summarize s3://...")
	ReviewType    string
	TaskID        string // Continue an existing task/session
	BearerToken   string // Optional JWT forwarded from the caller (user delegation)
	UserSPIFFEID  string // User SPIFFE ID — sent as X-Delegation-User header
	AgentSPIFFEID string // Agent SPIFFE ID — sent as X-Delegation-Agent header
}

// InvokeResult holds the response from an A2A agent invocation.
type InvokeResult struct {
	Text   string `json:"text"`
	State  string `json:"state"`
	TaskID string `json:"task_id,omitempty"`
}

// Invoke sends a message/send request to an A2A agent.
func (c *A2AClient) Invoke(ctx context.Context, req *InvokeRequest) (*InvokeResult, error) {
	var msg *a2a.Message
	if req.MessageText != "" {
		msg = a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(req.MessageText))
	} else {
		// Legacy mode: send structured DataPart
		data := map[string]any{
			"document_id": req.DocumentID,
		}
		if req.ReviewType != "" {
			data["review_type"] = req.ReviewType
		}
		msg = a2a.NewMessage(a2a.MessageRoleUser, a2a.NewDataPart(data))
	}

	if req.TaskID != "" {
		msg.TaskID = a2a.TaskID(req.TaskID)
	}

	params := &a2a.SendMessageRequest{
		Message: msg,
	}

	// Create a2a client using the A2A URL with explicit JSONRPC transport.
	opts := []a2aclient.FactoryOption{
		a2aclient.WithDefaultsDisabled(),
		a2aclient.WithJSONRPCTransport(c.httpClient),
	}

	// Forward the bearer token and delegation context as HTTP headers
	// via a CallInterceptor that injects ServiceParams.
	sp := a2aclient.ServiceParams{}
	if req.BearerToken != "" {
		sp.Append("authorization", "Bearer "+req.BearerToken)
	}
	if req.UserSPIFFEID != "" {
		sp.Append("x-delegation-user", req.UserSPIFFEID)
	}
	if req.AgentSPIFFEID != "" {
		sp.Append("x-delegation-agent", req.AgentSPIFFEID)
	}
	if len(sp) > 0 {
		opts = append(opts, a2aclient.WithCallInterceptors(&serviceParamsInjector{params: sp}))
	}

	endpoints := []*a2a.AgentInterface{
		a2a.NewAgentInterface(req.AgentURL, a2a.TransportProtocolJSONRPC),
	}
	client, err := a2aclient.NewFromEndpoints(ctx, endpoints, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}

	c.log.Info("Sending A2A message/send",
		"url", req.AgentURL,
		"document_id", req.DocumentID)

	result, err := client.SendMessage(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("A2A message/send failed: %w", err)
	}

	return c.parseResult(result)
}

// serviceParamsInjector injects ServiceParams into outbound requests.
type serviceParamsInjector struct {
	a2aclient.PassthroughInterceptor
	params a2aclient.ServiceParams
}

func (i *serviceParamsInjector) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	for k, vals := range i.params {
		req.ServiceParams.Append(k, vals...)
	}
	return ctx, nil, nil
}

// parseResult extracts text from a SendMessageResult (Task or Message).
func (c *A2AClient) parseResult(result a2a.SendMessageResult) (*InvokeResult, error) {
	switch r := result.(type) {
	case *a2a.Task:
		return c.parseTask(r)
	case *a2a.Message:
		return c.parseMessage(r)
	default:
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
}

func (c *A2AClient) parseTask(task *a2a.Task) (*InvokeResult, error) {
	state := string(task.Status.State)
	taskID := string(task.ID)

	// Check for failure or rejection
	if task.Status.State == a2a.TaskStateFailed || task.Status.State == a2a.TaskStateRejected {
		reason := "agent returned " + state
		if task.Status.Message != nil {
			if text := extractTextFromParts(task.Status.Message.Parts); text != "" {
				reason = text
			}
		}
		return &InvokeResult{Text: reason, State: state, TaskID: taskID}, nil
	}

	// Extract text from artifacts
	for _, artifact := range task.Artifacts {
		if text := extractTextFromParts(artifact.Parts); text != "" {
			return &InvokeResult{Text: text, State: state, TaskID: taskID}, nil
		}
	}

	// Fall back to status message
	if task.Status.Message != nil {
		if text := extractTextFromParts(task.Status.Message.Parts); text != "" {
			return &InvokeResult{Text: text, State: state, TaskID: taskID}, nil
		}
	}

	return &InvokeResult{Text: "", State: state, TaskID: taskID}, nil
}

func (c *A2AClient) parseMessage(msg *a2a.Message) (*InvokeResult, error) {
	text := extractTextFromParts(msg.Parts)
	return &InvokeResult{Text: text, State: "completed"}, nil
}

// extractTextFromParts concatenates text from all Parts in a part list.
func extractTextFromParts(parts a2a.ContentParts) string {
	var texts []string
	for _, part := range parts {
		if t := part.Text(); t != "" {
			texts = append(texts, t)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	result := texts[0]
	for _, t := range texts[1:] {
		result += "\n" + t
	}
	return result
}
