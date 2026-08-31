module github.com/reallyoldfogie/mc-agent

go 1.27.0

replace github.com/reallyoldfogie/mc-bot-go => /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-bot-go

replace github.com/reallyoldfogie/mc-protocol-go => /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-protocol-go

replace github.com/reallyoldfogie/mc-client-test-go => /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-client-test-go

replace github.com/reallyoldfogie/mc-replay-go => /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-replay-go

// Pinned to the local checkout rather than the v0.4.0 tag on origin so this
// branch can be developed/tested offline; see
// docs/plans/RL_POLICY_INTEGRATION_PLAN.md's dependency-wiring section for
// why v0.4.0 (a real tagged release, no replace needed) is the long-term
// intent once this lands and CI/network access to the tag is confirmed.
replace github.com/reallyoldfogie/cRL-go => /home/reallyoldfogie/src/github.com/reallyoldfogie/cRL-go

require (
	github.com/Tnze/go-mc v1.20.3-0.20240907175330-9a1f5431370e
	github.com/aquasecurity/go-version v0.0.1
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/google/uuid v1.6.0
	github.com/maxsupermanhd/go-mc-ms-auth v0.0.0-20230820124717-22f4d907eac4
	github.com/moby/moby/api v1.52.0
	github.com/moby/moby/client v0.2.1
	github.com/ollama/ollama v0.20.7
	github.com/pkg/errors v0.9.1
	github.com/reallyoldfogie/cRL-go v0.4.0
	github.com/reallyoldfogie/mc-bot-go v0.0.0-00010101000000-000000000000
	github.com/reallyoldfogie/mc-client-test-go v0.0.0-00010101000000-000000000000
	github.com/reallyoldfogie/mc-data-gen/loader v0.0.4
	github.com/reallyoldfogie/mc-protocol-go v0.0.0-00010101000000-000000000000
	github.com/reallyoldfogie/mc-replay-go v0.0.3
	github.com/stretchr/testify v1.12.1
	golang.org/x/tools v0.41.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gorcon/rcon v1.4.0 // indirect
	github.com/iancoleman/strcase v0.2.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.65.0 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/metric v1.41.0 // indirect
	go.opentelemetry.io/otel/trace v1.41.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/mod v0.32.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
)
