package models

import (
	"net/http"

	"github.com/go-playground/validator"
	"github.com/labstack/echo/v5"
)

const (
	VALIDATION_SETTINGS_EMAIL_DB         = "validation_settings_email_db"
	VALIDATION_SETTINGS_EMAIL_DBX        = "validation_settings_email_dbx"
	VALIDATION_SETTINGS_EMAIL_NOT_UNQIUE = "validation_settings_email_not_unique"
	VALIDATION_SETTINGS_EMAIL_COUNT_DBX  = "validation_settings_email_count_dbx"
)

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
