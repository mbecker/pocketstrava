package handler

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/security"
)

func ParseNotificationVerificationToken(app *pocketbase.PocketBase, token string) (*models.Record, error) {
	claims, err := security.ParseJWT(token, app.Settings().UserEmailChangeToken.Secret)
	if err != nil {
		return nil, err
	}
	email, _ := claims["email"].(string)
	if email == "" {
		return nil, validation.NewError("validation_invalid_token_payload", "Invalid token payload - email must be set.")
	}

	tokenType, _ := claims["type"].(string)
	if tokenType == "" || tokenType != "notification" {
		return nil, validation.NewError("validation_invalid_token_payload", "Invalid token payload - type must be set.")
	}

	userId, _ := claims["id"].(string)
	if userId == "" {
		return nil, validation.NewError("validation_invalid_token_payload", "Invalid token payload - id must be set.")
	}

	emailCollection, err := app.Dao().FindCollectionByNameOrId(migrations.EmailCollectionName)
	if err != nil {
		return nil, err
	}
	expr := dbx.HashExp{"email": email, models.ProfileCollectionUserFieldName: userId, migrations.EmailValid: false}
	emailRecords, err := app.Dao().FindRecordsByExpr(emailCollection, expr)
	if err != nil {
		return nil, err
	}
	if len(emailRecords) != 1 {
		return nil, validation.NewError("validation_notification_count", "Invalid notification count for user, email and validation")
	}
	return emailRecords[0], nil
}
