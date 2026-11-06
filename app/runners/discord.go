package runners

import (
	"github.com/goravel/framework/contracts/foundation"

	discordservices "goravel/app/services/discord"
)

// Discord is an application runner that closes the shared Discord session on shutdown.
type Discord struct {
	app foundation.Application
}

// NewDiscord creates a Discord runner bound to the given application.
func NewDiscord(app foundation.Application) *Discord {
	return &Discord{app: app}
}

// Signature returns the unique runner signature.
func (r *Discord) Signature() string { return "discord" }

// ShouldRun reports whether the runner should start.
func (r *Discord) ShouldRun() bool { return true }

// Run blocks until the application context is cancelled on SIGINT/SIGTERM.
func (r *Discord) Run() error {
	<-r.app.Context().Done()
	return nil
}

// Shutdown closes the shared Discord session.
func (r *Discord) Shutdown() error {
	discordservices.CloseDiscord()
	return nil
}
