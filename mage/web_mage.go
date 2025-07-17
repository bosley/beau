package mage

import (
	"context"
	"fmt"
	"strings"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
	"github.com/bosley/beau/toolkit/webkit"
)

type webMage struct {
	portal       *Portal
	client       *beau.Client
	conversation *beau.Conversation
	kit          *toolkit.LlmToolKit

	contextMessages []string
	resultBuilder   strings.Builder
}

var _ Mage = &webMage{}

func newWebMage(portal *Portal) (*webMage, error) {
	logger := portal.logger.WithGroup("web_mage")

	client, err := beau.NewClient(portal.apiKey, portal.baseURL, portal.HTTPClient, logger, portal.RetryConfig)
	if err != nil {
		return nil, err
	}

	webMage := &webMage{
		portal:          portal,
		client:          client,
		conversation:    nil,
		kit:             nil,
		contextMessages: []string{},
		resultBuilder:   strings.Builder{},
	}

	// Initialize the web kit with portal's configuration
	webMage.kit = webkit.GetWebKit(logger, func(isError bool, id string, result interface{}) {
		if isError {
			logger.Error("Tool execution error", "id", id, "error", result)
		} else {
			logger.Info("Tool execution result", "id", id)

			// Add tool result to the conversation
			switch v := result.(type) {
			case string:
				webMage.conversation.AddToolResult(id, v)
				webMage.resultBuilder.WriteString(v)
			case map[string]interface{}:
				// For structured results, convert to readable format
				if success, ok := v["success"].(bool); ok && success {
					if msg, ok := v["message"].(string); ok {
						webMage.conversation.AddToolResult(id, msg)
						webMage.resultBuilder.WriteString(msg + "\n")
					}
					if screenshot, ok := v["screenshot"].(string); ok {
						webMage.resultBuilder.WriteString(fmt.Sprintf("Screenshot saved: %s\n", screenshot))
					}
				}
			default:
				webMage.conversation.AddToolResult(id, fmt.Sprintf("%v", v))
				webMage.resultBuilder.WriteString(fmt.Sprintf("%v\n", v))
			}
		}
	}, portal.projectBounds)

	err = webMage.resetConversationInternals()
	if err != nil {
		return nil, err
	}

	return webMage, nil
}

func (m *webMage) resetConversationInternals() error {
	m.resultBuilder.Reset()
	m.contextMessages = []string{}

	// Create conversation with the primary model
	m.conversation = m.client.NewConversation(m.portal.primaryModel,
		beau.WithTemperature(m.portal.temperature),
		beau.WithMaxTokens(m.portal.maxTokens),
		beau.WithTools(m.kit.GetTools()),
		beau.WithToolChoice("auto"),
	)

	return nil
}

func (m *webMage) Reset() error {
	return m.resetConversationInternals()
}

func (m *webMage) AddToContext(context string) error {
	m.contextMessages = append(m.contextMessages, context)
	return nil
}

func (m *webMage) Execute(ctx context.Context, command string) (string, error) {
	// Add project bounds information
	if len(m.portal.projectBounds) > 0 {
		var projectInfo strings.Builder
		projectInfo.WriteString("Screenshots will be saved to: ")
		for _, pb := range m.portal.projectBounds {
			projectInfo.WriteString(fmt.Sprintf("%s/.web/screenshots/\n", pb.ABSPath))
		}
		m.conversation.AddSystemMessage(projectInfo.String())
	}

	// Add system messages
	for _, msg := range m.contextMessages {
		m.conversation.AddSystemMessage(msg)
	}

	m.conversation.AddUserMessage(command)

	// Loop to handle the conversation with function calls
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("execution cancelled: %w", ctx.Err())
		default:
		}

		// Get a response from the model
		m.portal.logger.Info("Sending message to model")
		response, err := m.conversation.Send(ctx, m.portal.temperature, m.portal.maxTokens)
		if err != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("execution cancelled: %w", ctx.Err())
			}
			return "", fmt.Errorf("failed to get response: %w", err)
		}

		// If there are no tool calls, append the response and break
		if len(response.ToolCalls) == 0 {
			asString, ok := response.Content.(string)
			if !ok {
				return "", fmt.Errorf("response content is not a string")
			}
			m.resultBuilder.WriteString(asString)
			break
		}

		m.portal.logger.Info("Found tool calls", "count", len(response.ToolCalls))

		// Handle the tool calls
		m.kit.HandleResponseCalls(response)
	}

	return m.resultBuilder.String(), nil
}
