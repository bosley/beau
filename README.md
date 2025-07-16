# Beau

Beau is a Go library for building AI agents that use specialized 'mages' as tools for tasks like filesystem operations and image analysis. It allows AI to delegate tasks to other AI components via tools, enabling complex workflows.

Repository: github.com/bosley/beau

## Features

- AI client for chat completions (supports OpenAI, Anthropic, XAI) with streaming, tool calls, retries, and multimodal support.
- Mage system with portal for summoning mages (e.g., filesystem, image).
- Agent implementation for conversation management and tool handling.
- CLI for interactive agent use.
- Toolkits for filesystem, image, and path utilities.
- Examples and generated outputs (e.g., Snake game HTML).

## Installation

```sh
go get github.com/bosley/beau
```

## Using the Mage System Independently

The `examples/simple-mage` demonstrates standalone mage usage. It summons a filesystem mage and executes a query to list files.

### Example

```go
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bosley/beau"
	"github.com/bosley/beau/mage"
	"github.com/fatih/color"
)

func main() {
	// (abbreviated for README) Setup logger, flags for API config, directory, etc.
	// Retrieve API key, create HTTP client.

	dirAsABS, err := filepath.Abs(directory)
	if err != nil {
		// handle error
	}

	portal := mage.NewPortal(mage.PortalConfig{
		// config with APIKey, BaseURL, Model, etc.
		ProjectBounds: []beau.ProjectBounds{{
			Name: "project",
			Description: "The project directory",
			ABSPath: dirAsABS,
		}},
	})

	tulpa, err := portal.Summon(mage.Mage_FS)
	if err != nil {
		// handle error
	}

	tulpa.AddToContext("You are a helpful assistant that can help with tasks related to the project directory.")

	result, err := tulpa.Execute(context.Background(), "What files do you see. DOnt read them. Just list them.")
	if err != nil {
		// handle error
	}

	color.HiGreen("Result: %s", result)
}
```

Run with flags, e.g., `./simple-mage -dir /path/to/project -model grok-3`

## Using the Agent System

The agent is used via the CLI in `cmd/beau-cli`. It supports providers like XAI, OpenAI, with interactive chat.

### Example

```sh
go build cmd/beau-cli/main.go
./main -provider xai -dir /project
```

In the CLI, type queries like "list files" or press Ctrl+C to interrupt.

### Code Example

```go
// From cmd/beau-cli/main.go (summarized)
config := agent.Config{
	// APIKey, BaseURL, Model, etc.
	ProjectBounds: []beau.ProjectBounds{ /* ... */ },
}

ag, err := agent.NewAgent(config)
if err != nil {
	// handle
}
ag.Start(context.Background())
ag.SendMessage("Your query here")
```

For more, check generated_examples/Snake80/index.html (agent-generated, just like this readme.)