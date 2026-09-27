package agent

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestProjectileRenderAndVelocityUpdateDoNotDeadlock exercises the two paths
// that previously acquired entitiesMu and activeProjectilesMu in opposite
// orders. Repeating the overlap makes the regression deterministic enough to
// catch the old nested-lock implementation without requiring a live server.
func TestProjectileRenderAndVelocityUpdateDoNotDeadlock(t *testing.T) {
	a := &agent{
		entities: map[int32]*trackedEntity{
			2: {EntityID: 2, X: 0, Y: 0, Z: 0},
		},
		activeProjectiles: map[int32]*activeProjectileInfo{
			1: {
				projectileType:    models.Arrow,
				currentServerTime: time.Now(),
				lastServerTime:    time.Now().Add(-50 * time.Millisecond),
				lastServerPos:     models.V3{X: 0, Y: 0, Z: 0},
				currentServerPos:  models.V3{X: 0, Y: 0, Z: 0},
			},
		},
	}

	start := make(chan struct{})
	done := make(chan struct{}, 2)
	go func() {
		<-start
		for i := 0; i < 1000; i++ {
			a.renderTick()
		}
		done <- struct{}{}
	}()
	go func() {
		<-start
		for i := 0; i < 1000; i++ {
			a.updateTrackedEntityVelocity(1, 0, 0, 0)
		}
		done <- struct{}{}
	}()
	close(start)

	deadline := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("projectile rendering and velocity updates deadlocked")
		}
	}
}
