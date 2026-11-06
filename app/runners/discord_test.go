package runners

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/goravel/framework/foundation"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	frameworkmock "github.com/goravel/framework/testing/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newAppMock returns a framework Application mock whose Context returns ctx. When
// entered is non-nil it is closed, exactly once, on the first Context call so a
// test can deterministically observe that Run reached its blocking wait.
func newAppMock(t *testing.T, ctx context.Context, entered chan struct{}) *mocksfoundation.Application {
	t.Helper()

	// mock.Factory() installs a mocked application as the package-level
	// foundation.App, so restore the previous one when the test finishes.
	originalApp := foundation.App
	mockFactory := frameworkmock.Factory()
	t.Cleanup(func() { foundation.App = originalApp })

	appMock := mockFactory.App()
	appMock.Test(t)

	// A bare Run hook would fire on every Context call and a second close(entered)
	// would panic, so guard the signal with a sync.Once.
	var once sync.Once
	appMock.On("Context").Return(ctx).Run(func(mock.Arguments) {
		if entered != nil {
			once.Do(func() {
				close(entered)
			})
		}
	})

	return appMock
}

func TestDiscordRunnerSignature(t *testing.T) {
	runner := NewDiscord(newAppMock(t, context.Background(), nil))

	require.Equal(t, "discord", runner.Signature())
}

func TestDiscordRunnerShouldRun(t *testing.T) {
	runner := NewDiscord(newAppMock(t, context.Background(), nil))

	require.True(t, runner.ShouldRun())
}

func TestDiscordRunnerRunBlocksUntilContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	runner := NewDiscord(newAppMock(t, ctx, entered))

	done := make(chan error, 1)
	go func() {
		done <- runner.Run()
	}()

	// Wait until Run is inside the blocking wait before asserting it has not
	// returned.
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Run did not reach the blocking wait")
	}

	select {
	case err := <-done:
		t.Fatalf("Run returned before the context was cancelled: %v", err)
	default:
	}

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

func TestDiscordRunnerShutdownReturnsNil(t *testing.T) {
	runner := NewDiscord(newAppMock(t, context.Background(), nil))

	require.NoError(t, runner.Shutdown())
}
