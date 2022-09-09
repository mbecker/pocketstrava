package tokens

import (
	"github.com/golang-jwt/jwt/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

// NewNotificationVerificationToken generates and returns a new `notification verification token` request.
func NewNotificationVerificationToken(app core.App, userId string, email string) (string, error) {
	return security.NewToken(
		jwt.MapClaims{"id": userId, "type": "notification", "email": email},
		(app.Settings().UserEmailChangeToken.Secret),
		app.Settings().UserEmailChangeToken.Duration,
	)
}
