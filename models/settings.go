package models

import (
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-playground/validator"
	"github.com/labstack/echo/v5"
	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

const (
	VALIDATION_SETTINGS_EMAIL_DB         = "validation_settings_email_db"
	VALIDATION_SETTINGS_EMAIL_DBX        = "validation_settings_email_dbx"
	VALIDATION_SETTINGS_EMAIL_NOT_UNQIUE = "validation_settings_email_not_unique"
	VALIDATION_SETTINGS_EMAIL_COUNT_DBX  = "validation_settings_email_count_dbx"
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

func IsRecordUnique(dao *daos.Dao, record *models.Record) bool {
	var exists bool

	expr := dbx.HashExp{}
	data := record.Data()
	for k, v := range data {
		expr[k] = v
	}

	err := dao.RecordQuery(record.Collection()).
		Select("count(*)").
		AndWhere(dbx.Not(dbx.HashExp{"id": record.Id})).
		AndWhere(expr).
		Limit(1).
		Row(&exists)

	return err == nil && !exists
}

// EmailCount counts the emails in the emailcollection for the given user id
func EmailCount(app *pocketbase.PocketBase, userId string, email string) error {
	emailCollection, err := app.Dao().FindCollectionByNameOrId(migrations.EmailCollectionName)
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
	if emailRecord[0].GetStringDataValue(migrations.EmailEmail) != email {
		return validation.NewError(VALIDATION_SETTINGS_EMAIL_COUNT_DBX, "Collection emails records count email is not the given email")
	}
	return nil
}

// EmailSetValid sets the email valid and if it's the only email set it's primary=true
func EmailSetValid(app *pocketbase.PocketBase, email string, rec *models.Record) error {
	rec.SetDataValue(migrations.EmailValid, true)
	userId := rec.GetStringDataValue(models.ProfileCollectionUserFieldName)
	err := EmailCount(app, userId, email)
	// The user id has only email and the given email is the db email; set the email to primary
	if err == nil {
		rec.SetDataValue(migrations.EmailPrimary, true)
	}
	return app.Dao().SaveRecord(rec)
}
