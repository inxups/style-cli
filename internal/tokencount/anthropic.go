package tokencount

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const Endpoint = "https://api.anthropic.com/v1/messages/count_tokens"
const APIVersion = "2023-06-01"
const maxResponseBytes = 1 << 20

// Counter dependencies are injected only by callers/tests, never from an
// ambient endpoint or proxy credential setting. Redirects are always disabled.
type Counter struct {
	Client *http.Client
	Key    func() string
}

func (c Counter) anthropic(ctx context.Context, text string, o Options, r Result) (Result, error) {
	if text == "" {
		return Result{}, fmt.Errorf("count: anthropic message text must not be empty")
	}
	key := ""
	if c.Key != nil {
		key = c.Key()
	} else {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if strings.TrimSpace(key) == "" || len(key) > 4096 {
		return Result{}, fmt.Errorf("count: ANTHROPIC_API_KEY is required")
	}
	for _, v := range key {
		if v < 33 || v > 126 {
			return Result{}, fmt.Errorf("count: invalid API key configuration")
		}
	}
	payload := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{Model: o.Model}
	payload.Messages = append(payload.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{"user", text})
	data, _ := json.Marshal(payload)
	r.RequestSHA256 = digest(data)
	limited, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(limited, http.MethodPost, Endpoint, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", APIVersion)
	req.Header.Set("x-api-key", key)
	client := http.Client{Timeout: 15 * time.Second}
	if c.Client != nil {
		client = *c.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		if limited.Err() != nil {
			return Result{}, fmt.Errorf("count: request cancelled or timed out")
		}
		return Result{}, fmt.Errorf("count: API transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, fmt.Errorf("count: API returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return Result{}, fmt.Errorf("count: invalid API response")
	}
	r.Tokens, err = decodeCount(body)
	if err != nil {
		return Result{}, fmt.Errorf("count: invalid API token count")
	}
	if limited.Err() != nil {
		return Result{}, fmt.Errorf("count: request cancelled or timed out")
	}
	return r, nil
}
func decodeCount(body []byte) (int, error) {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return 0, fmt.Errorf("object required")
	}
	seen := map[string]bool{}
	count := -1
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return 0, err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return 0, fmt.Errorf("duplicate field")
		}
		seen[name] = true
		if name == "input_tokens" {
			var value any
			if err := d.Decode(&value); err != nil {
				return 0, err
			}
			n, ok := value.(json.Number)
			if !ok {
				return 0, fmt.Errorf("integer required")
			}
			count, err = strconv.Atoi(n.String())
			if err != nil || count < 0 {
				return 0, fmt.Errorf("nonnegative integer required")
			}
		} else {
			var ignored json.RawMessage
			if err := d.Decode(&ignored); err != nil {
				return 0, err
			}
		}
	}
	if _, err := d.Token(); err != nil {
		return 0, err
	}
	if err := d.Decode(new(any)); err != io.EOF || !seen["input_tokens"] {
		return 0, fmt.Errorf("missing count or trailing data")
	}
	return count, nil
}
