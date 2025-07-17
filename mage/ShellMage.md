# Shell Mage

The Shell Mage is a platform-aware command execution assistant that can run system commands, manage processes, and create scripts across different operating systems.

## Features

### Platform Detection
- Automatically detects OS (Windows, Linux, macOS, etc.)
- Identifies available shell (bash, zsh, PowerShell, cmd)
- Provides platform-specific command syntax
- Adapts to system architecture

### Available Tools

1. **execute_command** - Run shell commands with safety features
   - Timeout protection (default 30s, max 300s)
   - Working directory support
   - Environment variable injection
   - Captures stdout, stderr, and exit codes

2. **list_processes** - List and filter running processes
   - Platform-appropriate process listing
   - Case-insensitive filtering
   - Shows PIDs and process names

3. **get_environment** - Access environment variables
   - Filter by pattern
   - Show common variables by default
   - Option to show all variables

4. **get_working_directory** - Current directory information
   - Shows absolute path
   - Optional directory listing
   - Separates files and directories

5. **get_system_info** - Detailed system information
   - Platform details (OS, architecture)
   - Shell information
   - Go runtime info
   - PATH directories

6. **create_script** - Generate executable scripts
   - Platform-appropriate shebangs
   - Automatic file extensions (.sh, .ps1, .bat)
   - Proper permissions (755 on Unix)
   - Descriptive headers

## Usage Example

```go
// Summon the shell mage
shellMage, err := portal.Summon(mage.Mage_SH)

// Execute commands
result, err := shellMage.Execute(ctx, "List all Python files in the current directory")
```

## Common Commands

### Cross-Platform
- "Show me the system information"
- "List all files in the current directory"
- "Show environment variables"
- "Create a script to automate this task"

### Linux/macOS Specific
- "Execute 'ps aux | grep python' to find Python processes"
- "Run 'df -h' to check disk space"
- "Create a bash script for deployment"

### Windows Specific
- "Execute 'dir /s *.txt' to find all text files"
- "Show me running services"
- "Create a PowerShell script"

## Safety Features

1. **Timeout Protection**: Commands automatically timeout after 30 seconds by default
2. **Project Bounds**: File operations restricted to project directories
3. **Output Limits**: Large outputs are managed to prevent overwhelming responses
4. **Exit Code Tracking**: All commands report their exit status

## Platform Differences

### POSIX Systems (Linux, macOS)
- Uses `/bin/sh` or user's `$SHELL`
- Supports standard Unix utilities
- Script shebang: `#!/usr/bin/env <shell>`

### Windows
- Detects PowerShell or Command Prompt
- PowerShell preferred when available
- Scripts get `.ps1` or `.bat` extensions

## Best Practices

1. **Check Platform First**: Use `get_system_info` if unsure about the environment
2. **Use Timeouts**: Set appropriate timeouts for long-running commands
3. **Verify Output**: Check exit codes and stderr for errors
4. **Create Scripts**: For complex workflows, create reusable scripts
5. **Environment Safety**: Be cautious with environment variable modifications

## Technical Implementation

The Shell Mage uses Go's `os/exec` package with:
- Context-based cancellation
- Proper signal handling
- Cross-platform command execution
- Secure subprocess isolation 