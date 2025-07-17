package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/bosley/beau"
	"github.com/bosley/beau/agent"
	"github.com/fatih/color"
)

// CliObserver implements the Observer interface for CLI output
type CliObserver struct {
	mu       sync.Mutex
	complete chan bool
}

func NewCliObserver() *CliObserver {
	return &CliObserver{
		complete: make(chan bool, 1),
	}
}

func (o *CliObserver) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Drain the channel if needed
	select {
	case <-o.complete:
	default:
	}
}

func (o *CliObserver) OnChunk(chunk beau.StreamChunk) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Print streaming content without buffering
	if chunk.Content != "" {
		fmt.Print(chunk.Content)
		os.Stdout.Sync() // Force flush
	}
	return nil
}

func (o *CliObserver) OnError(err error) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	color.Red("\n❌ Error: %v\n", err)
	// Don't block on channel if it's already been used
	select {
	case o.complete <- true:
	default:
	}
	return nil
}

func (o *CliObserver) OnComplete(message beau.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Debug: print the entire message
	if os.Getenv("DEBUG_AGENT") != "" {
		fmt.Printf("\n[DEBUG] Complete message: %+v\n", message)
	}

	// Show tool calls if any
	if len(message.ToolCalls) > 0 {
		color.Yellow("\n🔧 Executing tools...\n")
		for _, tc := range message.ToolCalls {
			color.Cyan("  • %s\n", tc.Function.Name)
		}
	} else {
		// Check if there's content to display
		if content, ok := message.Content.(string); ok && content != "" {
			// Content was already streamed, just add newline
			fmt.Println()
		} else {
			// No content was streamed, might be a tool-only response
			color.Yellow("\n[Waiting for tool execution...]\n")
		}
	}

	// Don't block on channel if it's already been used
	select {
	case o.complete <- true:
	default:
	}
	return nil
}

func (o *CliObserver) OnUsage(usage agent.UsageStats) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Display usage statistics
	color.HiBlack("\n📊 Usage: %d bytes sent | ~%d tokens | Model: %s\n",
		usage.MessageSizeBytes, usage.TokensUsed, usage.Model)
	return nil
}

func (o *CliObserver) WaitForComplete() {
	<-o.complete
}

func main() {

	var dir string
	var provider string
	var debug bool
	var temperature float64
	var maxTokens int

	flag.StringVar(&provider, "provider", "xai", "The provider to use")
	flag.StringVar(&dir, "dir", "", "The directory to use")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Float64Var(&temperature, "temp", 0.7, "Temperature for generation (0.0-2.0)")
	flag.IntVar(&maxTokens, "max-tokens", 8192, "Maximum tokens for generation")

	flag.Parse()

	// Setup logger with pretty printing for CLI
	logLevel := slog.LevelWarn
	if debug {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	var model string
	var baseURL string
	var apiKey string

	switch provider {
	case "openai":
		baseURL = beau.DefaultBaseURL_OpenAI
		model = beau.DefaultModel_OpenAI
		apiKey = os.Getenv("OPENAI_API_KEY")
	case "anthropic":
		baseURL = beau.DefaultBaseURL_Claude
		model = beau.DefaultModel_Claude
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	case "xai":
		baseURL = beau.DefaultBaseURL_XAI
		model = beau.DefaultModel_XAI
		apiKey = os.Getenv("XAI_API_KEY")
	default:
		color.Red("❌ Invalid provider: %s", provider)
		os.Exit(1)
	}

	fmt.Printf(`
	Provider: %s
	Model: %s
	BaseURL: %s
	APIKey: %s
	
	`,
		color.HiCyanString(provider),
		color.HiYellowString(model),
		color.HiGreenString(baseURL),
		color.HiRedString(apiKey[:10]+"..."),
	)

	if apiKey == "" {
		color.Red("❌ No API key found. Please set OPENAI_API_KEY, ANTHROPIC_API_KEY, or XAI_API_KEY")
		os.Exit(1)
	}

	var err error
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			color.Red("❌ Failed to get working directory: %v", err)
			os.Exit(1)
		}
	}

	// Create observer
	observer := NewCliObserver()

	// Configure the agent
	config := agent.Config{
		Logger:      logger,
		Observer:    observer,
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Model:       model,
		RetryConfig: beau.DefaultRetryConfig(),
		Temperature: temperature,
		MaxTokens:   maxTokens,

		// Restrict file operations to current directory
		ProjectBounds: []beau.ProjectBounds{
			{
				Name:        "workspace",
				Description: "Current working directory",
				ABSPath:     dir,
			},
		},
	}

	// Create and start the agent
	ag, err := agent.NewAgent(config)
	if err != nil {
		color.Red("❌ Failed to create agent: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := ag.Start(ctx); err != nil {
		color.Red("❌ Failed to start agent: %v", err)
		os.Exit(1)
	}

	// Print welcome message
	color.HiGreen("🤖 Beau Agent CLI")
	color.HiGreen("================")
	fmt.Printf("Model: %s\n", color.YellowString(model))
	fmt.Printf("Working Directory: %s\n", color.YellowString(dir))
	fmt.Println()
	color.HiWhite("Available tools:")
	color.White("  • Image analysis (analyze images)")
	color.White("  • File operations (read, write, list files)")
	color.White("  • Web browsing (capture screenshots of websites)")
	color.White("  • Shell commands (execute system commands)")
	fmt.Println()
	color.HiWhite("Commands:")
	color.White("  • Type your message and press Enter")
	color.White("  • Type 'reset' to start a new conversation")
	color.White("  • Type 'exit' or 'quit' to leave")
	color.White("  • Press Ctrl+C during generation to interrupt")
	fmt.Println()

	// Setup signal handler for graceful interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	activeRequest := false
	activeRequestMu := &sync.Mutex{}

	go func() {
		for sig := range sigChan {
			if sig == os.Interrupt {
				activeRequestMu.Lock()
				isActive := activeRequest
				activeRequestMu.Unlock()

				if isActive {
					// Try to interrupt current request
					if err := ag.InterruptCurrentRequest(); err == nil {
						color.Yellow("\n⚡ Request interrupted\n")
						color.HiBlue("> ")
					}
				} else {
					// No active request, exit program
					color.HiGreen("\n👋 Goodbye!\n")
					os.Exit(0)
				}
			}
		}
	}()

	// Main chat loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		color.HiBlue("\n> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		// Handle special commands
		switch strings.ToLower(input) {
		case "exit", "quit":
			color.HiGreen("\n👋 Goodbye!")
			os.Exit(0)
		case "reset":
			if err := ag.ResetConversation(); err != nil {
				color.Red("❌ Failed to reset: %v", err)
			} else {
				color.Green("✅ Conversation reset")
			}
			continue
		case "":
			continue
		}

		// Reset observer for new message
		observer.Reset()

		// Mark request as active
		activeRequestMu.Lock()
		activeRequest = true
		activeRequestMu.Unlock()

		// Send the message
		if err := ag.SendMessage(input); err != nil {
			color.Red("❌ Failed to send: %v", err)
			activeRequestMu.Lock()
			activeRequest = false
			activeRequestMu.Unlock()
			continue
		}

		// Wait for response to complete
		observer.WaitForComplete()

		// Mark request as inactive
		activeRequestMu.Lock()
		activeRequest = false
		activeRequestMu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		color.Red("❌ Scanner error: %v", err)
	}
}
