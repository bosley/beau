package mage

import (
	"context"
	"fmt"
	"strings"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
	"github.com/bosley/beau/toolkit/imkit"
)

type imMage struct {
	portal       *Portal
	client       *beau.Client
	conversation *beau.Conversation
	kit          *toolkit.LlmToolKit

	contextMessages []string

	resultBuilder strings.Builder
}

var _ Mage = &imMage{}

func newIMMage(portal *Portal) (*imMage, error) {
	logger := portal.logger.WithGroup("im_mage")

	// Create XAI client for conversation
	client, err := beau.NewClient(portal.apiKey, portal.baseURL, portal.HTTPClient, logger, portal.RetryConfig)
	if err != nil {
		return nil, err
	}

	imMage := &imMage{
		portal:          portal,
		client:          client,
		conversation:    nil,
		kit:             nil,
		contextMessages: []string{},
		resultBuilder:   strings.Builder{},
	}

	// Initialize the image kit with portal's API configuration
	imMage.kit = imkit.GetImageKit(portal.apiKey, portal.baseURL, logger, func(isError bool, id string, result interface{}) {
		if isError {
			logger.Error("Tool execution error", "id", id, "error", result)
		} else {
			logger.Info("Tool execution result", "id", id, "result", result)

			// Add tool result to the conversation
			switch v := result.(type) {
			case string:
				imMage.conversation.AddToolResult(id, v)
				imMage.resultBuilder.WriteString(v)
			case []byte:
				imMage.conversation.AddToolResult(id, string(v))
				imMage.resultBuilder.WriteString(string(v))
			default:
				imMage.conversation.AddToolResult(id, fmt.Sprintf("%v", v))
				imMage.resultBuilder.WriteString(fmt.Sprintf("%v", v))
			}
		}
	}, portal.imageModel, portal.projectBounds) // Using portal's image model and project bounds

	err = imMage.resetConversationInternals()
	if err != nil {
		return nil, err
	}

	return imMage, nil
}

func (m *imMage) resetConversationInternals() error {
	m.resultBuilder.Reset()
	m.contextMessages = []string{}

	// Create conversation with the primary model (for orchestration)
	m.conversation = m.client.NewConversation(m.portal.primaryModel,
		beau.WithTemperature(m.portal.temperature),
		beau.WithMaxTokens(m.portal.maxTokens),
		beau.WithTools(m.kit.GetTools()),
		beau.WithToolChoice("auto"),
	)

	return nil
}

func (m *imMage) Reset() error {
	err := m.resetConversationInternals()
	if err != nil {
		return err
	}
	return nil
}

func (m *imMage) AddToContext(context string) error {
	m.contextMessages = append(m.contextMessages, context)
	return nil
}

func (m *imMage) Execute(ctx context.Context, command string) (string, error) {
	// Add project bounds information as a system message
	if len(m.portal.projectBounds) > 0 {
		var projectInfo strings.Builder
		projectInfo.WriteString("You have access to the following project directories for image files:\n")
		for _, pb := range m.portal.projectBounds {
			projectInfo.WriteString(fmt.Sprintf("- %s: %s (use absolute path: %s)\n", pb.Name, pb.Description, pb.ABSPath))
		}
		projectInfo.WriteString("\nAlways use the full absolute paths when working with image files. Do not use generic paths like /home/user/project.")
		m.conversation.AddSystemMessage(projectInfo.String())
	}

	// Add system messages
	for _, msg := range m.contextMessages {
		m.conversation.AddSystemMessage(msg)
	}

	m.conversation.AddUserMessage(command)

	// Loop to handle the conversation with function calls
	for {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("execution cancelled: %w", ctx.Err())
		default:
		}

		// Get a response from the model
		m.portal.logger.Info("Sending message to model")
		response, err := m.conversation.Send(ctx, m.portal.temperature, m.portal.maxTokens)
		if err != nil {
			// Check if it's a context cancellation
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
			// Append the final response to the result builder
			m.resultBuilder.WriteString(asString)
			break
		}

		m.portal.logger.Info("Found tool calls", "count", len(response.ToolCalls))

		// Handle the tool calls
		m.kit.HandleResponseCalls(response)
	}

	return m.resultBuilder.String(), nil
}
