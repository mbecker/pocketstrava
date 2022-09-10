package models

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

type Email struct {
	Email     string `json:"email" validate:"required,email"`
	Retrigger bool   `json:"retrigger"`
}

type Token struct {
	Token string `json:"token" validate:"required,token"`
}

func EmailUniqueForUser(app *pocketbase.PocketBase, userId string, email string) (*models.Record, error) {
	emailCollection, err := app.Dao().FindCollectionByNameOrId(migrations.NotificationsCollectionName)
	if err != nil {
		return nil, validation.NewError(VALIDATION_SETTINGS_EMAIL_DB, "Collection emails not found")
	}
	expr := dbx.HashExp{migrations.NotificationsEmail: email, models.ProfileCollectionUserFieldName: userId, migrations.NotificationsValid: false}
	emailRecords, err := app.Dao().FindRecordsByExpr(emailCollection, expr)
	if err != nil {
		return nil, validation.NewError(VALIDATION_SETTINGS_EMAIL_DBX, "Collection emails records not found")
	}
	if len(emailRecords) == 1 {
		return emailRecords[0], nil
	}
	return nil, validation.NewError(VALIDATION_SETTINGS_EMAIL_NOT_UNQIUE, "Collection emails not unique")
}

// EmailCount counts the emails in the emailcollection for the given user id
func EmailCount(app *pocketbase.PocketBase, userId string, email string) error {
	emailCollection, err := app.Dao().FindCollectionByNameOrId(migrations.NotificationsCollectionName)
	if err != nil {
		return validation.NewError(VALIDATION_SETTINGS_EMAIL_DB, "Collection emails not found")
	}
	expr := dbx.HashExp{models.ProfileCollectionUserFieldName: userId}
	emailRecord, err := app.Dao().FindRecordsByExpr(emailCollection, expr)
	if err != nil {
		return validation.NewError(VALIDATION_SETTINGS_EMAIL_COUNT_DBX, "Collection emails records count")
	}
	if len(emailRecord) != 1 {
		return validation.NewError(VALIDATION_SETTINGS_EMAIL_COUNT_DBX, "Collection emails records count is not 1")
	}
	if emailRecord[0].GetStringDataValue(migrations.NotificationsEmail) != email {
		return validation.NewError(VALIDATION_SETTINGS_EMAIL_COUNT_DBX, "Collection emails records count email is not the given email")
	}
	return nil
}

// EmailSetValid sets the email valid and if it's the only email set it's primary=true
func EmailSetValid(app *pocketbase.PocketBase, rec *models.Record) error {
	rec.SetDataValue(migrations.NotificationsValid, true)
	userId := rec.GetStringDataValue(models.ProfileCollectionUserFieldName)
	email := rec.GetStringDataValue(migrations.NotificationsEmail)
	err := EmailCount(app, userId, email)
	// The user id has only email and the given email is the db email; set the email to primary
	if err == nil {
		rec.SetDataValue(migrations.NotificationsPrimary, true)
	}
	return app.Dao().SaveRecord(rec)
}
