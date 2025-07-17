package mage // micro agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bosley/beau"
)

var (
	DefaultMaxTokens   = 8192
	DefaultTemperature = 0.7
)

type PortalConfig struct {
	Logger  *slog.Logger
	APIKey  string
	BaseURL string

	HTTPClient  *http.Client
	RetryConfig beau.RetryConfig

	PrimaryModel string // Must be able to do function calling
	ImageModel   string // For image understanding
	MiniModel    string // For quick tasks - no function calling (summarize, etc)

	MaxTokens   int
	Temperature float64

	// Project bounds for path validation
	ProjectBounds []beau.ProjectBounds
}

type MageVariant string

const (
	Mage_FS MageVariant = "mage_fs"
	Mage_IM MageVariant = "mage_im"
	Mage_WB MageVariant = "mage_web"
)

type Mage interface {
	// Reset the llm chat history and all settings
	Reset() error

	// Add context to the mages SYSTEM Prompt
	AddToContext(context string) error

	// Add the user inquery/ command whatever it is and get a response
	Execute(ctx context.Context, command string) (string, error)
}

type Portal struct {
	logger      *slog.Logger
	apiKey      string
	baseURL     string
	HTTPClient  *http.Client
	RetryConfig beau.RetryConfig

	primaryModel string
	imageModel   string
	miniModel    string

	maxTokens   int
	temperature float64

	// Project bounds for path validation
	projectBounds []beau.ProjectBounds
}

func NewPortal(config PortalConfig) *Portal {
	if config.MaxTokens == 0 {
		config.MaxTokens = DefaultMaxTokens
	}
	if config.Temperature == 0 {
		config.Temperature = DefaultTemperature
	}

	return &Portal{
		logger:        config.Logger,
		apiKey:        config.APIKey,
		baseURL:       config.BaseURL,
		HTTPClient:    config.HTTPClient,
		RetryConfig:   config.RetryConfig,
		primaryModel:  config.PrimaryModel,
		imageModel:    config.ImageModel,
		miniModel:     config.MiniModel,
		maxTokens:     config.MaxTokens,
		temperature:   config.Temperature,
		projectBounds: config.ProjectBounds,
	}
}

func (p *Portal) Summon(variant MageVariant) (Mage, error) {
	switch variant {
	case Mage_FS:
		return newFSMage(p)
	case Mage_IM:
		return newIMMage(p)
	case Mage_WB:
		return newWebMage(p)
	default:
		return nil, fmt.Errorf("unknown variant: %s", variant)
	}
}

// ----------------------------------------
