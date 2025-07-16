package beau

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrMissingAPIKey        = fmt.Errorf("apiKey is required")
	ErrMissingBaseURL       = fmt.Errorf("baseURL is required")
	ErrNoResponseChoices    = fmt.Errorf("no response choices returned")
	ErrReadImageFile        = fmt.Errorf("failed to read image file")
	ErrReadRequestBody      = fmt.Errorf("failed to read request body")
	ErrMaxRetriesExceeded   = fmt.Errorf("max retries exceeded")
	ErrMarshalRequest       = fmt.Errorf("failed to marshal request")
	ErrCreateRequest        = fmt.Errorf("failed to create request")
	ErrReadResponseBody     = fmt.Errorf("failed to read response body")
	ErrUnexpectedStatusCode = fmt.Errorf("API returned unexpected status code")
	ErrUnmarshalResponse    = fmt.Errorf("failed to unmarshal response")
	ErrReadStream           = fmt.Errorf("error reading stream")
)

func NewClient(
	apiKey string,
	baseURL string,
	httpClient *http.Client,
	logger *slog.Logger,
	retryConfig RetryConfig,
) (*Client, error) {

	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	if baseURL == "" {
		return nil, ErrMissingBaseURL
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: DefaultTimeout,
		}
	}

	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	return &Client{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		HTTPClient:  httpClient,
		Logger:      logger,
		RetryConfig: retryConfig,
	}, nil
}

func (x *Client) WithLogger(logger *slog.Logger) *Client {
	x.Logger = logger
	return x
}

func (x *Client) WithBaseURL(baseURL string) *Client {
	x.BaseURL = baseURL
	return x
}

func (x *Client) WithHTTPClient(client *http.Client) *Client {
	x.HTTPClient = client
	return x
}

func (x *Client) WithRetryConfig(config RetryConfig) *Client {
	x.RetryConfig = config
	return x
}

func CreateTextMessage(role MessageRole, content string) Message {
	return Message{
		Role:    role,
		Content: content,
	}
}

func CreateComplexMessage(role MessageRole, items []ContentItem) Message {
	return Message{
		Role:    role,
		Content: items,
	}
}

func CreateTextItem(text string) ContentItem {
	return ContentItem{
		Type: ContentTypeText,
		Text: text,
	}
}

func CreateImageURLItem(url string, detail string) ContentItem {
	if detail == "" {
		detail = "auto"
	}
	return ContentItem{
		Type: ContentTypeImageURL,
		ImageURL: &ImageURL{
			URL:    url,
			Detail: detail,
		},
	}
}

func CreateImageBase64Item(base64Image, mimeType, detail string) ContentItem {
	if detail == "" {
		detail = "auto"
	}
	if !strings.HasPrefix(base64Image, "data:") {
		base64Image = fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image)
	}
	return ContentItem{
		Type: ContentTypeImageURL,
		ImageURL: &ImageURL{
			URL:    base64Image,
			Detail: detail,
		},
	}
}

func CreateToolCallItem(id, toolType, functionName, arguments string) ContentItem {
	return ContentItem{
		Type: ContentTypeTool,
		ToolCall: &ToolCall{
			ID:   id,
			Type: toolType,
			Function: ToolFunction{
				Name:      functionName,
				Arguments: arguments,
			},
		},
	}
}

func CreateToolResultItem(toolCallID, content string) ContentItem {
	return ContentItem{
		Type: ContentTypeToolResult,
		ToolResult: &ToolResult{
			ToolCallID: toolCallID,
			Content:    content,
		},
	}
}

func ReadImageFile(filePath string) (string, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrReadImageFile, err)
	}

	base64Str := base64.StdEncoding.EncodeToString(data)

	ext := strings.ToLower(filepath.Ext(filePath))
	mimeType := "image/jpeg" // Default

	if ext == ".png" {
		mimeType = "image/png"
	} else if ext == ".jpg" || ext == ".jpeg" {
		mimeType = "image/jpeg"
	}

	return base64Str, mimeType, nil
}

func parseRetryAfter(retryAfterHeader string) time.Duration {
	if seconds, err := time.ParseDuration(retryAfterHeader + "s"); err == nil {
		return seconds
	}
	if t, err := http.ParseTime(retryAfterHeader); err == nil {
		return time.Until(t)
	}
	return DefaultRateLimitDuration
}

func (x *Client) doRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	delay := x.RetryConfig.InitialDelay

	for attempt := 0; attempt <= x.RetryConfig.MaxRetries; attempt++ {
		reqCopy := req.Clone(ctx)
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrReadRequestBody, err)
			}
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			reqCopy.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := x.HTTPClient.Do(reqCopy)
		if err != nil {
			lastErr = err
			if attempt < x.RetryConfig.MaxRetries {
				x.Logger.Warn("Request failed, retrying",
					"attempt", attempt+1,
					"error", err,
					"delay", delay)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
					delay = time.Duration(float64(delay) * x.RetryConfig.BackoffFactor)
					if delay > x.RetryConfig.MaxDelay {
						delay = x.RetryConfig.MaxDelay
					}
					continue
				}
			}
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests && x.RetryConfig.Enabled && attempt < x.RetryConfig.MaxRetries {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			retryAfter := delay
			if retryAfterHeader := resp.Header.Get("Retry-After"); retryAfterHeader != "" {
				retryAfter = parseRetryAfter(retryAfterHeader)
			}

			if retryAfter > delay {
				delay = retryAfter
			}

			if delay > x.RetryConfig.MaxDelay {
				delay = x.RetryConfig.MaxDelay
			}

			x.Logger.Warn("Rate limited, retrying",
				"attempt", attempt+1,
				"statusCode", resp.StatusCode,
				"delay", delay,
				"responseBody", string(body))

			lastErr = &RateLimitError{
				StatusCode:   resp.StatusCode,
				RetryAfter:   delay,
				ResponseBody: string(body),
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				delay = time.Duration(float64(delay) * x.RetryConfig.BackoffFactor)
				continue
			}
		}

		return resp, nil
	}

	return nil, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

func (x *Client) Send(ctx context.Context, temperature float64, maxTokens int, messages []Message, model string, opts ...RequestOption) (*ChatCompletionResponse, error) {
	req := ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	for _, opt := range opts {
		opt(&req)
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMarshalRequest, err)
	}

	x.Logger.Debug("Sending request to Client", "model", model, "messageCount", len(messages))

	cleanBaseURL := strings.TrimSuffix(x.BaseURL, "/")
	fullURL := fmt.Sprintf("%s/v1/chat/completions", cleanBaseURL)

	httpReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fullURL,
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateRequest, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", x.APIKey))

	// Add Anthropic-specific headers if using Claude
	if strings.Contains(x.BaseURL, "anthropic.com") {
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		// For Anthropic, use x-api-key instead of Bearer token
		httpReq.Header.Set("x-api-key", x.APIKey)
		httpReq.Header.Del("Authorization")
	}

	// If streaming is enabled and we have a channel, handle streaming
	if req.Stream && req.streamConfig != nil && req.streamConfig.Channel != nil {
		x.Logger.Debug("Using streaming response with channel")
		return x.handleStreamingResponse(ctx, httpReq, req.streamConfig.Channel)
	}

	resp, err := x.doRequestWithRetry(ctx, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadResponseBody, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d: %s", ErrUnexpectedStatusCode, resp.StatusCode, string(body))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnmarshalResponse, err)
	}

	if len(result.Choices) > 0 && result.Choices[0].FinishReason == "length" {
		x.Logger.Warn("Response was truncated due to length limits",
			"finishReason", result.Choices[0].FinishReason,
			"model", model)
	}

	return &result, nil
}

func (x *Client) handleStreamingResponse(ctx context.Context, req *http.Request, stream chan StreamChunk) (*ChatCompletionResponse, error) {
	resp, err := x.doRequestWithRetry(ctx, req)
	if err != nil {
		stream <- StreamChunk{Error: err}
		close(stream)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("%w: %d: %s", ErrUnexpectedStatusCode, resp.StatusCode, string(body))
		stream <- StreamChunk{Error: err}
		close(stream)
		return nil, err
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullMessage string
	var toolCalls []ToolCall

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			stream <- StreamChunk{Error: ctx.Err()}
			close(stream)
			return nil, ctx.Err()
		default:
			line := scanner.Text()
			if line == "" {
				continue
			}

			line = strings.TrimPrefix(line, "data: ")

			if line == "[DONE]" {
				stream <- StreamChunk{Done: true}
				close(stream)
				return &ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: Message{
								Role:      RoleAssistant,
								Content:   fullMessage,
								ToolCalls: toolCalls,
							},
						},
					},
				}, nil
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				x.Logger.Debug("Failed to parse chunk", "error", err, "line", line)
				continue
			}

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta

				// Handle content streaming
				if delta.Content != "" {
					stream <- StreamChunk{Content: delta.Content}
					fullMessage += delta.Content
				}

				// Handle tool calls
				if len(delta.ToolCalls) > 0 {
					// Merge tool calls - streaming may send partial tool calls
					for _, tc := range delta.ToolCalls {
						if tc.ID != "" {
							// New tool call
							toolCalls = append(toolCalls, tc)
						} else if len(toolCalls) > 0 {
							// Update last tool call with additional data
							lastIdx := len(toolCalls) - 1
							if tc.Function.Name != "" {
								toolCalls[lastIdx].Function.Name = tc.Function.Name
							}
							if tc.Function.Arguments != "" {
								toolCalls[lastIdx].Function.Arguments += tc.Function.Arguments
							}
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		stream <- StreamChunk{Error: fmt.Errorf("%w: %v", ErrReadStream, err)}
		close(stream)
		return nil, err
	}

	return &ChatCompletionResponse{
		Choices: []Choice{
			{
				Message: Message{
					Role:      RoleAssistant,
					Content:   fullMessage,
					ToolCalls: toolCalls,
				},
			},
		},
	}, nil
}

type Conversation struct {
	client   *Client
	messages []Message
	model    string
	options  []RequestOption
}

func (x *Client) NewConversation(model string, opts ...RequestOption) *Conversation {
	return &Conversation{
		client:   x,
		messages: []Message{},
		model:    model,
		options:  opts,
	}
}

func (c *Conversation) AddMessage(message Message) *Conversation {
	c.messages = append(c.messages, message)
	return c
}

func (c *Conversation) AddSystemMessage(content string) *Conversation {
	return c.AddMessage(CreateTextMessage(RoleSystem, content))
}

func (c *Conversation) AddUserMessage(content string) *Conversation {
	return c.AddMessage(CreateTextMessage(RoleUser, content))
}

func (c *Conversation) AddAssistantMessage(content string) *Conversation {
	return c.AddMessage(CreateTextMessage(RoleAssistant, content))
}

func (c *Conversation) AddComplexUserMessage(items []ContentItem) *Conversation {
	return c.AddMessage(CreateComplexMessage(RoleUser, items))
}

func (c *Conversation) AddToolResult(toolCallID string, result string) *Conversation {
	return c.AddMessage(Message{
		Role:       RoleTool,
		Content:    result,
		ToolCallID: toolCallID,
	})
}

func (c *Conversation) Send(ctx context.Context, temperature float64, maxTokens int, opts ...RequestOption) (*Message, error) {
	finalOptions := append(c.options, opts...)
	response, err := c.client.Send(ctx, temperature, maxTokens, c.messages, c.model, finalOptions...)
	if err != nil {
		return nil, err
	}

	if len(response.Choices) == 0 {
		return nil, ErrNoResponseChoices
	}

	message := response.Choices[0].Message

	// Check if the response was truncated
	if response.Choices[0].FinishReason == "length" || response.Choices[0].FinishReason == "max_tokens" {
		c.client.Logger.Warn("Response truncated",
			"finishReason", response.Choices[0].FinishReason,
			"contentLength", len(fmt.Sprintf("%v", message.Content)))
	}
	c.messages = append(c.messages, message)
	return &message, nil
}

func (c *Conversation) GetMessages() []Message {
	return c.messages
}

type RequestOption func(*ChatCompletionRequest)

func WithTemperature(temperature float64) RequestOption {
	return func(req *ChatCompletionRequest) {
		req.Temperature = temperature
	}
}

func WithMaxTokens(maxTokens int) RequestOption {
	return func(req *ChatCompletionRequest) {
		req.MaxTokens = maxTokens
	}
}

func WithTools(tools []Tool) RequestOption {
	return func(req *ChatCompletionRequest) {
		req.Tools = tools
	}
}

func WithStream(channel chan StreamChunk) RequestOption {
	return func(req *ChatCompletionRequest) {
		req.Stream = true
		req.streamConfig = &StreamConfig{
			Enabled: true,
			Channel: channel,
		}
	}
}

func WithToolChoice(toolChoice interface{}) RequestOption {
	return func(req *ChatCompletionRequest) {
		req.ToolChoice = toolChoice
	}
}
