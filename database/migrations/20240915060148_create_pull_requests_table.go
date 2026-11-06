package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M20240915060148CreatePullRequestsTable struct {
}

// Signature The unique signature for the migration.
func (r *M20240915060148CreatePullRequestsTable) Signature() string {
	return "20240915060148_create_pull_requests_table"
}

// Up Run the migrations.
func (r *M20240915060148CreatePullRequestsTable) Up() error {
	return facades.Schema().Create("pull_requests", func(table schema.Blueprint) {
		table.ID("id")
		table.UnsignedBigInteger("github_id")
		table.String("discord_thread_id")
		table.String("title")
		table.String("url")
		table.Timestamps()
	})
}

// Down Reverse the migrations.
func (r *M20240915060148CreatePullRequestsTable) Down() error {
	return facades.Schema().DropIfExists("pull_requests")
}
