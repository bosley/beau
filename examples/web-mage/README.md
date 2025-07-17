# Web Mage Example

This example demonstrates how to use the Web Mage to capture screenshots of websites using Chromium.

## Prerequisites

1. **Chromium/Chrome Browser**: The web mage requires Chromium or Google Chrome to be installed:
   ```bash
   # Ubuntu/Pop!_OS/Debian
   sudo apt install chromium-browser
   
   # macOS
   brew install --cask chromium
   ```

2. **API Key**: Set your XAI API key:
   ```bash
   export XAI_API_KEY="your-api-key-here"
   ```

## Usage

### Basic Usage
```bash
# Build the example
go build -o web-mage-example main.go

# Run with required --target flag
./web-mage-example --target /path/to/output/directory
```

### Custom URLs
You can specify custom URLs as arguments:
```bash
./web-mage-example --target ~/Desktop https://example.com https://golang.org
```

### Output Structure
Screenshots are saved to:
```
<target-directory>/
└── .web/
    └── screenshots/
        ├── example.com_20250717_070024.png
        ├── example.com_20250717_070024_metadata.json
        └── ...
```

## Example Commands

The web mage understands natural language commands:
- "Navigate to https://example.com and take a fullpage screenshot"
- "Capture https://site.com with a viewport screenshot, wait 5 seconds"
- "Go to https://page.com and capture the whole page"

## Metadata

Each screenshot is accompanied by a metadata JSON file containing:
- URL captured
- Screenshot type (fullpage/viewport)
- Viewport dimensions
- Timestamp
- File path and size

## Troubleshooting

### Chrome Not Found
If you get "executable file not found", ensure Chromium is installed and in your PATH:
```bash
which chromium chromium-browser google-chrome
```

### Permission Errors
The web mage runs Chrome in headless mode with sandbox disabled for compatibility. If you encounter permission issues, check that your user can execute Chromium.

### Snap Installation
On Ubuntu/Pop!_OS, Chromium is often installed as a snap. The toolkit automatically detects snap installations at `/snap/bin/chromium`. 