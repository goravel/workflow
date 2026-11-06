package config

import (
	"goravel/app/facades"
)

func init() {
	config := facades.Config()
	config.Add("github", map[string]any{
		"webhook_secret": config.Env("GITHUB_WEBHOOK_SECRET", ""),
		"api_token":      config.Env("API_TOKEN", ""),
	})
}
