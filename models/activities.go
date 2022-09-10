package models

import (
	"fmt"

	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
	strava "github.com/strava/go.strava"
)

type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func createActivities(app *pocketbase.PocketBase, userId string, activities []*strava.ActivitySummary) error {

	activityCollection, err := app.Dao().FindCollectionByNameOrId(migrations.ActivityCollectionName)
	if err != nil {
		return err
	}

	// "SELECT * FROM _users WHERE email={:email}",
	for _, a := range activities {
		// expr := dbx.HashExp{migrations.ActivityId: a.Id}
		// activityRecords, err := app.Dao().FindRecordsByExpr(activityCollection, expr)
		// if err != nil {
		// 	log.Panicln(err)
		// 	continue
		// }
		// if len(activityRecords) > 0 {
		// 	log.Printf("Activity Record found: activity.id=%d", a.Id)
		// 	continue
		// }

		startLatLng := LatLng{
			Lat: a.StartLocation[0],
			Lng: a.StartLocation[1],
		}
		endLatLng := LatLng{
			Lat: a.EndLocation[0],
			Lng: a.EndLocation[1],
		}
		// activity, err := json.Marshal(a)
		// if err != nil {
		// 	log.Println(err)
		// 	continue
		// }
		rec := models.NewRecord(activityCollection)
		rec.SetDataValue(migrations.ProfileCollectionUserId, userId)
		rec.SetDataValue(migrations.ActivityId, a.Id)
		rec.SetDataValue(migrations.ActivityExternalId, a.ExternalId)
		rec.SetDataValue(migrations.ActivityUploadId, a.UploadId)
		rec.SetDataValue(migrations.ActivityAthleteId, a.Athlete.Id)
		rec.SetDataValue(migrations.ActivityName, a.Name)
		rec.SetDataValue(migrations.ActivityDistance, a.Distance)
		rec.SetDataValue(migrations.ActivityMovingTime, a.MovingTime)
		rec.SetDataValue(migrations.ActivityElapsedTime, a.ElapsedTime)
		rec.SetDataValue(migrations.ActivityTotalElevationGain, a.TotalElevationGain)
		rec.SetDataValue(migrations.ActivityType, a.Type)
		rec.SetDataValue(migrations.ActivityStartDate, a.StartDate)
		rec.SetDataValue(migrations.ActivityStartDateLocal, a.StartDateLocal)
		rec.SetDataValue(migrations.ActivityTimeZone, a.TimeZone)
		rec.SetDataValue(migrations.ActivityStartLatLng, startLatLng)
		rec.SetDataValue(migrations.ActivityEndLatLng, endLatLng)
		rec.SetDataValue(migrations.ActivityMapId, a.Map.Id)
		rec.SetDataValue(migrations.ActivityMapPolyline, string(a.Map.SummaryPolyline))
		rec.SetDataValue(migrations.Activity, a)
		err := forms.NewRecordUpsert(app, rec).Submit()
		if err != nil {
			fmt.Println(err)
			continue
		}

		// HACK: will change in v0.4.0 and the hooks will be replaced with onModelAfterCreate and OnModelAfterUpdate (they are triggered automatically on successful insert/update db operation).
		event := &core.RecordCreateEvent{Record: rec}
		app.OnRecordAfterCreateRequest().Trigger(event)

		// event := &core.RecordUpdateEvent{Record: rec}
		// app.OnRecordAfterUpdateRequest().Trigger(event)
	}
	return nil

}
