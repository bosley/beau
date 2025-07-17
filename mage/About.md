# Mage System

The mage library implements specialized AI assistants ("mages") that can be summoned to perform specific tasks. Each mage has its own toolkit and expertise, allowing for modular and focused functionality.

## Overview

The mage system takes the concept of an "agent" and encapsulates it behind a "tool" interface. This allows an AI to delegate specialized tasks to purpose-built AI assistants, each with their own tools and context.

## Available Mages

### 1. Filesystem Mage (Mage_FS)
Handles all file system operations with safety and intelligence.
- Read, write, and analyze files
- Navigate directories
- Search and replace content
- Handle large files intelligently

### 2. Image Mage (Mage_IM)
Specialized in visual analysis and image understanding.
- Analyze images using vision models
- Describe visual content
- Answer questions about images
- Identify objects, text, and patterns

### 3. Web Mage (Mage_WB)
Browser automation and web content capture.
- Navigate to websites
- Capture screenshots (full page or viewport)
- Save screenshots with metadata
- Future: interact with web elements

### 4. Shell Mage (Mage_SH)
Platform-aware command execution and system interaction.
- Execute shell commands safely
- Manage processes and environment
- Create executable scripts
- Adapt to different operating systems

## Architecture

Each mage follows a consistent pattern:
1. **Portal**: Central factory for summoning mages
2. **Mage Interface**: Common interface for all mages
3. **Toolkit**: Specialized tools for each mage's domain
4. **Context Management**: Each mage maintains its own conversation context

## Usage Pattern

```go
// Create a portal
portal := mage.NewPortal(config)

// Summon a specific mage
myMage, err := portal.Summon(mage.Mage_FS) // or Mage_IM, Mage_WB, Mage_SH

// Add context if needed
myMage.AddToContext("Additional instructions...")

// Execute natural language commands
result, err := myMage.Execute(ctx, "List all Python files")
```

## Benefits

1. **Separation of Concerns**: Each mage focuses on its domain
2. **Tool Specialization**: Purpose-built tools for each use case
3. **Scalability**: Easy to add new mages for new domains
4. **Safety**: Built-in safety features (timeouts, bounds checking)
5. **Platform Awareness**: Mages adapt to their environment