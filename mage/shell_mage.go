package mage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
	"github.com/bosley/beau/toolkit/shellkit"
)

type shellMage struct {
	portal       *Portal
	client       *beau.Client
	conversation *beau.Conversation
	kit          *toolkit.LlmToolKit

	contextMessages []string
	resultBuilder   strings.Builder
}

var _ Mage = &shellMage{}

func newShellMage(portal *Portal) (*shellMage, error) {
	logger := portal.logger.WithGroup("shell_mage")

	client, err := beau.NewClient(portal.apiKey, portal.baseURL, portal.HTTPClient, logger, portal.RetryConfig)
	if err != nil {
		return nil, err
	}

	shellMage := &shellMage{
		portal:          portal,
		client:          client,
		conversation:    nil,
		kit:             nil,
		contextMessages: []string{},
		resultBuilder:   strings.Builder{},
	}

	// Initialize the shell kit
	shellMage.kit = shellkit.GetShellKit(logger, func(isError bool, id string, result interface{}) {
		if isError {
			logger.Error("Tool execution error", "id", id, "error", result)
			shellMage.conversation.AddToolResult(id, fmt.Sprintf("Error: %v", result))
			shellMage.resultBuilder.WriteString(fmt.Sprintf("Error: %v\n", result))
		} else {
			logger.Info("Tool execution result", "id", id)

			// Handle different result types
			switch v := result.(type) {
			case string:
				shellMage.conversation.AddToolResult(id, v)
				shellMage.resultBuilder.WriteString(v + "\n")
			case map[string]interface{}:
				// Format command results nicely
				if stdout, ok := v["stdout"].(string); ok {
					output := stdout
					if stderr, ok := v["stderr"].(string); ok && stderr != "" {
						output += "\nStderr: " + stderr
					}
					if exitCode, ok := v["exit_code"].(int); ok && exitCode != 0 {
						output += fmt.Sprintf("\nExit code: %d", exitCode)
					}
					shellMage.conversation.AddToolResult(id, output)
					shellMage.resultBuilder.WriteString(output + "\n")
				} else {
					// Generic map handling
					formatted := formatMapResult(v)
					shellMage.conversation.AddToolResult(id, formatted)
					shellMage.resultBuilder.WriteString(formatted + "\n")
				}
			default:
				result := fmt.Sprintf("%v", v)
				shellMage.conversation.AddToolResult(id, result)
				shellMage.resultBuilder.WriteString(result + "\n")
			}
		}
	}, portal.projectBounds)

	err = shellMage.resetConversationInternals()
	if err != nil {
		return nil, err
	}

	return shellMage, nil
}

func formatMapResult(m map[string]interface{}) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}
	return strings.Join(parts, "\n")
}

func (m *shellMage) resetConversationInternals() error {
	m.resultBuilder.Reset()
	m.contextMessages = []string{}

	m.conversation = m.client.NewConversation(m.portal.primaryModel,
		beau.WithTemperature(m.portal.temperature),
		beau.WithMaxTokens(m.portal.maxTokens),
		beau.WithTools(m.kit.GetTools()),
		beau.WithToolChoice("auto"),
	)

	return nil
}

func (m *shellMage) Reset() error {
	return m.resetConversationInternals()
}

func (m *shellMage) AddToContext(context string) error {
	m.contextMessages = append(m.contextMessages, context)
	return nil
}

func (m *shellMage) Execute(ctx context.Context, command string) (string, error) {
	// Reset result builder for this execution
	m.resultBuilder.Reset()

	// Add platform-specific context
	platformContext := fmt.Sprintf(`You are running on %s/%s. The shell is %s.
Project directory: %s`,
		runtime.GOOS, runtime.GOARCH,
		getShellName(),
		m.portal.projectBounds[0].ABSPath)

	m.conversation.AddSystemMessage(platformContext)

	// Add custom context messages
	for _, msg := range m.contextMessages {
		m.conversation.AddSystemMessage(msg)
	}

	m.conversation.AddUserMessage(command)

	// Execute with tool calls
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("execution cancelled: %w", ctx.Err())
		default:
		}

		response, err := m.conversation.Send(ctx, m.portal.temperature, m.portal.maxTokens)
		if err != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("execution cancelled: %w", ctx.Err())
			}
			return "", fmt.Errorf("failed to get response: %w", err)
		}

		if len(response.ToolCalls) == 0 {
			asString, ok := response.Content.(string)
			if !ok {
				return "", fmt.Errorf("response content is not a string")
			}
			m.resultBuilder.WriteString(asString)
			break
		}

		m.portal.logger.Info("Found tool calls", "count", len(response.ToolCalls))
		m.kit.HandleResponseCalls(response)
	}

	return m.resultBuilder.String(), nil
}

func getShellName() string {
	if runtime.GOOS == "windows" {
		return "Windows Command Prompt or PowerShell"
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "sh"
	}
	return filepath.Base(shell)
}
