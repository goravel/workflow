package bootstrap

import (
	"github.com/goravel/framework/ai"
	"github.com/goravel/framework/cache"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/database"
	"github.com/goravel/framework/http"
	"github.com/goravel/framework/log"
	"github.com/goravel/framework/mail"
	"github.com/goravel/framework/process"
	"github.com/goravel/framework/queue"
	"github.com/goravel/framework/route"
	"github.com/goravel/framework/schedule"
	"github.com/goravel/framework/session"
	"github.com/goravel/framework/testing"
	"github.com/goravel/framework/validation"
	"github.com/goravel/framework/view"
	"github.com/goravel/gin"
	"github.com/goravel/openai"
	"github.com/goravel/sqlite"
)

func Providers() []foundation.ServiceProvider {
	return []foundation.ServiceProvider{
		&ai.ServiceProvider{},
		&openai.ServiceProvider{},
		&http.ServiceProvider{},
		&log.ServiceProvider{},
		&database.ServiceProvider{},
		&sqlite.ServiceProvider{},
		&cache.ServiceProvider{},
		&schedule.ServiceProvider{},
		&process.ServiceProvider{},
		&route.ServiceProvider{},
		&gin.ServiceProvider{},
		&validation.ServiceProvider{},
		&view.ServiceProvider{},
		&session.ServiceProvider{},
		&queue.ServiceProvider{},
		&mail.ServiceProvider{},
		&testing.ServiceProvider{},
	}
}
