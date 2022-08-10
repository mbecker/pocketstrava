package models

import (
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-playground/validator"
	"github.com/labstack/echo/v5"
	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

const (
	VALIDATION_SETTINGS_EMAIL_DB         = "validation_settings_email_db"
	VALIDATION_SETTINGS_EMAIL_DBX        = "validation_settings_email_dbx"
	VALIDATION_SETTINGS_EMAIL_NOT_UNQIUE = "validation_settings_email_not_unique"
)

type Email struct {
	Email     string `json:"email" validate:"required,email"`
	Retrigger bool   `json:"retrigger"`
}

type Token struct {
	Token string `json:"token" validate:"required,token"`
}

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	if err := cv.validator.Struct(i); err != nil {
		// Optionally, you could return the error to give each route more control over the status code
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func EmailUniqueForUser(app *pocketbase.PocketBase, userId string, email string) (*models.Record, error) {
	emailCollection, err := app.Dao().FindCollectionByNameOrId(migrations.EmailCollectionName)
	if err != nil {
		return nil, validation.NewError(VALIDATION_SETTINGS_EMAIL_DB, "Collection emails not found")
	}
	expr := dbx.HashExp{migrations.EmailEmail: email, models.ProfileCollectionUserFieldName: userId, migrations.EmailValid: false}
	emailRecords, err := app.Dao().FindRecordsByExpr(emailCollection, expr)
	if err != nil {
		return nil, validation.NewError(VALIDATION_SETTINGS_EMAIL_DBX, "Collection emails records not found")
	}
	if len(emailRecords) == 1 {
		return emailRecords[0], nil
	}
	return nil, validation.NewError(VALIDATION_SETTINGS_EMAIL_NOT_UNQIUE, "Collection emails not unique")
}

func EmailSetValid(app *pocketbase.PocketBase, rec *models.Record) error {
	rec.SetDataValue(migrations.EmailValid, true)
	return app.Dao().SaveRecord(rec)
}
