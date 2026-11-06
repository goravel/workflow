package bootstrap

import (
	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/foundation"

	"goravel/app/runners"
	"goravel/config"
	"goravel/routes"
)

func Boot() contractsfoundation.Application {
	return foundation.Setup().
		WithProviders(Providers).
		WithConfig(config.Boot).
		WithCommands(Commands).
		WithSchedule(Schedules).
		WithMigrations(Migrations).
		WithRunners(func() []contractsfoundation.Runner {
			return []contractsfoundation.Runner{
				runners.NewDiscord(foundation.App),
			}
		}).
		WithRouting(func() {
			routes.Web()
			routes.Api()
			routes.Grpc()
		}).
		Create()
}
