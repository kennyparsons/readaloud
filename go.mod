module github.com/kennyparsons/readaloud

go 1.24.5

require (
	github.com/spf13/pflag v1.0.6
	github.com/surfaceyu/edge-tts-go v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/term v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
)

replace github.com/kennyparsons/edge-tts-go => ../

replace github.com/surfaceyu/edge-tts-go => ./edge-tts-go
