package bootstrap

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/database/migrations"
)

// Migrations returns the database migrations registered with the application.
func Migrations() []schema.Migration {
	return []schema.Migration{
		&migrations.M20240915060148CreatePullRequestsTable{},
		&migrations.M20261010120000CreateWebhookEventsTable{},
	}
}
