package models

import (
	"github.com/goravel/framework/database/orm"
)

const PlatformGithub = "github"

type WebhookEvent struct {
	orm.Model
	orm.SoftDeletes
	Platform   string `json:"platform"`
	DeliveryID string `json:"delivery_id"`
	Event      string `json:"event"`
	Action     string `json:"action"`
	Repository string `json:"repository"`
	Sender     string `json:"sender"`
	Payload    string `json:"payload"`
}
