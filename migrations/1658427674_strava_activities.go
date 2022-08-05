package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
)

// Activity
const (
	ActivityCollectionName     = "activities"
	ActivityId                 = "activityid"
	ActivityExternalId         = "externalid"
	ActivityUploadId           = "uploadid"
	ActivityAthleteId          = "athlete_id"
	ActivityName               = "name"
	ActivityDistance           = "distance"
	ActivityMovingTime         = "moving_time"
	ActivityElapsedTime        = "elapsed_time"
	ActivityTotalElevationGain = "total_elevation_gain"
	ActivityType               = "type"
	ActivityStartDate          = "start_date"
	ActivityStartDateLocal     = "start_date_local"
	ActivityTimeZone           = "time_zone"
	ActivityStartLatLng        = "start_latlng"
	ActivityEndLatLng          = "end_latlng"
	ActivityMapId              = "map_id"
	ActivityMapPolyline        = "map_polyline"
	Activity                   = "activity"
)

func init() {
	m.Register(func(db dbx.Builder) error {
		// add up queries...
		// inserts the system profiles collection
		// -----------------------------------------------------------
		profileOwnerRule := fmt.Sprintf("%s = @request.user.id", models.ProfileCollectionUserFieldName)
		collection := &models.Collection{
			Name:       ActivityCollectionName,
			System:     false,
			CreateRule: &profileOwnerRule,
			ListRule:   &profileOwnerRule,
			ViewRule:   &profileOwnerRule,
			UpdateRule: &profileOwnerRule,
			Schema: schema.NewSchema(
				&schema.SchemaField{
					Name:     models.ProfileCollectionUserFieldName,
					Type:     schema.FieldTypeUser,
					Unique:   false,
					Required: true,
					System:   true,
					Options: &schema.UserOptions{
						MaxSelect:     1,
						CascadeDelete: true,
					},
				},
				&schema.SchemaField{
					Name: ActivityId,
					Type: schema.FieldTypeNumber,
					// // Options:  &schema.NumberOptions{},
					Required: true,
					Unique:   true,
				},
				&schema.SchemaField{
					Name: ActivityExternalId,
					Type: schema.FieldTypeText,
					// // Options:  &schema.NumberOptions{},
					Required: false,
					Unique:   false,
				},
				&schema.SchemaField{
					Name: ActivityUploadId,
					Type: schema.FieldTypeNumber,
					// // Options:  &schema.NumberOptions{},
					Required: false,
					Unique:   false,
				},
				&schema.SchemaField{
					Name: ActivityAthleteId,
					Type: schema.FieldTypeNumber,
					// Options:  &schema.NumberOptions{},
					Required: true,
					Unique:   false,
				},
				&schema.SchemaField{
					Name: ActivityName,
					Type: schema.FieldTypeText,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityDistance,
					Type: schema.FieldTypeNumber,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityMovingTime,
					Type: schema.FieldTypeNumber,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityElapsedTime,
					Type: schema.FieldTypeNumber,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityTotalElevationGain,
					Type: schema.FieldTypeNumber,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityType,
					Type: schema.FieldTypeText,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityStartDate,
					Type: schema.FieldTypeDate,
					// Options:  &schema.DateOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityStartDateLocal,
					Type: schema.FieldTypeDate,
					// Options:  &schema.DateOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityTimeZone,
					Type: schema.FieldTypeText,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityStartLatLng,
					Type: schema.FieldTypeJson,
					// Options:  &schema.JsonOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityEndLatLng,
					Type: schema.FieldTypeJson,
					// Options:  &schema.JsonOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityMapId,
					Type: schema.FieldTypeText,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: ActivityMapPolyline,
					Type: schema.FieldTypeText,
					// Options:  &schema.NumberOptions{},
					Required: false,
				},
				&schema.SchemaField{
					Name: Activity,
					Type: schema.FieldTypeJson,
					// Options:  &schema.JsonOptions{},
					Required: false,
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
