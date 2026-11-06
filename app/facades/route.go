package facades

import (
	"github.com/goravel/framework/contracts/route"
)

// Route returns the routing facade.
func Route() route.Route {
	return App().MakeRoute()
}
