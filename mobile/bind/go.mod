module github.com/hansshaw/OpenNovel/mobile/bind

go 1.26.0

require github.com/hansshaw/OpenNovel v0.0.0

require (
	github.com/gofrs/flock v0.13.2 // indirect
	github.com/voocel/agentcore v1.8.3 // indirect
	github.com/voocel/litellm v1.9.6 // indirect
	golang.org/x/image v0.48.0 // indirect
	golang.org/x/mobile v0.0.0-20261011034528-197770444867 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

replace github.com/hansshaw/OpenNovel => ../..

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
