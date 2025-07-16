package mage

import (
	"context"
	"fmt"
	"strings"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
	"github.com/bosley/beau/toolkit/fskit"
)

type fsMage struct {
	portal       *Portal
	client       *beau.Client
	conversation *beau.Conversation
	kit          *toolkit.LlmToolKit

	contextMessages []string

	resultBuilder strings.Builder
}

var _ Mage = &fsMage{}

func newFSMage(portal *Portal) (*fsMage, error) {

	logger := portal.logger
	client, err := beau.NewClient(portal.apiKey, portal.baseURL, portal.HTTPClient, logger, portal.RetryConfig)
	if err != nil {
		return nil, err
	}

	fsMage := &fsMage{
		portal:          portal,
		client:          client,
		conversation:    nil,
		kit:             nil,
		contextMessages: []string{},
		resultBuilder:   strings.Builder{},
	}

	err = fsMage.resetConversationInternals()
	if err != nil {
		return nil, err
	}

	return fsMage, nil
}

func (m *fsMage) resetConversationInternals() error {
	var conversation *beau.Conversation
	m.resultBuilder.Reset()
	m.contextMessages = []string{}

	// Use portal's project bounds directly

	// Use validated filesystem kit with project bounds
	kit := fskit.GetValidatedFsKit(m.portal.projectBounds, func(isError bool, id string, result interface{}) {

		if isError {
			m.portal.logger.Error("Tool execution error", "id", id, "error", result)
		} else {
			m.portal.logger.Info("Tool execution result", "id", id, "result", result)

			/*
				Hand tool call result to the conversation (directly no summary)
			*/
			switch v := result.(type) {
			case string:
				conversation.AddToolResult(id, v)
				m.resultBuilder.WriteString(v)

			case []byte:
				conversation.AddToolResult(id, string(v))
				m.resultBuilder.WriteString(string(v))
			default:
				conversation.AddToolResult(id, fmt.Sprintf("%v", v))
				m.resultBuilder.WriteString(fmt.Sprintf("%v", v))
			}
		}
	})
	conversation = m.client.NewConversation(m.portal.primaryModel,
		beau.WithTemperature(m.portal.temperature),
		beau.WithMaxTokens(m.portal.maxTokens),
		beau.WithTools(kit.GetTools()),
		beau.WithToolChoice("auto"),
	)
	m.conversation = conversation
	m.kit = kit
	return nil
}

func (m *fsMage) Reset() error {
	err := m.resetConversationInternals()
	if err != nil {
		return err
	}
	return nil
}

func (m *fsMage) AddToContext(context string) error {
	m.contextMessages = append(m.contextMessages, context)
	return nil
}

func (m *fsMage) Execute(ctx context.Context, command string) (string, error) {

	// Add project bounds information as a system message
	if len(m.portal.projectBounds) > 0 {
		var projectInfo strings.Builder
		projectInfo.WriteString("You have access to the following project directories:\n")
		for _, pb := range m.portal.projectBounds {
			projectInfo.WriteString(fmt.Sprintf("- %s: %s (use absolute path: %s)\n", pb.Name, pb.Description, pb.ABSPath))
		}
		projectInfo.WriteString("\nAlways use the full absolute paths when working with files. Do not use generic paths like /home/user/project.")
		m.conversation.AddSystemMessage(projectInfo.String())
	}

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

		// If there are no tool calls, print the response and break
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
