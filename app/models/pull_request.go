package models

import (
	"github.com/goravel/framework/database/orm"
)

type PullRequest struct {
	orm.Model
	DiscordThreadID string
	GithubID        int64
	Title           string
	Url             string
}
