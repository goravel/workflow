package config

import (
	"goravel/app/facades"
)

func init() {
	config := facades.Config()
	config.Add("discord", map[string]any{
		"bot": map[string]any{
			"token": config.Env("DISCORD_BOT_TOKEN", ""),
		},
		"heartbeat": map[string]any{
			"url":        []string{"https://www.goravel.dev/heart.html"},
			"channel_id": config.Env("DISCORD_HEARTBEAT_CHANNEL_ID", ""),
		},
		"pull_requests": map[string]any{
			"channel_id": config.Env("DISCORD_PULL_REQUESTS_CHANNEL_ID", ""),
		},
		"roles": map[string]any{
			"core": config.Env("DISCORD_CORE_ROLE_ID", ""),
		},
	})
}
