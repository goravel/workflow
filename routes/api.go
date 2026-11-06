package routes

import (
	"goravel/app/facades"
	"goravel/app/http/controllers"
	"goravel/app/http/middleware"
)

func Api() {
	webhookController := controllers.NewWebhookController()
	webhookEventController := controllers.NewWebhookEventController()

	facades.Route().Post("/webhooks/github", webhookController.Github)

	events := facades.Route().Middleware(middleware.APIToken())
	events.Get("/webhooks/events", webhookEventController.Index)
	events.Delete("/webhooks/events/{id}", webhookEventController.Destroy)
}
