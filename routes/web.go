package routes

import (
	"github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
)

func Web() {
	facades.Route().Get("/", func(ctx http.Context) http.Response {
		facades.Log().Info("Running")

		return ctx.Response().String(http.StatusOK, "Running")
	})
}
