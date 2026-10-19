// Package tokencount provides explicit cold-path text counters. It never
// changes STYLE's budget estimator and never infers a model's encoding.
package tokencount

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/inxups/style-cli/core/tokens"
	"github.com/tiktoken-go/tokenizer"
)

const MaxTextBytes = 256 << 10
const MaxRunBytes = 4096
const Implementation = "github.com/tiktoken-go/tokenizer@v0.7.0"

type Options struct{ Mode, Provider, Encoding, Model string }

func (o Options) Exact() bool { return o.Mode == "exact" }
func (o Options) Validate() error {
	if o.Mode != "" && o.Mode != "estimate" && o.Mode != "exact" {
		return fmt.Errorf("count: mode must be estimate or exact")
	}
	if !o.Exact() {
		if o.Provider != "" || o.Encoding != "" || o.Model != "" {
			return fmt.Errorf("count: provider, encoding and model require --exact")
		}
		return nil
	}
	switch o.Provider {
	case "openai":
		if (o.Encoding != "o200k_base" && o.Encoding != "cl100k_base") || o.Model != "" {
			return fmt.Errorf("count: local openai requires explicit o200k_base or cl100k_base and no model inference")
		}
	case "anthropic":
		if !modelID.MatchString(o.Model) || o.Encoding != "" {
			return fmt.Errorf("count: --exact anthropic:<model> requires a valid model id and no encoding")
		}
	default:
		return fmt.Errorf("count: --exact requires explicit provider openai or anthropic")
	}
	return nil
}

var modelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

type Result struct {
	Mode           string `json:"mode"`
	Provider       string `json:"provider"`
	Scope          string `json:"scope"`
	Encoding       string `json:"encoding,omitempty"`
	Model          string `json:"model,omitempty"`
	Implementation string `json:"implementation,omitempty"`
	APIVersion     string `json:"api_version,omitempty"`
	Tokens         int    `json:"tokens"`
	Chars          int    `json:"chars"`
	Bytes          int    `json:"bytes"`
	TextSHA256     string `json:"text_sha256"`
	RequestSHA256  string `json:"request_sha256,omitempty"`
}

func digest(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
func base(text string, o Options) Result {
	r := Result{Mode: o.Mode, Provider: o.Provider, Encoding: o.Encoding, Model: o.Model, Chars: utf8.RuneCountInString(text), Bytes: len(text), TextSHA256: digest([]byte(text))}
	if !o.Exact() {
		r.Mode = "estimate"
		r.Provider = "style-estimator"
		r.Scope = "utf8-text-estimate"
	} else if o.Provider == "openai" {
		r.Scope = "utf8-text-encoding"
		r.Implementation = Implementation
	} else {
		r.Scope = "anthropic-messages-request"
		r.APIVersion = APIVersion
	}
	return r
}
func None(o Options) (Result, error) {
	if err := o.Validate(); err != nil {
		return Result{}, err
	}
	r := base("", o)
	r.Scope = "not-model-input"
	return r, nil
}
func (c Counter) Count(ctx context.Context, text string, o Options) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("count: context is required")
	}
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("count: cancelled")
	}
	if err := o.Validate(); err != nil {
		return Result{}, err
	}
	if len(text) > MaxTextBytes || !utf8.ValidString(text) {
		return Result{}, fmt.Errorf("count: text must be valid UTF-8 within 256 KiB")
	}
	r := base(text, o)
	if !o.Exact() {
		r.Tokens = tokens.EstimateText(text).Tokens
		return r, nil
	}
	if o.Provider == "anthropic" {
		return c.anthropic(ctx, text, o, r)
	}
	// The pinned BPE merges quadratically within a pre-token. Reject pathological
	// runs instead of chunking them (which would change exact token boundaries).
	run, last, first := 0, false, true
	for _, v := range text {
		space := unicode.IsSpace(v)
		if first || space != last {
			run = 0
			first = false
		}
		run += utf8.RuneLen(v)
		last = space
		if run > MaxRunBytes {
			return Result{}, fmt.Errorf("count: local exact rejects whitespace or non-whitespace runs over 4096 bytes")
		}
	}
	enc, err := tokenizer.Get(tokenizer.Encoding(o.Encoding))
	if err != nil {
		return Result{}, fmt.Errorf("count: encoding unavailable")
	}
	r.Tokens, err = enc.Count(text)
	if err != nil {
		return Result{}, fmt.Errorf("count: local encoding failed")
	}
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("count: cancelled")
	}
	return r, nil
}
