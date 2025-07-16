package toolkit

import (
	"fmt"

	"github.com/bosley/beau"

	"github.com/fatih/color"
)

type LlmTool interface {
	// GetDefinition returns the definition of the tool.
	GetDefinition() beau.Tool

	// We already know that its this tool so we pass the args and let it handle
	// the decoding/ etc as required.
	// Should usually return a string but can return anything that can be encoded
	// to json.
	// LIMITATION: This interface doesn't support context cancellation. Tools
	// cannot be cancelled when the user clicks stop. This needs architectural
	// change to add context.Context parameter.
	Call(input []byte) (interface{}, error)
}

// Called if a tool is executed. It will execute each tool present in the given
// hjands back the id that was called with result
// if isError defined, then the result should be considered an error type
type KitCallback func(isError bool, id string, result interface{})

type LlmToolKit struct {
	name  string    // the name of the toolkit
	tools []LlmTool // All available tools in the kit

	// flattened from tools[] for use in calling xai
	ironedTools []beau.Tool

	callback KitCallback
}

func NewKit(name string) *LlmToolKit {
	return &LlmToolKit{
		name:        name,
		tools:       []LlmTool{},
		ironedTools: []beau.Tool{},
		callback:    nil,
	}
}

func (x *LlmToolKit) WithTool(tool LlmTool) *LlmToolKit {
	x.tools = append(x.tools, tool)
	return x
}

func (x *LlmToolKit) WithCallback(callback KitCallback) *LlmToolKit {
	x.callback = callback
	return x
}

func (x *LlmToolKit) GetTools() []beau.Tool {
	if len(x.ironedTools) > 0 {
		return x.ironedTools
	}
	for _, tool := range x.tools {
		x.ironedTools = append(x.ironedTools, tool.GetDefinition())
	}
	return x.ironedTools
}

func (x *LlmToolKit) HandleResponseCalls(response *beau.Message) error {
	if x.callback == nil {
		return fmt.Errorf("no callback set")
	}
	for _, toolCall := range response.ToolCalls {
		toolFound := false
		for _, tool := range x.tools {
			if tool.GetDefinition().Function.Name == toolCall.Function.Name {
				toolFound = true
				color.HiYellow("Executing tool: %s", toolCall.Function.Name)
				color.HiCyan("Args: %s", toolCall.Function.Arguments)
				result, err := tool.Call([]byte(toolCall.Function.Arguments))
				if err != nil {
					color.HiRed("Error executing tool %s: %s", toolCall.Function.Name, err)
					// Pass the error to the callback as an error
					x.callback(true, toolCall.ID, err)
				} else {
					color.HiGreen("Tool %s executed successfully", toolCall.Function.Name)
					x.callback(false, toolCall.ID, result)
				}
				break
			}
		}
		if !toolFound {
			errMsg := fmt.Sprintf("Tool '%s' not found in toolkit", toolCall.Function.Name)
			color.HiRed(errMsg)
			x.callback(true, toolCall.ID, fmt.Errorf("%s", errMsg))
		}
	}
	return nil
}
