package scheduler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/msoedov/secondorder/internal/db"
)

// agentTools defines the tools available to API-based agents (Copilot, Ollama, etc.).
var agentTools = []map[string]interface{}{
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "read_file",
			"description": "Read the contents of a file",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "File path to read"},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "write_file",
			"description": "Write content to a file",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":    map[string]interface{}{"type": "string", "description": "File path to write"},
					"content": map[string]interface{}{"type": "string", "description": "Content to write"},
				},
				"required": []string{"path", "content"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "bash",
			"description": "Run a bash command and return its output",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string", "description": "Bash command to execute"},
				},
				"required": []string{"command"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "list_dir",
			"description": "List files in a directory",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Directory path to list"},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "supermemory_store",
			"description": "Store a learning, finding, or fact in Supermemory for recall in future sessions. Call this at the end of a run to persist key insights.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{"type": "string", "description": "The content to store (finding, learning, decision, technical fact)"},
					"tags":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Tags to categorize this memory (e.g. repo name, domain, agent slug)"},
				},
				"required": []string{"content"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "supermemory_recall",
			"description": "Search Supermemory for relevant past learnings and context. Call this at the start of a run to recall domain knowledge from prior sessions.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "Search query to find relevant memories"},
					"limit": map[string]interface{}{"type": "integer", "description": "Maximum results to return (default: 5)"},
				},
				"required": []string{"query"},
			},
		},
	},
}

// executeTool runs a tool call and returns the result string.
// agentID and runID are used to log supermemory events; database may be nil (e.g. in tests).
func executeTool(name, argsJSON, workingDir, agentID, runID string, database *db.DB) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing args: %v", err)
	}

	strArg := func(key string) string {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}

	switch name {
	case "read_file":
		path := strArg("path")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("error reading file: %v", err)
		}
		return string(data)

	case "write_file":
		path := strArg("path")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Sprintf("error creating dirs: %v", err)
		}
		if err := os.WriteFile(path, []byte(strArg("content")), 0644); err != nil {
			return fmt.Sprintf("error writing file: %v", err)
		}
		return "file written successfully"

	case "bash":
		cmd := exec.Command("bash", "-c", strArg("command"))
		cmd.Dir = workingDir
		out, err := cmd.CombinedOutput()
		result := string(out)
		if err != nil {
			result += fmt.Sprintf("\n[exit error: %v]", err)
		}
		if len(result) > 8000 {
			result = result[:8000] + "\n... (truncated)"
		}
		return result

	case "list_dir":
		path := strArg("path")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Sprintf("error listing dir: %v", err)
		}
		var lines []string
		for _, e := range entries {
			prefix := "  "
			if e.IsDir() {
				prefix = "d "
			}
			lines = append(lines, prefix+e.Name())
		}
		return strings.Join(lines, "\n")

	case "supermemory_store":
		apiKey := os.Getenv("SUPERMEMORY_API_KEY")
		if apiKey == "" {
			return "error: SUPERMEMORY_API_KEY not set"
		}
		content := strArg("content")
		if content == "" {
			return "error: content is required"
		}
		var tags []string
		if tagsRaw, ok := args["tags"]; ok {
			if tagsSlice, ok := tagsRaw.([]interface{}); ok {
				for _, t := range tagsSlice {
					tags = append(tags, fmt.Sprintf("%v", t))
				}
			}
		}
		tags = append(tags, "secondorder")
		payload := map[string]interface{}{"content": content, "tags": tags}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "https://api.supermemory.ai/v3/documents", bytes.NewReader(body))
		if err != nil {
			return fmt.Sprintf("error creating request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("error calling supermemory: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			result := fmt.Sprintf("supermemory error %d: %s", resp.StatusCode, string(respBody))
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "store", "", 1, false)
			}
			return result
		}
		var resultMap map[string]interface{}
		var result string
		if err := json.Unmarshal(respBody, &resultMap); err == nil {
			if id, ok := resultMap["id"].(string); ok {
				result = fmt.Sprintf("stored successfully (id: %s)", id)
			}
		}
		if result == "" {
			result = "stored successfully"
		}
		if database != nil {
			database.LogSupermemoryEvent(agentID, runID, "store", "", 1, true)
		}
		return result

	case "supermemory_recall":
		apiKey := os.Getenv("SUPERMEMORY_API_KEY")
		if apiKey == "" {
			return "error: SUPERMEMORY_API_KEY not set"
		}
		query := strArg("query")
		if query == "" {
			return "error: query is required"
		}
		limit := 5
		if lv, ok := args["limit"]; ok {
			if lf, ok := lv.(float64); ok && lf > 0 {
				limit = int(lf)
			}
		}
		payload := map[string]interface{}{"q": query, "limit": limit}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "https://api.supermemory.ai/v3/search", bytes.NewReader(body))
		if err != nil {
			return fmt.Sprintf("error creating request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("error calling supermemory: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			result := fmt.Sprintf("supermemory error %d: %s", resp.StatusCode, string(respBody))
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "recall", query, 0, false)
			}
			return result
		}
		var searchResp struct {
			Results []struct {
				Title  string  `json:"title"`
				Score  float64 `json:"score"`
				Chunks []struct {
					Content string `json:"content"`
				} `json:"chunks"`
			} `json:"results"`
		}
		if err := json.Unmarshal(respBody, &searchResp); err != nil {
			result := fmt.Sprintf("error parsing supermemory response: %v", err)
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "recall", query, 0, false)
			}
			return result
		}
		if len(searchResp.Results) == 0 {
			result := "no memories found for query: " + query
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "recall", query, 0, true)
			}
			return result
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(searchResp.Results)))
		for i, r := range searchResp.Results {
			sb.WriteString(fmt.Sprintf("### %d. %s (score: %.2f)\n", i+1, r.Title, r.Score))
			for _, chunk := range r.Chunks {
				sb.WriteString(chunk.Content)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
		result := sb.String()
		resultCount := parseRecallCount(result)
		success := !strings.HasPrefix(result, "error") && !strings.HasPrefix(result, "supermemory error")
		if database != nil {
			database.LogSupermemoryEvent(agentID, runID, "recall", query, resultCount, success)
		}
		return result

	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
}

// parseRecallCount extracts N from "Found N memories:\n\n..."
// Returns 0 if result starts with "no memories found" or on parse failure.
func parseRecallCount(result string) int {
	if strings.HasPrefix(result, "no memories found") {
		return 0
	}
	var n int
	fmt.Sscanf(result, "Found %d memories", &n)
	return n
}
