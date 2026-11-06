package facades

import (
	"github.com/goravel/framework/contracts/mail"
)

// Mail returns the mail facade.
func Mail() mail.Mail {
	return App().MakeMail()
}
