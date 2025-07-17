# Web Mage

The Web Mage is a specialized browser automation assistant that can navigate websites, capture screenshots, and interact with web pages using Chromium.

## Features

### Primary Capability: Navigate and Screenshot
The main tool `navigate_and_screenshot` combines navigation and screenshot capture in one operation:
- Navigate to any URL
- Capture full page or viewport screenshots
- Configurable wait times for page loading
- Custom viewport dimensions
- Automatic filename generation with timestamps
- Metadata JSON files with capture details

### Screenshot Storage
All screenshots are automatically saved to:
```
<project_directory>/.web/screenshots/
```

Each screenshot includes:
- PNG image file with timestamp
- Accompanying metadata JSON file
- Cleaned URL in filename for easy identification

### Available Tools

1. **navigate_and_screenshot** - Navigate to URL and capture screenshot
   - Parameters: url, screenshot_type, wait_seconds, viewport_width, viewport_height
   - This is the primary tool that works fully

2. **navigate_to_url** - Navigate without screenshot
3. **take_screenshot** - Screenshot current page
4. **click_element** - Click elements by CSS selector
5. **fill_form** - Fill form fields
6. **execute_javascript** - Run JavaScript on page
7. **wait_for_element** - Wait for elements to appear
8. **get_page_info** - Get page title, URL, etc.

Note: Tools 2-8 are placeholders for future browser session management features.

## Usage Example

```go
// Summon the web mage
webMage, err := portal.Summon(mage.Mage_WB)

// Execute commands
result, err := webMage.Execute(ctx, "Navigate to https://example.com and take a fullpage screenshot")
```

## Common Commands

- "Navigate to https://example.com and take a fullpage screenshot"
- "Capture https://site.com with viewport screenshot, wait 5 seconds"
- "Go to https://page.com and capture the whole page with 1280x720 viewport"

## Technical Details

The Web Mage uses:
- **chromedp** for Chrome DevTools Protocol communication
- Headless Chromium for rendering
- Automatic browser management (launch/cleanup)
- PNG format for screenshots at 90% quality

## Future Enhancements

- Browser session persistence for multi-step interactions
- Element-specific screenshots
- Form filling and interaction workflows
- JavaScript execution for dynamic content
- Cookie and authentication management
- PDF generation
- Network request interception 