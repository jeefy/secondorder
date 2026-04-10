package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/models"
)

type ollamaToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaChatRequest struct {
	Model    string                   `json:"model"`
	Messages []ollamaMessage          `json:"messages"`
	Stream   bool                     `json:"stream"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

type ollamaChatResponse struct {
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int64         `json:"prompt_eval_count"`
	EvalCount       int64         `json:"eval_count"`
}

type ollamaInstanceEntry struct {
	url string
	sem chan struct{}
}

var (
	ollamaRegistryMu sync.Mutex
	ollamaRegistry   = map[string]*ollamaInstanceEntry{}
)

func getOllamaInstance(url string, maxParallel int) *ollamaInstanceEntry {
	ollamaRegistryMu.Lock()
	defer ollamaRegistryMu.Unlock()

	if inst, ok := ollamaRegistry[url]; ok {
		return inst
	}
	if maxParallel <= 0 {
		maxParallel = 1
	}
	inst := &ollamaInstanceEntry{
		url: url,
		sem: make(chan struct{}, maxParallel),
	}
	ollamaRegistry[url] = inst
	return inst
}

// SetOllamaInstances initializes the registry from configured instances.
func SetOllamaInstances(instances []models.OllamaInstance) {
	for _, i := range instances {
		getOllamaInstance(i.URL, i.MaxParallel)
	}
}

func parseOllamaModel(modelStr string) (modelName, instanceURL string) {
	if idx := strings.LastIndex(modelStr, "@"); idx >= 0 {
		modelName = modelStr[:idx]
		host := modelStr[idx+1:]
		if !strings.HasPrefix(host, "http") {
			host = "http://" + host
		}
		instanceURL = host
		return
	}
	return modelStr, "http://localhost:11434"
}

func resolveOllamaInstance(modelStr string) (string, *ollamaInstanceEntry) {
	modelName, instanceURL := parseOllamaModel(modelStr)
	inst := getOllamaInstance(instanceURL, 1)
	return modelName, inst
}

func (s *Scheduler) execOllama(ctx context.Context, agent *models.Agent, soAPIKey, runID, issueKey, prompt string) (string, error) {
	modelName, inst := resolveOllamaInstance(agent.Model)

	inst.sem <- struct{}{}
	defer func() { <-inst.sem }()

	var systemParts []string

	if agent.ArchetypeSlug != "" {
		content, err := archetypes.Get(agent.ArchetypeSlug)
		if err == nil && content != "" {
			systemParts = append(systemParts, content)
		}
	}

	claudeMD := filepath.Join(agent.WorkingDir, "artifact-docs", "CLAUDE.md")
	if data, err := os.ReadFile(claudeMD); err == nil {
		systemParts = append(systemParts, "## Project Context\n\n"+string(data))
	}

	hotMem := filepath.Join(agent.WorkingDir, "artifact-docs", "hot-memory.md")
	if data, err := os.ReadFile(hotMem); err == nil {
		systemParts = append(systemParts, "## Working Memory\n\n"+string(data))
	}

	systemParts = append(systemParts, fmt.Sprintf(`## secondorder API

You have access to the secondorder task board via REST API at %s.
Your API key: %s
Your agent ID: %s
Current issue key: %s
Working directory: %s

Use the bash tool to call the API with curl when you need to update issue status, post comments, or create sub-issues.`, fmt.Sprintf("http://localhost:%d", s.port), soAPIKey, agent.ID, issueKey, agent.WorkingDir))

	systemPrompt := strings.Join(systemParts, "\n\n---\n\n")

	messages := []ollamaMessage{}
	if systemPrompt != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, ollamaMessage{Role: "user", Content: prompt})

	var outputBuf strings.Builder
	var totalInputTokens, totalOutputTokens int64

	maxTurns := agent.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	apiURL := strings.TrimRight(inst.url, "/") + "/api/chat"
	client := &http.Client{Timeout: 300 * time.Second}

	for turn := 0; turn < maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return outputBuf.String(), ctx.Err()
		default:
		}

		reqBody := ollamaChatRequest{
			Model:    modelName,
			Messages: messages,
			Stream:   false,
			Tools:    agentTools,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return outputBuf.String(), fmt.Errorf("marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return outputBuf.String(), fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return outputBuf.String(), fmt.Errorf("ollama API request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return outputBuf.String(), fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode != 200 {
			return outputBuf.String(), fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ollamaChatResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			return outputBuf.String(), fmt.Errorf("parse response: %w", err)
		}

		totalInputTokens += apiResp.PromptEvalCount
		totalOutputTokens += apiResp.EvalCount

		msg := apiResp.Message
		messages = append(messages, msg)

		if msg.Content != "" {
			outputBuf.WriteString(msg.Content)
			outputBuf.WriteString("\n")
		}

		if len(msg.ToolCalls) == 0 {
			slog.Debug("ollama: agent done (no tool calls)", "turn", turn, "run_id", runID)
			break
		}

		for _, tc := range msg.ToolCalls {
			slog.Debug("ollama: tool call", "tool", tc.Function.Name, "run_id", runID)
			argsJSON := string(tc.Function.Arguments)
			result := executeTool(tc.Function.Name, argsJSON, agent.WorkingDir, agent.ID, runID, s.db)
			outputBuf.WriteString(fmt.Sprintf("[tool:%s] %s\n", tc.Function.Name, result))
			messages = append(messages, ollamaMessage{
				Role:    "tool",
				Content: result,
			})
		}
	}

	outputBuf.WriteString(fmt.Sprintf("\n{\"type\":\"result\",\"result\":{\"input_tokens\":%d,\"output_tokens\":%d}}\n",
		totalInputTokens, totalOutputTokens))

	return outputBuf.String(), nil
}
