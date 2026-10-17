package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/internal/adapter"
	"github.com/inxups/style-cli/internal/probe"
	"github.com/speakeasy-api/agenthooks"
)

// Runtime connects the provider codec, adapter enrichment, state, and core
// engine. The codec remains responsible for reading stdin and encoding output.
type Runtime struct {
	// InitializationError rejects decoded events before any state access.
	InitializationError error
	Observer            *probe.Observer
	Trace               *engine.Trace
	Engine              *engine.Engine
	State               *state.Store
	Adapter             adapter.RuntimeEnricher
	Provider            agenthooks.Provider
	FailClosed          bool
	Logger              *slog.Logger
	Now                 func() time.Time
	CodecStateDir       string
	loadErr             error
}

// New assembles an invocation-scoped hook runtime.
func New(engineCore *engine.Engine, stateStore *state.Store, enricher adapter.RuntimeEnricher, provider agenthooks.Provider, failClosed bool) (*Runtime, error) {
	if stateStore == nil || enricher == nil || provider == "" {
		return nil, fmt.Errorf("runtime: state, adapter, and provider are required")
	}
	if failClosed && provider != agenthooks.ProviderClaudeCode {
		return nil, fmt.Errorf("runtime: fail-closed is supported only for Claude Code")
	}
	return &Runtime{Engine: engineCore, State: stateStore, Adapter: enricher, Provider: provider, FailClosed: failClosed, Now: time.Now}, nil
}

// Load creates an engine from the installed index. A load error is retained
// for the event handler so agenthooks can emit its provider-correct fail-open
// response after it has decoded the event.
func Load(indexPath, stateRoot string, enricher adapter.RuntimeEnricher, provider agenthooks.Provider, failClosed bool) (*Runtime, error) {
	store, err := state.New(stateRoot)
	if err != nil {
		return nil, err
	}
	indexFile, err := os.Open(indexPath)
	if err != nil {
		runtime, initErr := New(nil, store, enricher, provider, failClosed)
		if initErr != nil {
			return nil, initErr
		}
		runtime.loadErr = err
		return runtime, nil
	}
	defer indexFile.Close()
	engineCore, loadErr := engine.Load(indexFile, store)
	runtime, err := New(engineCore, store, enricher, provider, failClosed)
	if err != nil {
		return nil, err
	}
	runtime.loadErr = loadErr
	return runtime, nil
}

type eventAdapter func(context.Context, *agenthooks.Event, any) (engine.Output, error)

// Register wires the STYLE handler into agenthooks' typed event pipeline.
func (r *Runtime) Register(runner *agenthooks.Runner) error {
	if r == nil || runner == nil || r.State == nil || r.Adapter == nil || r.Provider == "" {
		return fmt.Errorf("runtime: initialized runtime and agenthooks runner are required")
	}
	if r.Observer != nil {
		runner.OnAny(func(_ context.Context, event *agenthooks.Event) error {
			if event == nil || event.Backfilled {
				return nil
			}
			meta := probe.Metadata{Harness: string(event.Provider), Event: event.NativeName, Session: event.Session.ID, Turn: event.Session.TurnID}
			if event.Agent != nil {
				meta.Agent = event.Agent.ID
			}
			r.Observer.Submit(meta)
			return nil
		})
	}
	handle := eventAdapter(r.process)
	runner.OnSessionStart(func(ctx context.Context, event *agenthooks.SessionStartEvent) (agenthooks.SessionStartDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.SessionStartDecision{}, err
		}
		decision := agenthooks.SessionStartDecision{}
		if output.Block {
			return decision, fmt.Errorf("runtime: session.start cannot block")
		}
		if output.AdditionalContext != "" || output.SystemMessage != "" {
			decision = agenthooks.ContinueSession()
			if output.AdditionalContext != "" {
				decision = decision.WithContext(output.AdditionalContext)
			}
			if output.SystemMessage != "" {
				decision = decision.WithSystemMessage(output.SystemMessage)
			}
		}
		return decision, nil
	})
	runner.OnPromptSubmitted(func(ctx context.Context, event *agenthooks.PromptEvent) (agenthooks.PromptDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.PromptDecision{}, err
		}
		return promptDecision(output), nil
	})
	runner.OnPromptExpansion(func(ctx context.Context, event *agenthooks.PromptExpansionEvent) (agenthooks.PromptDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.PromptDecision{}, err
		}
		return promptDecision(output), nil
	})
	runner.OnToolPre(func(ctx context.Context, event *agenthooks.ToolPreEvent) (agenthooks.ToolPreDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.NoDecision(), err
		}
		return toolPreDecision(output), nil
	})
	runner.OnPermission(func(ctx context.Context, event *agenthooks.PermissionEvent) (agenthooks.ToolPreDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.NoDecision(), err
		}
		return toolPreDecision(output), nil
	})
	runner.OnToolPost(func(ctx context.Context, event *agenthooks.ToolPostEvent) (agenthooks.ToolPostDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.ToolPostDecision{}, err
		}
		return toolPostDecision(output), nil
	})
	runner.OnToolError(func(ctx context.Context, event *agenthooks.ToolPostEvent) (agenthooks.ToolPostDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.ToolPostDecision{}, err
		}
		return toolPostDecision(output), nil
	})
	runner.OnSubagentStartDecision(func(ctx context.Context, event *agenthooks.SubagentStartEvent) (agenthooks.SubagentStartDecision, error) {
		output, err := handle(ctx, &event.Event, event)
		if err != nil {
			return agenthooks.NoSubagentStartDecision(), err
		}
		decision := agenthooks.NoSubagentStartDecision()
		if output.AdditionalContext != "" {
			decision = decision.WithContext(output.AdditionalContext)
		}
		return decision, nil
	})
	runner.OnOther("PostModelSwitch", func(_ context.Context, event *agenthooks.Event) error {
		if r.InitializationError != nil {
			return r.InitializationError
		}
		if event == nil {
			return fmt.Errorf("runtime: missing model-switch event")
		}
		previous, err := r.State.ReadSession(event.Session.ID)
		if err != nil {
			r.logError(event, "state-error")
			return fmt.Errorf("runtime: model-switch state read failed")
		}
		update := r.Adapter.StateUpdate(event, event, previous)
		applySessionFacts(&update, event.Session)
		if _, _, err := r.State.Update(event.Session.ID, runtimeAgentID(event), update); err != nil {
			r.logError(event, "state-error")
			return fmt.Errorf("runtime: model-switch state update failed")
		}
		return nil
	})
	return nil
}

// Run delegates stdin decode and provider output encoding to agenthooks.
func (r *Runtime) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, extra ...agenthooks.Option) int {
	if r == nil {
		_, _ = fmt.Fprintln(stderr, "runtime: nil runtime")
		return 1
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	options := []agenthooks.Option{agenthooks.WithoutBackfill(), agenthooks.WithPolicyFunc(func(event *agenthooks.Event) agenthooks.Policy {
		policy := agenthooks.Policy{Fail: agenthooks.FailOpen}
		if r.FailClosed {
			policy.Fail = agenthooks.FailClosed
			if event != nil && event.Provider == agenthooks.ProviderClaudeCode && event.Kind == agenthooks.KindToolPre {
				policy.FailClosedExitCode = 1
			}
		}
		return policy
	}), agenthooks.WithLogger(logger)}
	if r.CodecStateDir != "" {
		options = append(options, agenthooks.WithDedupDir(r.CodecStateDir))
	}
	options = append(options, extra...)
	runner := agenthooks.New(options...)
	runtime := *r
	runtime.Logger = logger
	if err := runtime.Register(runner); err != nil {
		logger.Error("STYLE runtime registration failed", "code", "runtime-registration")
		return 1
	}
	return runner.Run(ctx, args, stdin, stdout, stderr)
}

func (r *Runtime) process(ctx context.Context, event *agenthooks.Event, typed any) (engine.Output, error) {
	if event == nil {
		return engine.Output{}, fmt.Errorf("runtime: missing decoded event")
	}
	if r.loadErr != nil || r.InitializationError != nil {
		r.logError(event, "index-invalid")
		return engine.Output{}, fmt.Errorf("runtime: index is unavailable")
	}
	if r.Engine == nil {
		r.logError(event, "index-invalid")
		return engine.Output{}, fmt.Errorf("runtime: index is unavailable")
	}
	previous, err := r.State.ReadSession(event.Session.ID)
	if err != nil {
		r.logError(event, "state-error")
		return engine.Output{}, fmt.Errorf("runtime: state read failed")
	}
	update := r.Adapter.StateUpdate(event, typed, previous)
	applySessionFacts(&update, event.Session)
	if event.Kind == agenthooks.KindPromptSubmitted {
		update.PromptSubmitted = true
	}
	if sessionStart, ok := typed.(*agenthooks.SessionStartEvent); ok {
		update.Source = sessionStart.Source
	}
	agentID := ""
	if event.Agent != nil {
		agentID = event.Agent.ID
	}
	session, scope, err := r.State.Update(event.Session.ID, agentID, update)
	if err != nil {
		r.logError(event, "state-error")
		return engine.Output{}, fmt.Errorf("runtime: state update failed")
	}
	input, limit, err := r.Adapter.Enrich(event, typed, session, scope)
	if err != nil {
		r.logError(event, "enrich-error")
		return engine.Output{}, fmt.Errorf("runtime: event enrichment failed")
	}
	input.Harness = string(r.Provider)
	input.Source = update.Source
	input.Scope = scope
	if input.Variables == nil {
		input.Variables = make(map[string]string)
	}
	if session.Model != "" {
		input.Variables["model"] = session.Model
	}
	if session.ThinkingEffort != "" {
		input.Variables["thinking_effort"] = session.ThinkingEffort
	}
	if session.EffortRaw != "" {
		input.Variables["thinking_effort_raw"] = session.EffortRaw
	}
	if session.EffortApprox != nil {
		input.Variables["thinking_effort_approx"] = strconv.FormatBool(*session.EffortApprox)
	}
	if session.HarnessVersion != "" {
		input.Variables["harness.version"] = session.HarnessVersion
	}
	if event.Session.TurnID != "" {
		input.TurnID = event.Session.TurnID
		input.TurnSource = "native"
	} else if input.TurnID == "" {
		input.TurnID = strconv.FormatUint(scope.PromptCount, 10)
		input.TurnSource = "prompt-count"
	}
	if input.TurnSource == "" {
		input.TurnSource = "prompt-count"
	}
	var output engine.Output
	if r.Trace != nil {
		var trace engine.Trace
		output, trace, err = r.Engine.ProcessWithTrace(input, limit)
		*r.Trace = trace
	} else {
		output, err = r.Engine.Process(input, limit)
	}
	if err != nil {
		r.logError(event, "engine-error")
		return engine.Output{}, fmt.Errorf("runtime: event evaluation failed")
	}
	if err := output.ValidateFor(string(r.Provider)); err != nil {
		r.logError(event, "output-invalid")
		return engine.Output{}, fmt.Errorf("runtime: output validation failed")
	}
	r.record(event, output)
	return output, nil
}

func applySessionFacts(update *state.EventUpdate, session agenthooks.SessionInfo) {
	if session.Model != "" {
		update.Model = session.Model
	}
	if session.Effort != "" {
		update.ThinkingEffort = session.Effort
	}
	if session.EffortRaw != "" {
		update.EffortRaw = session.EffortRaw
	}
	if session.EffortApprox != nil {
		value := *session.EffortApprox
		update.EffortApprox = &value
	}
	if session.HarnessVersion != "" {
		update.HarnessVersion = session.HarnessVersion
	}
}

func promptDecision(output engine.Output) agenthooks.PromptDecision {
	if output.Block {
		decision := agenthooks.BlockPrompt(output.Reason)
		if output.SuppressOriginalPrompt {
			decision = decision.WithSuppressOriginalPrompt()
		}
		return decision
	}
	if output.AdditionalContext == "" && output.SystemMessage == "" {
		return agenthooks.PromptDecision{}
	}
	decision := agenthooks.AcceptPrompt()
	if output.AdditionalContext != "" {
		decision = decision.WithContext(output.AdditionalContext)
	}
	if output.SystemMessage != "" {
		decision = decision.WithSystemMessage(output.SystemMessage)
	}
	return decision
}

func toolPreDecision(output engine.Output) agenthooks.ToolPreDecision {
	if output.Block {
		return agenthooks.Deny(output.Reason)
	}
	if output.AdditionalContext == "" && output.SystemMessage == "" {
		return agenthooks.NoDecision()
	}
	decision := agenthooks.NoDecision()
	if output.AdditionalContext != "" {
		decision = decision.WithContext(output.AdditionalContext)
	}
	if output.SystemMessage != "" {
		decision = decision.WithSystemMessage(output.SystemMessage)
	}
	return decision
}

func toolPostDecision(output engine.Output) agenthooks.ToolPostDecision {
	if output.AdditionalContext == "" && output.SystemMessage == "" {
		return agenthooks.ToolPostDecision{}
	}
	decision := agenthooks.Observed()
	if output.AdditionalContext != "" {
		decision = decision.WithContext(output.AdditionalContext)
	}
	if output.SystemMessage != "" {
		decision = decision.WithSystemMessage(output.SystemMessage)
	}
	return decision
}

func (r *Runtime) record(event *agenthooks.Event, output engine.Output) {
	for _, item := range output.Records {
		if r.Logger != nil {
			r.Logger.Info("STYLE rule evaluated", "provider", r.Provider, "event", event.NativeName,
				"point", item.Point, "entry", item.Entry, "action", item.Action, "result", item.Result)
		}
		payload, err := json.Marshal(state.StatsRecord{
			At: r.now().UTC().Format(time.RFC3339Nano), Provider: string(r.Provider),
			Event: event.NativeName, Point: item.Point, Entry: item.Entry,
			Action: item.Action, Result: item.Result, Tokens: &item.EstimatedTokens, BodySHA256: item.BodySHA256,
		})
		if err != nil || r.State.AppendStats(event.Session.ID, runtimeAgentID(event), payload) != nil {
			if r.Logger != nil {
				r.Logger.Warn("STYLE stats append failed", "code", "stats-write")
			}
		}
	}
}

func (r *Runtime) logError(event *agenthooks.Event, code string) {
	if r.Logger == nil {
		return
	}
	r.Logger.Error("STYLE hook evaluation failed", "code", code, "provider", r.Provider,
		"event", event.NativeName, "point", string(event.Kind))
}

func (r *Runtime) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func runtimeAgentID(event *agenthooks.Event) string {
	if event.Agent == nil || event.Agent.ID == "" {
		return "main"
	}
	return event.Agent.ID
}
