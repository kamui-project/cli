package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// mcpToolsList sends a JSON-RPC `tools/list` call to a Streamable HTTP MCP
// endpoint and returns the number of tools the server advertises.
//
// The MCP spec uses JSON-RPC 2.0 over HTTP. The server may respond with
// either application/json or a text/event-stream containing one SSE
// `message` event whose data field is the JSON-RPC response — both are
// handled here.
func mcpToolsList(ctx context.Context, mcpURL, token string) (int, error) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	hc := &http.Client{Timeout: 15 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	jsonPayload := raw
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		jsonPayload = extractSSEData(raw)
		if len(jsonPayload) == 0 {
			return 0, fmt.Errorf("empty SSE response from MCP server")
		}
	}

	var rpc struct {
		Result *struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(jsonPayload, &rpc); err != nil {
		return 0, fmt.Errorf("invalid JSON-RPC response: %w (body: %s)", err, truncate(string(jsonPayload), 200))
	}
	if rpc.Error != nil {
		return 0, fmt.Errorf("MCP server returned error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if rpc.Result == nil {
		return 0, fmt.Errorf("MCP server returned neither result nor error")
	}
	return len(rpc.Result.Tools), nil
}

// extractSSEData scans an SSE stream and concatenates the `data:` payloads
// of the first `message` event (or first event when no event type is given).
func extractSSEData(b []byte) []byte {
	var data []byte
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			if len(data) > 0 {
				return data
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimPrefix(line, "data:")
			payload = strings.TrimPrefix(payload, " ")
			data = append(data, []byte(payload)...)
		}
	}
	return data
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
