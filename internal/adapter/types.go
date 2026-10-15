package adapter

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/inxups/style-cli/core/engine"
	"github.com/inxups/style-cli/core/plan"
	"github.com/inxups/style-cli/core/spec"
	"github.com/inxups/style-cli/core/state"
	"github.com/inxups/style-cli/core/tokens"
	"github.com/speakeasy-api/agenthooks"
)

// ProviderID is agenthooks' canonical provider identifier.
type ProviderID = agenthooks.Provider

// Detector reports whether a provider is installed and where its configuration
// lives. Discovery performs no writes.
type Detector interface {
	Detect(context.Context) (Installation, bool, error)
}

// CapabilityProvider describes support for one STYLE output capability at an
// evaluation point.
type CapabilityProvider interface {
	Capability(point string, capability plan.Capability) CapabilitySupport
}

// DataSourceProvider describes provider fields available at an evaluation
// point, including their provenance and approximation status.
type DataSourceProvider interface {
	DataSource(point, field string) DataAvailability
}

// BlindSpotProvider identifies targets the provider can never observe.
type BlindSpotProvider interface {
	BlindSpot(rule spec.Rule) (reason string, unobservable bool)
}

// StaticLocationProvider reports whether a mode has a provider-owned slot.
type StaticLocationProvider interface {
	StaticLocation(mode string) (StaticLocation, bool)
}

// PlanningFacts composes only the provider facts needed by core/plan.
// BuildPlanAdapter projects this richer contract onto core/plan's small port.
type PlanningFacts interface {
	CapabilityProvider
	DataSourceProvider
	BlindSpotProvider
	StaticLocationProvider
}

// RuntimeEnricher turns decoded agenthooks events into core runtime facts.
// Implementations own provider-specific payload interpretation; the engine
// only receives normalized points, fields, values, and output limits.
type RuntimeEnricher interface {
	StateUpdate(event *agenthooks.Event, typed any, previous state.Session) state.EventUpdate
	Enrich(event *agenthooks.Event, typed any, session state.Session, scope state.Scope) (engine.Input, tokens.InputLimit, error)
}

// CapabilitySupport records why one output capability is or is not available
// at a STYLE evaluation point.
type CapabilitySupport struct {
	Available bool
	Reason    string
}

// DataAvailability identifies the source and certainty of runtime data.
type DataAvailability struct {
	Available   bool
	Source      string
	Approximate bool
	Reason      string
}

// StaticLocation is an adapter-owned static instruction slot.
type StaticLocation struct {
	Name      string
	Mode      string
	Supported bool
	Reason    string
}

// StaticPositionProvider lists static slots available for one detected
// installation. PlanningFacts.StaticLocation is the mode-specific lookup used
// by core/plan; this inventory is for capability reports and route planning.
type StaticPositionProvider interface {
	StaticPositions(Installation) []StaticLocation
}

// Installation contains detected, non-secret provider state needed by routes.
type Installation struct {
	Provider  ProviderID
	Version   string
	ConfigDir string
}

// InputLimit is the hard model-input limit for one native event/tool bucket.
// Package budgets remain in core/tokens and must not be confused with it.
type InputLimit = tokens.InputLimit

// OverflowBehavior describes the provider's externally observable behavior
// when a hook payload exceeds its hard input limit.
type OverflowBehavior string

const (
	OverflowFilePreview OverflowBehavior = "file-preview"
	OverflowSilentDrop  OverflowBehavior = "silent-drop"
	OverflowReject      OverflowBehavior = "reject"
	OverflowUnknown     OverflowBehavior = "unknown"
)

// Limits combines the adapter's native input limit and overflow behavior.
type Limits struct {
	Input       InputLimit
	Approximate bool
	Source      string
	Overflow    OverflowBehavior
}

// CompactSignal describes how an adapter reports context compaction. An empty
// Event means the provider has no observable compact signal.
type CompactSignal struct {
	Event         agenthooks.EventKind
	SessionSource string
	Deferred      bool
}

// Route is one provider installation location. Plan must be side-effect free;
// Apply and Remove receive the resulting changeset explicitly so the caller can
// display or persist it before mutation.
type Route interface {
	Name() string
	Available(context.Context, Installation) (bool, string, error)
	Trust() TrustInfo
	Plan(context.Context, []HookSpec, []StaticBlock) (Changeset, error)
	Apply(context.Context, Changeset) error
	Remove(context.Context, Owner) (Changeset, error)
	Status(context.Context, Installation) (RouteStatus, error)
}

// TrustKind describes the confirmation mechanism a route requires.
type TrustKind string

const (
	TrustNone             TrustKind = "none"
	TrustUserConfirmation TrustKind = "user-confirmation"
	TrustPreseed          TrustKind = "preseed"
	TrustWorkspace        TrustKind = "workspace"
)

// TrustInfo is safe to show in a plan or doctor report.
type TrustInfo struct {
	Kind      TrustKind
	StableKey bool
	Summary   string
}

// HookSpec is the provider-neutral subscription request consumed by a route.
type HookSpec struct {
	NativeEvent string
	Matcher     string
	Points      []string
	Extra       map[string]any
}

// StaticBlock is a STYLE-owned static instruction to place at a provider slot.
type StaticBlock struct {
	Owner Owner
	Slot  string
	Body  string
}

// Owner identifies only the STYLE-owned configuration to remove.
type Owner string

// FileChange is one planned file replacement. Contents may include user
// configuration; callers must not log them or render them as a diff without
// explicit redaction policy.
type FileChange struct {
	Path       string
	Root       string
	Mode       fs.FileMode
	Before     []byte
	After      []byte
	CreateOnly bool
	Remove     bool
}

// Changeset is an ordered, reviewable set of file updates.
type Changeset struct {
	Files []FileChange
}

// RouteStatus is the public installation/trust status of one route.
type RouteStatus struct {
	Available bool
	Installed bool
	Trusted   bool
	Detail    string
	LastEvent time.Time
}

// Definition assembles the small ports implemented by one provider adapter.
// Routes retain their declared order because it determines selection priority.
type Definition struct {
	ID              ProviderID
	Detector        Detector
	Facts           PlanningFacts
	StaticPositions StaticPositionProvider
	Routes          []Route
	Limits          Limits
	Compact         CompactSignal
}

// Detect applies this provider's detector and normalizes its result. A
// detector cannot accidentally report an installation for another provider.
func (definition Definition) Detect(ctx context.Context) (Installation, bool, error) {
	if definition.Detector == nil {
		return Installation{}, false, fmt.Errorf("adapter: detector is required for %q", definition.ID)
	}
	installation, installed, err := definition.Detector.Detect(ctx)
	if err != nil {
		return Installation{}, false, fmt.Errorf("adapter: detect provider %q: %w", definition.ID, err)
	}
	if !installed {
		return Installation{}, false, nil
	}
	if installation.Provider != "" && installation.Provider != definition.ID {
		return Installation{}, false, fmt.Errorf("adapter: detector for %q returned provider %q", definition.ID, installation.Provider)
	}
	installation.Provider = definition.ID
	return installation, true, nil
}
