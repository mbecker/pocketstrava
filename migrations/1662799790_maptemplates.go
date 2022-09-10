package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
)

// Email
const (
	MapTemplatesCollectionName = "maptemplates"
	MapTemplatesJson           = "json"
	MapTemplatesPrimary        = "primary"
)

func init() {
	m.Register(func(db dbx.Builder) error {
		profileOwnerRule := fmt.Sprintf("%s = @request.user.id", models.ProfileCollectionUserFieldName)
		collection := &models.Collection{
			Name:       MapTemplatesCollectionName,
			System:     false,
			CreateRule: &profileOwnerRule,
			ListRule:   &profileOwnerRule,
			ViewRule:   &profileOwnerRule,
			UpdateRule: &profileOwnerRule,
			DeleteRule: &profileOwnerRule,
			Schema: schema.NewSchema(
				&schema.SchemaField{
					Name:     models.ProfileCollectionUserFieldName,
					Type:     schema.FieldTypeUser,
					Unique:   true,
					Required: true,
					System:   true,
					Options: &schema.UserOptions{
						MaxSelect:     1,
						CascadeDelete: true,
					},
				},
				&schema.SchemaField{
					Name: MapTemplatesJson,
					Type: schema.FieldTypeJson,
					// Options:  &schema.NumberOptions{},
					Required: true,
					Unique:   false,
					Options:  &schema.JsonOptions{},
				},
				&schema.SchemaField{
					Name:     MapTemplatesPrimary,
					Type:     schema.FieldTypeBool,
					Options:  &schema.BoolOptions{},
					Required: true,
					Unique:   false,
				})}
		return daos.New(db).SaveCollection(collection)
	}, func(db dbx.Builder) error {
		tables := []string{
			MapTemplatesCollectionName,
		}

		for _, name := range tables {
			if _, err := db.DropTable(name).Execute(); err != nil {
				return err
			}
		}
		return nil
	})
}
