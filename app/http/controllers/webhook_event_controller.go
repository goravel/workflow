package controllers

import (
	"github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
	"goravel/app/models"
)

type WebhookEventController struct {
}

func NewWebhookEventController() *WebhookEventController {
	return &WebhookEventController{}
}

// Index returns a paginated list of non-deleted events, newest first.
func (r *WebhookEventController) Index(ctx http.Context) http.Response {
	page := ctx.Request().QueryInt("page", 1)
	if page <= 0 {
		page = 1
	} else if page > 100000 {
		page = 100000
	}
	limit := ctx.Request().QueryInt("limit", 50)
	if limit <= 0 {
		limit = 50
	} else if limit > 500 {
		limit = 500
	}

	query := facades.Orm().Query().Model(&models.WebhookEvent{})
	if platform := ctx.Request().Query("platform"); platform != "" {
		query = query.Where("platform", platform)
	}
	if event := ctx.Request().Query("event"); event != "" {
		query = query.Where("event", event)
	}

	events := make([]models.WebhookEvent, 0, limit)
	var total int64
	if err := query.OrderByDesc("id").Paginate(page, limit, &events, &total); err != nil {
		facades.Log().Error(err.Error())
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": "internal error"})
	}

	return ctx.Response().Json(http.StatusOK, http.Json{
		"data":  events,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Destroy soft-deletes a single event: 200 (no body) on success, 404 otherwise.
func (r *WebhookEventController) Destroy(ctx http.Context) http.Response {
	id := ctx.Request().RouteInt64("id")
	if id <= 0 {
		return ctx.Response().NoContent(http.StatusNotFound)
	}

	result, err := facades.Orm().Query().Where("id", id).Delete(&models.WebhookEvent{})
	if err != nil {
		facades.Log().Error(err.Error())
		return ctx.Response().NoContent(http.StatusInternalServerError)
	}
	if result.RowsAffected == 0 {
		return ctx.Response().NoContent(http.StatusNotFound)
	}

	return ctx.Response().NoContent(http.StatusOK)
}
