package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M20261010120000CreateWebhookEventsTable struct {
}

// Signature The unique signature for the migration.
func (r *M20261010120000CreateWebhookEventsTable) Signature() string {
	return "20261010120000_create_webhook_events_table"
}

// Up Run the migrations.
func (r *M20261010120000CreateWebhookEventsTable) Up() error {
	return facades.Schema().Create("webhook_events", func(table schema.Blueprint) {
		table.ID("id")
		table.String("platform")
		table.String("delivery_id")
		table.String("event")
		table.String("action")
		table.String("repository")
		table.String("sender")
		table.LongText("payload")
		table.Timestamps()
		table.SoftDeletes()
		table.Unique("platform", "delivery_id")
		table.Index("event")
		table.Index("deleted_at", "id")
	})
}

// Down Reverse the migrations.
func (r *M20261010120000CreateWebhookEventsTable) Down() error {
	return facades.Schema().DropIfExists("webhook_events")
}
