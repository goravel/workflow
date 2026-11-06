package middleware

import (
	"crypto/subtle"

	"github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
)

type apiToken struct{}

func APIToken() http.Middleware { return &apiToken{} }

func (m *apiToken) Signature() string { return "api_token" }

func (m *apiToken) Handle(ctx http.Context) {
	expected := facades.Config().GetString("github.api_token")
	given := ctx.Request().Header("X-Api-Token")
	if expected == "" || subtle.ConstantTimeCompare([]byte(given), []byte(expected)) != 1 {
		ctx.Request().Abort(http.StatusUnauthorized)
		return
	}
	ctx.Request().Next()
}
