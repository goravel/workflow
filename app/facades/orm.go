package facades

import (
	"github.com/goravel/framework/contracts/database/orm"
)

// Orm returns the ORM facade.
func Orm() orm.Orm {
	return App().MakeOrm()
}
