package controllers

import (
	"errors"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/mail"

	"goravel/app/facades"
	discordservices "goravel/app/services/discord"
)

type WebhookController struct {
	github discordservices.Github
}

func NewWebhookController() *WebhookController {
	github, err := discordservices.NewGithub()
	if err != nil {
		facades.Log().Error(err.Error())
	}

	return &WebhookController{
		github: github,
	}
}

func (r *WebhookController) Github(ctx http.Context) http.Response {
	if err := r.github.ProcessWebhook(ctx.Request().Origin()); err != nil {
		facades.Log().Error(err.Error())

		if errors.Is(err, discordservices.ErrMissingSecret) || errors.Is(err, discordservices.ErrInvalidSignature) {
			return ctx.Response().Json(http.StatusUnauthorized, http.Json{
				"error": "invalid signature",
			})
		}

		facades.Mail().To([]string{facades.Config().GetString("mail.to")}).Subject("Github Webhook Failed").Content(mail.Content{Html: err.Error()}).Send()

		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"error": err.Error(),
		})
	}

	return ctx.Response().Json(http.StatusOK, http.Json{
		"message": "Webhook processed successfully",
	})
}
