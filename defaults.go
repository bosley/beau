package beau

import "time"

const (
	DefaultBaseURL_XAI    = "https://api.x.ai"
	DefaultModel_XAI      = "grok-4"
	DefaultBaseURL_Claude = "https://api.anthropic.com"
	DefaultModel_Claude   = "claude-3-5-sonnet-20240620"
	DefaultBaseURL_OpenAI = "https://api.openai.com"
	DefaultModel_OpenAI   = "gpt-4o"
	DefaultTemperature    = 0.7
	DefaultMaxTokens      = 8000

	// Retry configuration
	DefaultMaxRetries    = 3
	DefaultInitialDelay  = 1 * time.Second
	DefaultMaxDelay      = 32 * time.Second
	DefaultBackoffFactor = 2.0

	DefaultTimeout = 10 * time.Minute

	DefaultRateLimitDuration = 30 * time.Second
)
