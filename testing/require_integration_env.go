package testing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

const defaultTestServerImage = "itzg/minecraft-server:latest"

// RequireIntegrationEnv skips the test when Docker or the test server image is unavailable.
func RequireIntegrationEnv(t *testing.T, cfg ServerConfig) {
	t.Helper()

	if host := os.Getenv(client.EnvOverrideHost); strings.HasPrefix(host, "ssh://") {
		// mc-client-test-go's testenv.Manager opens a dedicated SSH tunnel
		// per container for this case rather than handing ssh:// to the
		// Docker client directly (which has no real SSH transport -
		// confirmed directly: it attempts a plain DNS lookup of the host,
		// not an SSH connection). There's no cheap way to pre-check
		// reachability here without actually opening a tunnel, which is
		// more than this fast-skip precheck is worth doing - instead, skip
		// straight to Framework.StartServer and let its own Start (via a
		// real, tunnel-aware Ping with a clear error) be the first signal.
		// This means an unreachable ssh:// target now surfaces as a test
		// FAILURE rather than a graceful SKIP here - acceptable, since
		// DOCKER_HOST is only ever set to ssh://... deliberately (never by
		// a default config or CI), so failing loudly on a target someone
		// explicitly asked to use is more useful than silently skipping.
		return
	}

	cli, err := client.New(client.FromEnv)
	if err != nil {
		t.Skipf("docker client unavailable: %v", err)
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
		return
	}

	if err := ensureServerImage(ctx, cli, cfg); err != nil {
		t.Skipf("minecraft server image unavailable: %v", err)
	}
}

func ensureServerImage(ctx context.Context, cli *client.Client, cfg ServerConfig) error {
	image := defaultTestServerImage
	if envImage := os.Getenv("MC_AGENT_TEST_IMAGE"); envImage != "" {
		image = envImage
	}

	if _, err := cli.ImageInspect(ctx, image); err == nil {
		return nil
	}

	if cfg.PullImage {
		// The server start will attempt to pull; allow that path to proceed.
		return nil
	}

	return fmt.Errorf("image %q not present (pre-pull or set PullImage=true)", image)
}
