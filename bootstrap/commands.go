package bootstrap

import (
	"github.com/goravel/framework/contracts/console"

	"goravel/app/console/commands"
)

// Commands returns the application console commands.
func Commands() []console.Command {
	return []console.Command{
		commands.NewMajor(),
		commands.NewPatch(),
		commands.NewPreview(),
		&commands.PollEvents{},
	}
}
