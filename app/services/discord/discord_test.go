package discord

import (
	"context"
	"testing"

	"github.com/goravel/framework/config"
	"github.com/goravel/framework/contracts/binding"
	contractstelemetry "github.com/goravel/framework/contracts/telemetry"
	"github.com/goravel/framework/foundation"
	"github.com/goravel/framework/foundation/json"
	frameworklog "github.com/goravel/framework/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"goravel/app/facades"
)

// setup binds a config and log facade so NewDiscord's info line has somewhere
// to go; booting the app package here would import-cycle through commands.
func setup(t *testing.T, token string) {
	t.Helper()
	foundation.App.Fresh()
	foundation.App.Instance(binding.Config, config.NewApplication(""))

	logger, err := frameworklog.NewApplication(context.Background(), nil, facades.Config(), json.New(), func() contractstelemetry.Telemetry {
		return nil
	})
	require.NoError(t, err)
	foundation.App.Instance(binding.Log, logger)

	facades.Config().Add("discord", map[string]any{"bot": map[string]any{"token": token}})
}

func TestNewDiscord_TokenEmpty(t *testing.T) {
	setup(t, "")

	impl, err := NewDiscord()
	require.NoError(t, err)
	assert.Nil(t, impl.session)

	threadID, err := impl.CreateThread("channel", Thread{Title: "title", Content: "content"})
	assert.ErrorIs(t, err, ErrNotConfigured)
	assert.Empty(t, threadID)
	assert.ErrorIs(t, impl.DeleteThread("thread"), ErrNotConfigured)
	assert.ErrorIs(t, impl.SendMessage("channel", "message"), ErrNotConfigured)
}

func TestCloseDiscord_NoSession(t *testing.T) {
	setup(t, "")

	_, err := NewDiscord()
	require.NoError(t, err)

	// CloseDiscord is the shutdown path the runner calls with Discord off.
	assert.NotPanics(t, CloseDiscord)
}
