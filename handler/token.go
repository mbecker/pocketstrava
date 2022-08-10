package handler

import (
	"log"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	internalmodels "github.com/mbecker/pocketstrava/models"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/security"
)

func ParseToken(app *pocketbase.PocketBase, token string) (*models.Record, string, error) {
	claims, _ := security.ParseUnverifiedJWT(token)
	newEmail, _ := claims["newEmail"].(string)
	if newEmail == "" {
		return nil, "", validation.NewError("validation_invalid_token_payload", "Invalid token payload - newEmail must be set.")
	}

	// verify that the token is not expired and its signiture is valid
	user, err := app.Dao().FindUserByToken(
		token,
		app.Settings().UserEmailChangeToken.Secret,
	)
	if err != nil || user == nil {
		log.Println(err)
		return nil, "", validation.NewError("validation_invalid_token", "Invalid or expired token.")
	}

	// ensure that the mew email is unique for the user
	rec, err := internalmodels.EmailUniqueForUser(app, user.Id, newEmail)
	if err != nil {
		return nil, "", err
	}

	return rec, newEmail, nil
}
