module github.com/inxups/style-cli

go 1.25.0

require (
	github.com/google/jsonschema-go v0.4.3
	github.com/pelletier/go-toml/v2 v2.4.2
	github.com/speakeasy-api/agenthooks v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require golang.org/x/sys v0.46.0 // indirect

replace github.com/speakeasy-api/agenthooks => ./agenthooks
