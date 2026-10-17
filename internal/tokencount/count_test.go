package tokencount

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inxups/style-cli/core/tokens"
	"github.com/tiktoken-go/tokenizer"
)

type a15RoundTrip func(*http.Request) (*http.Response, error)

func (f a15RoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func local(encoding string) Options {
	return Options{Mode: "exact", Provider: "openai", Encoding: encoding}
}
func anthropic() Options {
	return Options{Mode: "exact", Provider: "anthropic", Model: "claude-sonnet-fixture"}
}

func TestM3A15IndependentTiktokenGoldenAndVocabularies(t *testing.T) {
	data, err := os.ReadFile("../../testdata/count/exact-text-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		TiktokenVersion string `json:"tiktoken_version"`
		Vocabularies    map[string]struct {
			Tokens int
			SHA256 string
		}
		Cases []struct {
			ID, Text, SHA256 string
			Tokens           map[string]int
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil || fixture.TiktokenVersion != "0.12.0" || len(fixture.Cases) != 137 {
		t.Fatal(err)
	}
	for name, vocab := range fixture.Vocabularies {
		t.Run(name+"/vocabulary", func(t *testing.T) {
			enc, err := tokenizer.Get(tokenizer.Encoding(name))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.New()
			header := make([]byte, 16)
			for id := 0; id < vocab.Tokens; id++ {
				token, err := enc.Decode([]uint{uint(id)})
				if err != nil {
					t.Fatal(id, err)
				}
				binary.BigEndian.PutUint64(header, uint64(id))
				binary.BigEndian.PutUint64(header[8:], uint64(len(token)))
				hash.Write(header)
				hash.Write([]byte(token))
			}
			if hex.EncodeToString(hash.Sum(nil)) != vocab.SHA256 {
				t.Fatal("embedded vocabulary differs from independent official tiktoken reference", name)
			}
		})
	}
	for _, row := range fixture.Cases {
		for name, want := range row.Tokens {
			t.Run(name+"/"+row.ID, func(t *testing.T) {
				r, err := (Counter{}).Count(context.Background(), row.Text, local(name))
				if err != nil || r.Tokens != want || r.TextSHA256 != "sha256:"+row.SHA256 || r.Implementation != Implementation || r.Scope != "utf8-text-encoding" {
					t.Fatal(row.ID, r, want, err)
				}
			})
		}
	}
}

func TestM3A15OptionsLimitsEstimatesAndCancellation(t *testing.T) {
	for _, o := range []Options{{Mode: "wrong"}, {Provider: "openai"}, {Mode: "exact"}, {Mode: "exact", Provider: "codex"}, {Mode: "exact", Provider: "openai", Encoding: "auto"}, {Mode: "exact", Provider: "openai", Encoding: "o200k_base", Model: "gpt-5"}, {Mode: "exact", Provider: "anthropic", Encoding: "cl100k_base", Model: "x"}, {Mode: "exact", Provider: "anthropic", Model: "../sensitive"}} {
		if o.Validate() == nil {
			t.Fatal(o)
		}
	}
	c := Counter{Key: func() string { t.Fatal("local read credentials"); return "" }, Client: &http.Client{Transport: a15RoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("local used network"); return nil, nil })}}
	for _, text := range []string{"", "normal 中文\n", strings.Repeat("ab ", 30000)} {
		r, err := c.Count(context.Background(), text, Options{})
		if err != nil || r.Tokens != tokens.EstimateText(text).Tokens || r.Mode != "estimate" {
			t.Fatal(r, err)
		}
	}
	for _, tc := range []struct{ name, text string }{{"oversize", strings.Repeat("a", MaxTextBytes+1)}, {"invalid-utf8", string([]byte{255})}, {"word-run", strings.Repeat("a", 4097)}, {"space-run", strings.Repeat(" ", 4097)}, {"unicode-run", strings.Repeat("中", 1366)}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.Count(context.Background(), tc.text, local("o200k_base")); err == nil {
				t.Fatal("accepted unsupported input")
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Count(cancelled, "text", local("o200k_base")); err == nil {
		t.Fatal("cancel accepted")
	}
	if _, err := c.Count(nil, "text", local("o200k_base")); err == nil {
		t.Fatal("nil context accepted")
	}
	if r, err := None(anthropic()); err != nil || r.Tokens != 0 || r.Scope != "not-model-input" || r.RequestSHA256 != "" {
		t.Fatal(r, err)
	}
}

func TestM3A15AnthropicRequestAndSafeFailures(t *testing.T) {
	const key = "fixture-api-key-canary"
	const text = "PRIVATE-count-text-中文\n<>&"
	cases := []struct {
		name, body                        string
		status                            int
		transportError, readError, cancel bool
		want                              int
	}{
		{name: "ok", body: `{"input_tokens":17}`, status: 200, want: 17},
		{name: "zero", body: `{"input_tokens":0,"metadata":{}}`, status: 200, want: 0},
		{name: "http401", status: 401}, {name: "http429", status: 429}, {name: "http500", status: 500},
		{name: "redirect", status: 302}, {name: "transport", transportError: true}, {name: "cancel", cancel: true},
		{name: "bad", body: `{`, status: 200}, {name: "missing", body: `{}`, status: 200}, {name: "negative", body: `{"input_tokens":-1}`, status: 200},
		{name: "float", body: `{"input_tokens":1.5}`, status: 200}, {name: "quoted", body: `{"input_tokens":"17"}`, status: 200}, {name: "null", body: `{"input_tokens":null}`, status: 200},
		{name: "overflow", body: `{"input_tokens":9999999999999999999999999999999}`, status: 200}, {name: "duplicate", body: `{"input_tokens":1,"input_tokens":2}`, status: 200},
		{name: "trailing", body: `{"input_tokens":2}{}`, status: 200}, {name: "huge", body: strings.Repeat(" ", maxResponseBytes+1), status: 200},
		{name: "body-read", readError: true, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := Counter{Key: func() string { return key }, Client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { t.Fatal("caller redirect policy used"); return nil }, Transport: a15RoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.String() != Endpoint || req.Method != "POST" || req.Header.Get("x-api-key") != key || req.Header.Get("anthropic-version") != APIVersion || req.Header.Get("Content-Type") != "application/json" {
					t.Fatal("request contract")
				}
				deadline, ok := req.Context().Deadline()
				if !ok || time.Until(deadline) > 15*time.Second {
					t.Fatal("unbounded request")
				}
				body, _ := io.ReadAll(req.Body)
				var payload struct {
					Model    string
					Messages []struct{ Role, Content string }
				}
				if json.Unmarshal(body, &payload) != nil || payload.Model != anthropic().Model || len(payload.Messages) != 1 || payload.Messages[0].Role != "user" || payload.Messages[0].Content != text {
					t.Fatal("wrong counting text/shape")
				}
				if tc.transportError {
					return nil, fmt.Errorf("%s %s", key, text)
				}
				if tc.cancel {
					cancel()
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				response := &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}
				if tc.status == 302 {
					response.Header.Set("Location", "https://forbidden.invalid/"+key)
				}
				if tc.readError {
					response.Body = io.NopCloser(a15ReadError{})
				}
				return response, nil
			})}}
			r, err := c.Count(ctx, text, anthropic())
			if calls != 1 {
				t.Fatal(calls)
			}
			success := tc.name == "ok" || tc.name == "zero"
			if success {
				if err != nil || r.Tokens != tc.want || r.Scope != "anthropic-messages-request" || r.RequestSHA256 == "" || r.TextSHA256 != digest([]byte(text)) {
					t.Fatal(r, err)
				}
			} else if err == nil {
				t.Fatal("failure silently succeeded", r)
			}
			if err != nil && (strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "PRIVATE")) {
				t.Fatal("unsafe error", err)
			}
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), key) || strings.Contains(string(data), "PRIVATE") {
				t.Fatal("unsafe result")
			}
		})
	}
	for _, key := range []string{"", " ", "bad\nkey", strings.Repeat("x", 4097)} {
		c := Counter{Key: func() string { return key }, Client: &http.Client{Transport: a15RoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("invalid key used network"); return nil, nil })}}
		if _, err := c.Count(context.Background(), text, anthropic()); err == nil {
			t.Fatal("missing/invalid key accepted")
		}
	}
	if _, err := (Counter{}).Count(context.Background(), "", anthropic()); err == nil {
		t.Fatal("empty message accepted")
	}
}

type a15ReadError struct{}

func (a15ReadError) Read([]byte) (int, error) { return 0, fmt.Errorf("sensitive response body error") }

func TestM3A15ConcurrentLocalCounters(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := (Counter{}).Count(context.Background(), "hello world", local("o200k_base"))
			if err != nil || r.Tokens != 2 {
				t.Error(r, err)
			}
		}()
	}
	wg.Wait()
}
