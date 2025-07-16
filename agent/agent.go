package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/bosley/beau"
	"github.com/bosley/beau/mage"
	"github.com/bosley/beau/toolkit"
)

type Agent interface {
	Start(ctx context.Context) error
	InterruptCurrentRequest() error
	ResetConversation() error
	SendMessage(message string) error
}

type Observer interface {
	OnChunk(chunk beau.StreamChunk) error
	OnError(err error) error
	OnComplete(message beau.Message) error
	OnUsage(usage UsageStats) error
}

// UsageStats contains token and size information for tracking usage
type UsageStats struct {
	MessageSizeBytes int    // Total size of the message data sent to LLM
	Model            string // Model used for this request
	TokensUsed       int    // Estimated tokens used (if available from API)
	PromptTokens     int    // Input tokens
	CompletionTokens int    // Output tokens
}

type Config struct {
	Logger        *slog.Logger
	Observer      Observer
	APIKey        string
	BaseURL       string
	HTTPClient    *http.Client
	RetryConfig   beau.RetryConfig
	Model         string
	ImageModel    string // if empty will use the same as the model
	ProjectBounds []beau.ProjectBounds
	Temperature   float64
	MaxTokens     int

	PromptRefinements []string
}

type agent struct {
	config  Config
	logger  *slog.Logger
	client  *beau.Client
	conv    *beau.Conversation
	portal  *mage.Portal
	toolkit *toolkit.LlmToolKit

	// For managing concurrent operations
	mu            sync.Mutex
	activeRequest context.CancelFunc
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
}

func (a *agent) calculateMessageSize() int {
	messages := a.conv.GetMessages()
	totalSize := 0
	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			totalSize += len(content)
		case []interface{}:
			if jsonBytes, err := json.Marshal(content); err == nil {
				totalSize += len(jsonBytes)
			}
		}

		totalSize += len(string(msg.Role))
		totalSize += len(msg.Name)
		totalSize += len(msg.ToolCallID)

		for _, tc := range msg.ToolCalls {
			totalSize += len(tc.ID)
			totalSize += len(tc.Type)
			totalSize += len(tc.Function.Name)
			totalSize += len(tc.Function.Arguments)
		}
	}

	// Add some overhead for JSON structure
	totalSize = int(float64(totalSize) * 1.2)
	return totalSize
}

// estimateTokens provides a rough estimate of tokens based on character count
// This is a simple heuristic: ~4 characters per token on average
func (a *agent) estimateTokens(size int) int {
	return size / 4
}

func NewAgent(config Config) (Agent, error) {
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewJSONHandler(nil, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	if config.ImageModel == "" {
		config.ImageModel = config.Model
	}

	if config.Temperature == 0 {
		config.Temperature = 0.7
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 8192
	}

	client, err := beau.NewClient(
		config.APIKey,
		config.BaseURL,
		config.HTTPClient,
		config.Logger.WithGroup("beau_client"),
		config.RetryConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create beau client: %w", err)
	}

	portal := mage.NewPortal(mage.PortalConfig{
		Logger:        config.Logger.WithGroup("mage_portal"),
		APIKey:        config.APIKey,
		BaseURL:       config.BaseURL,
		HTTPClient:    config.HTTPClient,
		RetryConfig:   config.RetryConfig,
		PrimaryModel:  config.Model,
		ImageModel:    config.ImageModel,
		MiniModel:     config.Model, // Use same model for mini tasks
		ProjectBounds: config.ProjectBounds,
		MaxTokens:     8192,
		Temperature:   0.7,
	})

	a := &agent{
		config: config,
		logger: config.Logger.WithGroup("agent"),
		client: client,
		portal: portal,
	}

	a.toolkit = mage.GetUnifiedMageKit(
		portal,
		config.Logger.WithGroup("mage_kit"),
		a.handleToolCallback,
	)

	// Initialize conversation
	a.resetConversation()

	return a, nil
}

func (a *agent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("agent already running")
	}

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.running = true

	// No longer need to start stream handler since we create streams per request

	a.logger.Info("Agent started")
	return nil
}

func (a *agent) InterruptCurrentRequest() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.activeRequest != nil {
		a.logger.Info("Interrupting current request")
		a.activeRequest()
		a.activeRequest = nil
	}

	return nil
}

func (a *agent) ResetConversation() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.resetConversation()
}

func (a *agent) resetConversation() error {
	// Cancel any active request
	if a.activeRequest != nil {
		a.activeRequest()
		a.activeRequest = nil
	}

	// Create new conversation with tools
	a.conv = a.client.NewConversation(a.config.Model,
		beau.WithTools(a.toolkit.GetTools()),
		beau.WithToolChoice("auto"),
	)

	// Debug log the tools being registered
	tools := a.toolkit.GetTools()
	a.logger.Debug("Registered tools with conversation", "toolCount", len(tools))
	for _, tool := range tools {
		a.logger.Debug("Tool registered", "name", tool.Function.Name, "description", tool.Function.Description)
	}

	proompt := `You are a helpful AI assistant with access to tools for file operations and image analysis,
	aimed at helping a user explore and work on their project.

When users ask about files, directories, or images, you should use the available tools to help them.
If they request ambiguous use your best judgement to leverage the tools to help them. If you require clarification, ask for it but
try to not be worthless fool. Actually think and consider in context what it is they are trying to do.

To use a tool, you must call it using the proper tool calling format. The main tool available is 'task_mage' which can perform various operations:
- For file operations (list, read, write files), use: mage_type='filesystem'
- For image analysis, use: mage_type='image'

Note: ALL file paths MUST be absolute paths. The mage will fail if you use relative paths.

Note: If attempt to write a file, list afterwards to ensure that it was written correctly. Do not assume. Do not lie and say
its there if it is not. We expect honest, hard working, thoughtful, and thorough agents.

Always use tools when appropriate to fulfill user requests. Be helpful and conversational.

`
	if len(a.config.PromptRefinements) > 0 {
		proompt += "# Further Instructions/ Refinements to instructions\n"
		for _, refinement := range a.config.PromptRefinements {
			proompt += refinement + "\n"
		}
	}

	a.conv.AddSystemMessage(proompt)

	a.logger.Info("Conversation reset")
	return nil
}

func (a *agent) SendMessage(message string) error {
	a.mu.Lock()

	if !a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent not started")
	}

	if a.activeRequest != nil {
		a.mu.Unlock()
		return fmt.Errorf("request already in progress")
	}

	reqCtx, reqCancel := context.WithCancel(a.ctx)
	a.activeRequest = reqCancel
	a.mu.Unlock()

	a.conv.AddUserMessage(message)

	go func() {
		defer func() {
			a.mu.Lock()
			a.activeRequest = nil
			a.mu.Unlock()
		}()

		a.logger.Info("Sending message", "message", message)

		streamChan := make(chan beau.StreamChunk, 100)

		go a.handleStreamForRequest(streamChan)

		response, err := a.conv.Send(reqCtx,
			a.config.Temperature,
			a.config.MaxTokens,
			beau.WithStream(streamChan),
		)

		if err != nil {
			if reqCtx.Err() != nil {
				a.logger.Info("Request cancelled")
				return
			}
			a.logger.Error("Failed to send message", "error", err)
			if a.config.Observer != nil {
				a.config.Observer.OnError(err)
			}
			return
		}

		if len(response.ToolCalls) > 0 {
			a.logger.Info("Handling tool calls", "count", len(response.ToolCalls))
			if err := a.toolkit.HandleResponseCalls(response); err != nil {
				a.logger.Error("Failed to handle tool calls", "error", err)
				if a.config.Observer != nil {
					a.config.Observer.OnError(err)
				}
			}
		} else {
			if a.config.Observer != nil {
				a.config.Observer.OnComplete(*response)

				// Send usage stats
				usage := UsageStats{
					MessageSizeBytes: a.calculateMessageSize(),
					Model:            a.config.Model,
					TokensUsed:       a.estimateTokens(a.calculateMessageSize()),
				}
				a.config.Observer.OnUsage(usage)
			}
		}
	}()

	return nil
}

func (a *agent) handleStreamForRequest(streamChan chan beau.StreamChunk) {
	for chunk := range streamChan {
		if a.config.Observer != nil {
			if chunk.Error != nil {
				a.config.Observer.OnError(chunk.Error)
			} else if !chunk.Done {
				a.config.Observer.OnChunk(chunk)
			}
		}
	}
}

func (a *agent) handleToolCallback(isError bool, id string, result interface{}) {
	a.logger.Info("Tool callback", "id", id, "isError", isError)

	var resultStr string
	if isError {
		resultStr = fmt.Sprintf("Error: %v", result)
	} else {
		resultStr = fmt.Sprintf("%v", result)
	}

	a.conv.AddToolResult(id, resultStr)

	go func() {
		reqCtx, reqCancel := context.WithCancel(a.ctx)
		a.mu.Lock()
		a.activeRequest = reqCancel
		a.mu.Unlock()

		defer func() {
			a.mu.Lock()
			a.activeRequest = nil
			a.mu.Unlock()
		}()

		streamChan := make(chan beau.StreamChunk, 100)

		go a.handleStreamForRequest(streamChan)

		response, err := a.conv.Send(reqCtx,
			a.config.Temperature,
			a.config.MaxTokens,
			beau.WithStream(streamChan),
		)

		if err != nil {
			if reqCtx.Err() != nil {
				a.logger.Info("Request cancelled during tool response")
				return
			}
			a.logger.Error("Failed to send tool response", "error", err)
			if a.config.Observer != nil {
				a.config.Observer.OnError(err)
			}
			return
		}

		// Check for more tool calls
		if len(response.ToolCalls) > 0 {
			a.logger.Info("More tool calls needed", "count", len(response.ToolCalls))
			if err := a.toolkit.HandleResponseCalls(response); err != nil {
				a.logger.Error("Failed to handle additional tool calls", "error", err)
				if a.config.Observer != nil {
					a.config.Observer.OnError(err)
				}
			}
		} else {
			// Final response complete
			if a.config.Observer != nil {
				a.config.Observer.OnComplete(*response)

				// Send usage stats
				usage := UsageStats{
					MessageSizeBytes: a.calculateMessageSize(),
					Model:            a.config.Model,
					TokensUsed:       a.estimateTokens(a.calculateMessageSize()),
				}
				a.config.Observer.OnUsage(usage)
			}
		}
	}()
}
