module github.com/Gorakhnath-R-Patil/Pulse

go 1.25.0

require (
	github.com/cilium/ebpf v0.22.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	go.opentelemetry.io/proto/otlp v1.11.0
	golang.org/x/sys v0.47.0
	google.golang.org/grpc v1.83.2
)

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260720211330-0afa2a65878a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260720211330-0afa2a65878a // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

tool github.com/cilium/ebpf/cmd/bpf2go
