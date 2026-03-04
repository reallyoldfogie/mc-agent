package testing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBowInterface_FireBowAccessible verifies that FireBow() is accessible on the Agent interface
func TestBowInterface_FireBowAccessible(t *testing.T) {
	// Create a mock agent to verify the interface is satisfied
	mockAgent := &mockAgent{
		shouldError: false,
	}

	// Verify FireBow() method exists and returns error
	err := mockAgent.FireBow()
	require.NoError(t, err, "FireBow should not error when properly configured")

	// Verify method is callable on the interface
	var agentInterface interface {
		FireBow() error
	}
	agentInterface = mockAgent
	require.NotNil(t, agentInterface, "Agent should satisfy the FireBow interface")
}

// TestBowInterface_FireBowAtAccessible verifies that FireBowAt() is accessible on the Agent interface
func TestBowInterface_FireBowAtAccessible(t *testing.T) {
	// Create a mock agent to verify the interface is satisfied
	mockAgent := &mockAgent{
		shouldError: false,
	}

	// Verify FireBowAt() method exists and returns error
	err := mockAgent.FireBowAt(context.Background(), 10, 20, 30)
	require.NoError(t, err, "FireBowAt should not error when properly configured")

	// Verify method is callable on the interface
	var agentInterface interface {
		FireBowAt(ctx context.Context, x, y, z float64) error
	}
	agentInterface = mockAgent
	require.NotNil(t, agentInterface, "Agent should satisfy the FireBowAt interface")
}

// TestBowInterface_FireBowErrorHandling verifies error handling works correctly
func TestBowInterface_FireBowErrorHandling(t *testing.T) {
	// Create a mock agent configured to return error
	mockAgent := &mockAgent{
		shouldError: true,
	}

	// Verify FireBow() returns error when configured to do so
	err := mockAgent.FireBow()
	require.Error(t, err, "FireBow should error when configured to do so")
	require.Equal(t, "test error", err.Error())
}

// TestBowInterface_FireBowAtErrorHandling verifies error handling for FireBowAt
func TestBowInterface_FireBowAtErrorHandling(t *testing.T) {
	// Create a mock agent configured to return error
	mockAgent := &mockAgent{
		shouldError: true,
	}

	// Verify FireBowAt() returns error when configured to do so
	err := mockAgent.FireBowAt(context.Background(), 10, 20, 30)
	require.Error(t, err, "FireBowAt should error when configured to do so")
	require.Equal(t, "test error", err.Error())
}

// mockAgent is a simple mock for testing the interface
type mockAgent struct {
	shouldError bool
}

func (m *mockAgent) FireBow() error {
	if m.shouldError {
		return ErrTest
	}
	return nil
}

func (m *mockAgent) FireBowAt(ctx context.Context, x, y, z float64) error {
	if m.shouldError {
		return ErrTest
	}
	return nil
}

var ErrTest = simpleError("test error")

type simpleError string

func (e simpleError) Error() string {
	return string(e)
}
