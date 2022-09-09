package mails

import (
	"html/template"
	"net/mail"
	"os"

	"github.com/mbecker/pocketstrava/templates"
	"github.com/mbecker/pocketstrava/tokens"
	"github.com/pocketbase/pocketbase/core"
)

// SendCustomEmail sends an email to the email address with the template string (html/template) as html body with the PocketBase mail client.
// The hooks OnMailerBeforeCustomEmailSend and OnMailerAfterCustomEmailSend are triggered.
func SendNotificationVerificationEmail(app core.App, userId string, email string) error {
	token, tokenErr := tokens.NewNotificationVerificationToken(app, userId, email)
	if tokenErr != nil {
		return tokenErr
	}

	mailClient := app.NewMailClient()

	templates.DefaultNotificationVerificationTemplate.ActionUrl = os.Getenv("NOTIFICATION_VERIFICATION_URL")

	subject, body, err := resolveEmailTemplate(app, token, templates.DefaultNotificationVerificationTemplate)
	if err != nil {
		return err
	}

	return mailClient.Send(
		mail.Address{
			Name:    app.Settings().Meta.SenderName,
			Address: app.Settings().Meta.SenderAddress,
		},
		mail.Address{Address: email},
		subject,
		body,
		nil,
	)
}

func resolveEmailTemplate(
	app core.App,
	token string,
	emailTemplate core.EmailTemplate,
) (subject string, body string, err error) {
	settings := app.Settings()

	subject, rawBody, _ := emailTemplate.Resolve(
		settings.Meta.AppName,
		settings.Meta.AppUrl,
		token,
	)

	params := struct {
		HtmlContent template.HTML
	}{
		HtmlContent: template.HTML(rawBody),
	}

	body, err = resolveTemplateContent(params, templates.Layout, templates.HtmlBody)
	if err != nil {
		return "", "", err
	}

	return subject, body, nil
}
