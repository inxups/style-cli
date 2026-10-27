package adapter

import "context"

// ManagementInputs exposes file paths only. Renderers retain configuration,
// owner and trust semantics; app owns capture and commit coordination.
type ManagementInputs interface{ ManagementPaths() ([]string, error) }

// UninstallStorage identifies adapter-owned trees removed with the last
// package. The application captures and stages them in its removal transaction.
type UninstallStorage interface {
	UninstallStorage(context.Context) ([]string, error)
}
