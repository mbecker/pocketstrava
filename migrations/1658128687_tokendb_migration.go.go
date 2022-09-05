package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
)

const (
	ProfileCollectionUserId            = "userId"
	OAuthTokenCollectionName           = "oauthtokens"
	OAuthTokenCollectionNameProvider   = "provider"
	OAuthTokenCollectionAccessToken    = "access_token"
	OAuthTokenCollectionRefreshToken   = "refresh_token"
	OAuthTokenCollectionTokenType      = "token_type"
	OAuthTokenCollectionExpiry         = "expiry"
	OAuthTokenCollectionProviderUserId = "provideruserid"
)

func init() {
	m.Register(func(db dbx.Builder) error {
		// add up queries...
		// inserts the system profiles collection
		// -----------------------------------------------------------
		profileOwnerRule := fmt.Sprintf("%s = @request.user.id", models.ProfileCollectionUserFieldName)
		collection := &models.Collection{
			Name:       OAuthTokenCollectionName,
			System:     false,
			CreateRule: &profileOwnerRule,
			ListRule:   &profileOwnerRule,
			ViewRule:   &profileOwnerRule,
			UpdateRule: &profileOwnerRule,
			DeleteRule: &profileOwnerRule,
			Schema: schema.NewSchema(
				&schema.SchemaField{
					Name:     OAuthTokenCollectionNameProvider,
					Type:     schema.FieldTypeText,
					Unique:   false,
					Required: true,
					System:   false,
				},
				&schema.SchemaField{
					Name:     models.ProfileCollectionUserFieldName,
					Type:     schema.FieldTypeUser,
					Unique:   false,
					Required: true,
					System:   false,
					Options: &schema.UserOptions{
						MaxSelect:     1,
						CascadeDelete: true,
					},
				},
				&schema.SchemaField{
					Name:     OAuthTokenCollectionProviderUserId,
					Type:     schema.FieldTypeText,
					Options:  &schema.TextOptions{},
					Required: true,
				},
				&schema.SchemaField{
					Name:     OAuthTokenCollectionAccessToken,
					Type:     schema.FieldTypeText,
					Options:  &schema.TextOptions{},
					Required: true,
				},
				&schema.SchemaField{
					Name:     OAuthTokenCollectionRefreshToken,
					Type:     schema.FieldTypeText,
					Options:  &schema.TextOptions{},
					Required: true,
				},
				&schema.SchemaField{
					Name:     OAuthTokenCollectionTokenType,
					Type:     schema.FieldTypeText,
					Options:  &schema.TextOptions{},
					Required: true,
				},
				&schema.SchemaField{
					Name:     OAuthTokenCollectionExpiry,
					Type:     schema.FieldTypeDate,
					Options:  &schema.TextOptions{},
					Required: true,
				},
			),
		}

		return daos.New(db).SaveCollection(collection)
	}, func(db dbx.Builder) error {
		tables := []string{
			OAuthTokenCollectionName,
		}

		for _, name := range tables {
			if _, err := db.DropTable(name).Execute(); err != nil {
				return err
			}
		}

		return nil
	})
}
