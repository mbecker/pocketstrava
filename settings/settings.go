package settings

import (
	"encoding/json"
	"log"
	"os"
	"strconv"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
)

func setInitialAdmin(app core.App) {
	// Init Admin
	admin := &models.Admin{}
	form := forms.NewAdminUpsert(app, admin)
	form.Email = os.Getenv("ADMIN_EMAIL")
	form.Password = os.Getenv("ADMIN_PASSWORD")
	form.PasswordConfirm = os.Getenv("ADMIN_PASSWORD")
	err := form.Submit()
	if err != nil {
		log.Printf("Error creating initial admin: err=%s", err)
	}
}

func SetAppSettings(e *core.ServeEvent) error {
	setInitialAdmin(e.App)
	var settings *core.Settings = core.NewSettings()
	settings.Meta.AppUrl = os.Getenv("APP_URL")
	settings.Meta.AppName = os.Getenv("APP_NAME")
	settings.StravaAuth.Enabled = true
	settings.StravaAuth.AllowRegistrations = true
	settings.StravaAuth.ClientId = os.Getenv("CLIENT_ID")
	settings.StravaAuth.ClientSecret = os.Getenv("CLIENT_SECRET")

	settings.UserAuthToken.Secret = os.Getenv("USER_AUTH_TOKEN_SECRET")
	settings.UserVerificationToken.Secret = os.Getenv("USER_VERIFICATION_TOKEN_SECRET")

	// SMTP
	smtpEnabled, err := strconv.ParseBool(os.Getenv("SMTP_ENABLED"))
	if err == nil && smtpEnabled {
		settings.Meta.SenderAddress = os.Getenv("META_SENDER_ADDRESS")
		settings.Smtp.Enabled = true
		settings.Smtp.Host = os.Getenv("SMTP_HOST")
		port, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
		if err != nil {
			settings.Smtp.Port = port
		}
		settings.Smtp.Password = os.Getenv(("SMTP_PASSWORD"))
		settings.Smtp.Username = os.Getenv(("SMTP_USERNAME"))
	}

	if len(os.Getenv("USER_CONFIRM_EMAIL_CHANGE_URL")) > 0 {
		confirmEmailChangeTemplate := settings.Meta.ConfirmEmailChangeTemplate
		confirmEmailChangeTemplate.ActionUrl = os.Getenv("USER_CONFIRM_EMAIL_CHANGE_URL")
		settings.Meta.ConfirmEmailChangeTemplate = confirmEmailChangeTemplate
	}

	/// Merge new with old / default settings; validate settings; marshal settings to JSON, update DB and refresh app settings
	err = e.App.Settings().Merge(settings)
	if err != nil {
		log.Printf("Error merging settings: err=%s", err)
		return err
	}
	err = e.App.Settings().Validate()
	if err != nil {
		log.Printf("Error validating merged settings: err=%s", err)
		return err
	}
	j, err := json.Marshal(e.App.Settings())
	if err != nil {
		log.Printf("Error marshal settings: err=%s", err)
		return err
	}
	_, err = e.App.DB().NewQuery("UPDATE `_params` SET `value`={:j} WHERE `key`='settings'").Bind(dbx.Params{"j": j}).Execute()
	if err != nil {
		log.Printf("Error update settings: err=%s", err)
		return err
	}
	log.Println("(SUCCESS) Update settings")
	e.App.RefreshSettings()
	return nil
}
