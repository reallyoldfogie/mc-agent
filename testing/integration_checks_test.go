package testing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

const defaultTestServerImage = "itzg/minecraft-server:latest"

// RequireIntegrationEnv skips the test when Docker or the test server image is unavailable.
func RequireIntegrationEnv(t *testing.T, cfg ServerConfig) {
	t.Helper()

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
