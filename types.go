package beau

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// RateLimitError represents a 429 rate limit error with retry information
type RateLimitError struct {
	StatusCode   int
	RetryAfter   time.Duration
	ResponseBody string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded (429): retry after %v", e.RetryAfter)
}

// RetryConfig defines the retry behavior for rate-limited requests
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	Enabled       bool
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    DefaultMaxRetries,
		InitialDelay:  DefaultInitialDelay,
		MaxDelay:      DefaultMaxDelay,
		BackoffFactor: DefaultBackoffFactor,
		Enabled:       true,
	}
}

type Client struct {
	APIKey      string
	BaseURL     string
	HTTPClient  *http.Client
	Logger      *slog.Logger
	RetryConfig RetryConfig
}

// MessageRole defines the role of a message in a conversation
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// ContentType defines the type of content in a message
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImageURL   ContentType = "image_url"
	ContentTypeTool       ContentType = "tool_call"
	ContentTypeToolResult ContentType = "tool_result"
)

// ContentItem represents a single piece of content in a message
type ContentItem struct {
	Type       ContentType `json:"type"`
	Text       string      `json:"text,omitempty"`
	ImageURL   *ImageURL   `json:"image_url,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// ImageURL represents an image URL content item
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Message represents a message in a conversation
type Message struct {
	Role       MessageRole `json:"role"`
	Content    interface{} `json:"content"`
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from the model
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents a function call within a tool call
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// StreamConfig represents streaming configuration
type StreamConfig struct {
	Enabled bool
	Channel chan StreamChunk
}

// StreamChunk represents a chunk of streamed response
type StreamChunk struct {
	Content string
	Error   error
	Done    bool
}

// ChatCompletionRequest represents a request to the chat completions API
type ChatCompletionRequest struct {
	Model        string        `json:"model"`
	Messages     []Message     `json:"messages"`
	MaxTokens    int           `json:"max_tokens,omitempty"`
	Temperature  float64       `json:"temperature,omitempty"`
	Stream       bool          `json:"stream,omitempty"`
	Tools        []Tool        `json:"tools,omitempty"`
	ToolChoice   interface{}   `json:"tool_choice,omitempty"`
	streamConfig *StreamConfig `json:"-"` // Internal use only
}

// Tool represents a tool that the model can use
type Tool struct {
	Type     string     `json:"type"`
	Function ToolSchema `json:"function"`
}

// ToolSchema defines the schema for a function tool
type ToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// PromptTokensDetails contains detailed token usage information
type PromptTokensDetails struct {
	ImageTokens int `json:"image_tokens"`
}

// ChatCompletionResponse represents a response from the chat completions API
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// ProjectBounds defines the allowed directories for file operations
type ProjectBounds struct {
	Name        string
	Description string
	ABSPath     string
}
