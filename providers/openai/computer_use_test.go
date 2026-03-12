package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/stretchr/testify/require"
)

func TestNewComputerUseTool(t *testing.T) {
	t.Parallel()

	tool := NewComputerUseTool()

	t.Run("has correct ID", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "openai.computer_use", tool.ID)
	})

	t.Run("has correct name", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "computer", tool.Name)
	})

	t.Run("has provider-defined type", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, fantasy.ToolTypeProviderDefined, tool.GetType())
	})
}

func TestIsComputerUseTool(t *testing.T) {
	t.Parallel()

	t.Run("returns true for computer use tool", func(t *testing.T) {
		t.Parallel()
		tool := NewComputerUseTool()
		require.True(t, IsComputerUseTool(tool))
	})

	t.Run("returns false for function tool", func(t *testing.T) {
		t.Parallel()
		tool := fantasy.FunctionTool{
			Name: "computer",
		}
		require.False(t, IsComputerUseTool(tool))
	})

	t.Run("returns false for other provider defined tool", func(t *testing.T) {
		t.Parallel()
		tool := fantasy.ProviderDefinedTool{
			ID:   "anthropic.computer_use",
			Name: "computer",
		}
		require.False(t, IsComputerUseTool(tool))
	})
}

func TestComputerUseMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	original := &ComputerUseMetadata{
		RawJSON: `{"some":"data"}`,
	}

	// Marshal produces the {type, data} envelope.
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal through the provider registry, which strips the
	// envelope and routes to the registered unmarshal function.
	raw := map[string]json.RawMessage{
		TypeComputerUseMetadata: data,
	}
	metadata, err := fantasy.UnmarshalProviderMetadata(raw)
	require.NoError(t, err)

	restored, ok := metadata[TypeComputerUseMetadata].(*ComputerUseMetadata)
	require.True(t, ok)
	require.Equal(t, original.RawJSON, restored.RawJSON)
}

func TestParseComputerCall(t *testing.T) {
	t.Parallel()

	t.Run("extracts call_id and actions", func(t *testing.T) {
		t.Parallel()
		raw := `{"type":"computer_call","call_id":"call_abc","actions":[{"type":"click","x":100,"y":200}]}`
		callID, input, err := parseComputerCall(raw)
		require.NoError(t, err)
		require.Equal(t, "call_abc", callID)
		require.JSONEq(t, `[{"type":"click","x":100,"y":200}]`, input)
	})

	t.Run("falls back to singular action", func(t *testing.T) {
		t.Parallel()
		raw := `{"type":"computer_call","call_id":"call_xyz","action":{"type":"screenshot"}}`
		callID, input, err := parseComputerCall(raw)
		require.NoError(t, err)
		require.Equal(t, "call_xyz", callID)
		require.JSONEq(t, `{"type":"screenshot"}`, input)
	})

	t.Run("returns empty object when no action data", func(t *testing.T) {
		t.Parallel()
		raw := `{"type":"computer_call","call_id":"call_empty"}`
		callID, input, err := parseComputerCall(raw)
		require.NoError(t, err)
		require.Equal(t, "call_empty", callID)
		require.Equal(t, "{}", input)
	})
}

func TestParseComputerActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantType   ComputerActionType
		validate   func(t *testing.T, action ComputerAction)
	}{
		{
			name:     "click",
			input:    `{"type":"click","x":100,"y":200,"button":"left"}`,
			wantType: ComputerActionClick,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(ClickAction)
				require.Equal(t, 100, a.X)
				require.Equal(t, 200, a.Y)
				require.Equal(t, "left", a.Button)
			},
		},
		{
			name:     "double_click",
			input:    `{"type":"double_click","x":300,"y":400}`,
			wantType: ComputerActionDoubleClick,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(DoubleClickAction)
				require.Equal(t, 300, a.X)
				require.Equal(t, 400, a.Y)
			},
		},
		{
			name:     "drag",
			input:    `{"type":"drag","start_x":10,"start_y":20,"path":[{"x":30,"y":40}]}`,
			wantType: ComputerActionDrag,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(DragAction)
				require.Equal(t, 10, a.StartX)
				require.Equal(t, 20, a.StartY)
				require.Len(t, a.Path, 1)
				require.Equal(t, 30, a.Path[0].X)
				require.Equal(t, 40, a.Path[0].Y)
			},
		},
		{
			name:     "keypress",
			input:    `{"type":"keypress","keys":["ctrl","c"]}`,
			wantType: ComputerActionKeypress,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(KeypressAction)
				require.Equal(t, []string{"ctrl", "c"}, a.Keys)
			},
		},
		{
			name:     "move",
			input:    `{"type":"move","x":500,"y":600}`,
			wantType: ComputerActionMove,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(MoveAction)
				require.Equal(t, 500, a.X)
				require.Equal(t, 600, a.Y)
			},
		},
		{
			name:     "screenshot",
			input:    `{"type":"screenshot"}`,
			wantType: ComputerActionScreenshot,
			validate: func(t *testing.T, action ComputerAction) {
				_, ok := action.(ScreenshotAction)
				require.True(t, ok)
			},
		},
		{
			name:     "scroll",
			input:    `{"type":"scroll","x":50,"y":50,"direction":"down","amount":3}`,
			wantType: ComputerActionScroll,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(ScrollAction)
				require.Equal(t, 50, a.X)
				require.Equal(t, 50, a.Y)
				require.Equal(t, "down", a.Direction)
				require.Equal(t, 3, a.Amount)
			},
		},
		{
			name:     "type",
			input:    `{"type":"type","text":"hello"}`,
			wantType: ComputerActionTyping,
			validate: func(t *testing.T, action ComputerAction) {
				a := action.(TypeAction)
				require.Equal(t, "hello", a.Text)
			},
		},
		{
			name:     "wait",
			input:    `{"type":"wait"}`,
			wantType: ComputerActionWait,
			validate: func(t *testing.T, action ComputerAction) {
				_, ok := action.(WaitAction)
				require.True(t, ok)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actions, err := ParseComputerActions(tc.input)
			require.NoError(t, err)
			require.Len(t, actions, 1)
			require.Equal(t, tc.wantType, actions[0].ActionType())
			tc.validate(t, actions[0])
		})
	}
}

func TestParseComputerActionsBatched(t *testing.T) {
	t.Parallel()

	input := `[{"type":"click","x":10,"y":20,"button":"left"},{"type":"type","text":"hello"},{"type":"keypress","keys":["enter"]}]`
	actions, err := ParseComputerActions(input)
	require.NoError(t, err)
	require.Len(t, actions, 3)
	require.Equal(t, ComputerActionClick, actions[0].ActionType())
	require.Equal(t, ComputerActionTyping, actions[1].ActionType())
	require.Equal(t, ComputerActionKeypress, actions[2].ActionType())

	click := actions[0].(ClickAction)
	require.Equal(t, 10, click.X)
	require.Equal(t, 20, click.Y)

	typ := actions[1].(TypeAction)
	require.Equal(t, "hello", typ.Text)

	kp := actions[2].(KeypressAction)
	require.Equal(t, []string{"enter"}, kp.Keys)
}

func TestParseComputerActionsInvalid(t *testing.T) {
	t.Parallel()

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComputerActions(`{not json}`)
		require.Error(t, err)
	})

	t.Run("unknown action type", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComputerActions(`{"type":"explode"}`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "explode")
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComputerActions("")
		require.Error(t, err)
	})
}

// computerHandlerCall records a single call to the mock handler.
type computerHandlerCall struct {
	method string
	action ComputerAction
}

// mockComputerHandler implements ComputerHandler and records all
// calls for verification.
type mockComputerHandler struct {
	calls []computerHandlerCall
	errOn string // return error when this method is called
}

func (m *mockComputerHandler) record(method string, action ComputerAction) []byte {
	m.calls = append(m.calls, computerHandlerCall{method: method, action: action})
	return []byte("screenshot_after_" + method)
}

func (m *mockComputerHandler) maybeErr(method string) error {
	if m.errOn == method {
		return fmt.Errorf("%s failed", method)
	}
	return nil
}

func (m *mockComputerHandler) Click(_ context.Context, a ClickAction) ([]byte, error) {
	return m.record("click", a), m.maybeErr("click")
}

func (m *mockComputerHandler) DoubleClick(_ context.Context, a DoubleClickAction) ([]byte, error) {
	return m.record("double_click", a), m.maybeErr("double_click")
}

func (m *mockComputerHandler) Drag(_ context.Context, a DragAction) ([]byte, error) {
	return m.record("drag", a), m.maybeErr("drag")
}

func (m *mockComputerHandler) Keypress(_ context.Context, a KeypressAction) ([]byte, error) {
	return m.record("keypress", a), m.maybeErr("keypress")
}

func (m *mockComputerHandler) Move(_ context.Context, a MoveAction) ([]byte, error) {
	return m.record("move", a), m.maybeErr("move")
}

func (m *mockComputerHandler) Screenshot(_ context.Context, a ScreenshotAction) ([]byte, error) {
	return m.record("screenshot", a), m.maybeErr("screenshot")
}

func (m *mockComputerHandler) Scroll(_ context.Context, a ScrollAction) ([]byte, error) {
	return m.record("scroll", a), m.maybeErr("scroll")
}

func (m *mockComputerHandler) Type(_ context.Context, a TypeAction) ([]byte, error) {
	return m.record("type", a), m.maybeErr("type")
}

func (m *mockComputerHandler) Wait(_ context.Context, a WaitAction) ([]byte, error) {
	return m.record("wait", a), m.maybeErr("wait")
}

func TestExecuteComputerActions(t *testing.T) {
	t.Parallel()

	t.Run("single click", func(t *testing.T) {
		t.Parallel()
		h := &mockComputerHandler{}
		ss, err := ExecuteComputerActions(
			t.Context(), h,
			`{"type":"click","x":100,"y":200,"button":"left"}`,
		)
		require.NoError(t, err)
		require.Equal(t, []byte("screenshot_after_click"), ss)
		require.Len(t, h.calls, 1)
		require.Equal(t, "click", h.calls[0].method)
		click := h.calls[0].action.(ClickAction)
		require.Equal(t, 100, click.X)
		require.Equal(t, 200, click.Y)
		require.Equal(t, "left", click.Button)
	})

	t.Run("batched actions returns last screenshot", func(t *testing.T) {
		t.Parallel()
		h := &mockComputerHandler{}
		ss, err := ExecuteComputerActions(
			t.Context(), h,
			`[{"type":"click","x":10,"y":20,"button":"left"},{"type":"type","text":"hello"},{"type":"screenshot"}]`,
		)
		require.NoError(t, err)
		require.Equal(t, []byte("screenshot_after_screenshot"), ss)
		require.Len(t, h.calls, 3)
		require.Equal(t, "click", h.calls[0].method)
		require.Equal(t, "type", h.calls[1].method)
		require.Equal(t, "screenshot", h.calls[2].method)
	})

	t.Run("error propagation", func(t *testing.T) {
		t.Parallel()
		h := &mockComputerHandler{errOn: "type"}
		_, err := ExecuteComputerActions(
			t.Context(), h,
			`[{"type":"click","x":1,"y":2,"button":"left"},{"type":"type","text":"x"}]`,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "type failed")
		// Click was called before the error.
		require.Len(t, h.calls, 2)
	})
}

// TestComputerUseAgentLoop exercises the full Generate -> tool-call
// -> rebuild-prompt -> Generate cycle with a mock HTTP server that
// returns computer_call actions covering every action type.
func TestComputerUseAgentLoop(t *testing.T) {
	t.Parallel()

	type step struct {
		actionType string
		rawItem    string
	}

	steps := []step{
		{"click", `{
			"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed",
			"action":{"type":"click","x":100,"y":200,"button":"left"},
			"pending_safety_checks":[]
		}`},
		{"type", `{
			"type":"computer_call","id":"item_2","call_id":"call_2","status":"completed",
			"action":{"type":"type","text":"hello world"},
			"pending_safety_checks":[]
		}`},
		{"scroll", `{
			"type":"computer_call","id":"item_3","call_id":"call_3","status":"completed",
			"action":{"type":"scroll","x":50,"y":50,"direction":"down","amount":3},
			"pending_safety_checks":[]
		}`},
		{"screenshot", `{
			"type":"computer_call","id":"item_4","call_id":"call_4","status":"completed",
			"action":{"type":"screenshot"},
			"pending_safety_checks":[]
		}`},
	}

	callIdx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &req))

		// Every request must contain the {type:computer} tool.
		var tools []map[string]any
		require.NoError(t, json.Unmarshal(req["tools"], &tools))
		hasComputer := false
		for _, tool := range tools {
			if tool["type"] == "computer" {
				hasComputer = true
			}
		}
		require.True(t, hasComputer, "request must contain {type:computer} tool, step %d", callIdx)

		// After the first call, input must contain computer_call_output.
		if callIdx > 0 {
			var input []map[string]any
			require.NoError(t, json.Unmarshal(req["input"], &input))
			hasOutput := false
			for _, item := range input {
				if item["type"] == "computer_call_output" {
					hasOutput = true
				}
			}
			require.True(t, hasOutput, "input must contain computer_call_output, step %d", callIdx)
		}

		w.Header().Set("Content-Type", "application/json")

		if callIdx < len(steps) {
			s := steps[callIdx]
			callIdx++
			writeResponse(w, "resp_"+s.actionType, []json.RawMessage{json.RawMessage(s.rawItem)})
			return
		}

		callIdx++
		writeResponse(w, "resp_final", []json.RawMessage{
			json.RawMessage(`{
				"type":"message","id":"msg_1","role":"assistant","status":"completed",
				"content":[{"type":"output_text","text":"Done!"}]
			}`),
		})
	}))
	defer srv.Close()

	client := openaisdk.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
	model := newResponsesLanguageModel("gpt-4.1", Name, client, fantasy.ObjectModeAuto)

	tools := []fantasy.Tool{NewComputerUseTool()}
	prompt := fantasy.Prompt{
		{Role: fantasy.MessageRoleSystem, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "You control a computer."},
		}},
		{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "Open the browser"},
		}},
	}

	var allToolCalls []fantasy.ToolCallContent

	for i := 0; i < len(steps)+2; i++ {
		resp, err := model.Generate(t.Context(), fantasy.Call{
			Prompt: prompt,
			Tools:  tools,
		})
		require.NoError(t, err, "step %d", i)

		var stepToolCalls []fantasy.ToolCallContent
		for _, c := range resp.Content {
			if tc, ok := fantasy.AsContentType[fantasy.ToolCallContent](c); ok {
				stepToolCalls = append(stepToolCalls, tc)
			}
		}

		if len(stepToolCalls) == 0 {
			require.Contains(t, resp.Content.Text(), "Done!")
			break
		}

		require.Len(t, stepToolCalls, 1, "step %d", i)
		tc := stepToolCalls[0]
		require.Equal(t, "computer", tc.ToolName)
		require.NotEmpty(t, tc.ToolCallID)

		// Verify metadata is attached.
		meta, ok := tc.ProviderMetadata[Name]
		require.True(t, ok, "step %d: missing provider metadata", i)
		cuMeta, ok := meta.(*ComputerUseMetadata)
		require.True(t, ok, "step %d: wrong metadata type", i)
		require.NotEmpty(t, cuMeta.RawJSON)

		// Verify input contains the action type.
		require.Contains(t, tc.Input, steps[i].actionType)

		// Verify typed parsing works on the input.
		actions, err := ParseComputerActions(tc.Input)
		require.NoError(t, err, "step %d: ParseComputerActions", i)
		require.Len(t, actions, 1)

		allToolCalls = append(allToolCalls, tc)

		// Rebuild prompt with tool call and screenshot result.
		prompt = append(prompt, fantasy.Message{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Input:      tc.Input,
					ProviderOptions: fantasy.ProviderOptions{
						Name: cuMeta,
					},
				},
			},
		})
		prompt = append(prompt, fantasy.Message{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: tc.ToolCallID,
					Output: fantasy.ToolResultOutputContentMedia{
						Data:      "iVBORw0KGgoAAAANSUhEUg==",
						MediaType: "image/png",
					},
				},
			},
		})
	}

	require.Len(t, allToolCalls, len(steps))
	require.Equal(t, len(steps)+1, callIdx)
}

// TestComputerUseToolsSkippedInToolArray verifies that
// toResponsesTools skips computer use tools without emitting an
// unsupported-tool warning.
func TestComputerUseToolsSkippedInToolArray(t *testing.T) {
	t.Parallel()

	tools := []fantasy.Tool{
		fantasy.FunctionTool{
			Name:        "search",
			Description: "Search the web",
			InputSchema: map[string]any{"type": "object"},
		},
		NewComputerUseTool(),
	}

	openaiTools, _, warnings := toResponsesTools(tools, nil, nil)

	require.Len(t, openaiTools, 1)
	require.Equal(t, "search", openaiTools[0].OfFunction.Name)

	for _, w := range warnings {
		require.NotEqual(t, fantasy.CallWarningTypeUnsupportedTool, w.Type,
			"computer use tool should not produce an unsupported warning")
	}
}

// writeResponse is a helper that writes a mock OpenAI Responses API
// response to the HTTP response writer.
func writeResponse(w http.ResponseWriter, id string, output []json.RawMessage) {
	resp := map[string]any{
		"id":     id,
		"object": "response",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 5,
			"input_tokens_details": map[string]any{
				"cached_tokens": 0,
			},
			"output_tokens_details": map[string]any{
				"reasoning_tokens": 0,
			},
		},
		"incomplete_details": nil,
		"status":             "completed",
	}
	json.NewEncoder(w).Encode(resp)
}
