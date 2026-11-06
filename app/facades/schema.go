package facades

import (
	"github.com/goravel/framework/contracts/database/schema"
)

// Schema returns the database schema facade.
func Schema() schema.Schema {
	return App().MakeSchema()
}
