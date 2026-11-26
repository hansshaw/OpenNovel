module github.com/hansshaw/OpenNovel

go 1.26.0

require (
	github.com/gofrs/flock v0.13.2
	// Keep this pair for model-specific strict schema and thinking capabilities.
	// LiteLLM 1.9 removes those facts; AgentCore 1.8.5+ replaces llm and context.
	github.com/voocel/agentcore v1.8.3
	github.com/voocel/litellm v1.8.11
	golang.org/x/image v0.48.0
	golang.org/x/mod v0.42.0
	golang.org/x/net v0.63.0
	golang.org/x/text v0.44.0
)

require golang.org/x/sys v0.50.0 // indirect
