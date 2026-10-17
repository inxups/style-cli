package adapter

// ManagementInputs exposes file paths only. Renderers retain configuration,
// owner and trust semantics; app owns capture and commit coordination.
type ManagementInputs interface{ ManagementPaths() ([]string, error) }
