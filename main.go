package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/golang/geo/s2"
	"github.com/twpayne/go-polyline"

	staticmaps "github.com/flopp/go-staticmaps"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/pocketbase/pocketbase/tools/rest"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/robfig/cron/v3"
	strava "github.com/strava/go.strava"
	"golang.org/x/oauth2"
)

const ContextUserKey string = "user"

var ErrNoUser = errors.New("no user")

type StravaActivitiesMsg struct {
	Before int64 `json:"before"`
	After  int64 `json:"after"`
}

var kp *kafka.Producer
var kptopic string

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Initialize app, cronjob, kafka
	app := pocketbase.New()
	c := cron.New()

	kptopic = os.Getenv("TOPIC")
	kp, err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": os.Getenv("BOOTSTRAP_SERVER")})
	if err != nil {
		panic(err)
	}

	defer kp.Close()

	// Delivery report handler for produced messages
	go func() {
		for e := range kp.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					fmt.Printf("Kafka Producer delivery failed: %v\n", ev.TopicPartition)
				} else {
					fmt.Printf("Kafka Producer delivered message to %v\n", ev.TopicPartition)
				}
			}
		}
	}()

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		var err error
		var res sql.Result
		// res, err := e.App.Dao().DB().NewQuery("CREATE UNIQUE INDEX activities_activityid_IDX ON activities (activityid)").Execute()
		// if err != nil {
		// 	log.Printf("Error create index: err=%s", err)
		// } else {
		// 	log.Printf("Success create index: res=%s", res)
		// }

		// Init Admin
		admin := &models.Admin{}
		form := forms.NewAdminUpsert(app, admin)
		form.Email = os.Getenv("ADMIN_EMAIL")
		form.Password = os.Getenv("ADMIN_PASSWORD")
		form.PasswordConfirm = os.Getenv("ADMIN_PASSWORD")
		err = form.Submit()
		if err != nil {
			log.Printf("Error creating admin: err=%s", err)
		}

		// Init OAuth provider
		var settings core.Settings
		err = json.Unmarshal([]byte(`{"meta":{"appName":"Acme","appUrl":"http://localhost:8090","senderName":"Support","senderAddress":"support@example.com","userVerificationUrl":"%APP_URL%/_/#/users/confirm-verification/%TOKEN%","userResetPasswordUrl":"%APP_URL%/_/#/users/confirm-password-reset/%TOKEN%","userConfirmEmailChangeUrl":"%APP_URL%/_/#/users/confirm-email-change/%TOKEN%"},"logs":{"maxDays":7},"smtp":{"enabled":false,"host":"smtp.example.com","port":587,"username":"","password":"","tls":false},"s3":{"enabled":false,"bucket":"","region":"","endpoint":"","accessKey":"","secret":""},"adminAuthToken":{"secret":"nYC34DS4IXtGG8d3HZIaLjgemQcg7v1yHxPCglZk0vEjw1VC9f","duration":1209600},"adminPasswordResetToken":{"secret":"av2hjrRXLWRZwYur29ByGB9kZblcTZ8n5tMBm0UvlBvAoxPRiq","duration":1800},"userAuthToken":{"secret":"5w1RwTkHuowGVGtAWZNzcDELr1kFieNMe7jEzBZK7UOkWJfc2y","duration":1209600},"userPasswordResetToken":{"secret":"esw3u4NiAz0vRWTKh3jDGWX4TiKfH5h6lR7DPFBqlDPxe8RQEa","duration":1800},"userEmailChangeToken":{"secret":"0OyEwCvEyIAZEnwqHz9diQ7sW9JetzPTu3BDotk0MEDTr7JoOm","duration":1800},"userVerificationToken":{"secret":"XJZH1VI2tHrc9oWucoOgfCxIodAyAE0Yf2GpJEbjmJhh7Hpjfo","duration":604800},"emailAuth":{"enabled":true,"exceptDomains":null,"onlyDomains":null,"minPasswordLength":8},"googleAuth":{"enabled":false,"allowRegistrations":true},"facebookAuth":{"enabled":false,"allowRegistrations":true},"githubAuth":{"enabled":false,"allowRegistrations":true},"gitlabAuth":{"enabled":false,"allowRegistrations":true},"stravaAuth":{"enabled":true,"allowRegistrations":true,"clientId":"0000","clientSecret":"000"}}`), &settings)
		if err != nil {
			log.Printf("Error unmarshal settings: err=%s", err)
		} else {
			settings.StravaAuth.Enabled = true
			settings.StravaAuth.ClientId = os.Getenv("CLIENT_ID")
			settings.StravaAuth.ClientSecret = os.Getenv("CLIENT_SECRET")
			j, err := json.Marshal(settings)
			if err != nil {
				log.Printf("Error marshal settings: err=%s", err)
			} else {
				res, err = e.App.DB().NewQuery("UPDATE `_params` SET `value`={:j} WHERE `key`='settings'").Bind(dbx.Params{"j": j}).Execute()
				if err != nil {
					log.Printf("Error update settings: err=%s", err)
				} else {
					log.Printf("Success update settings res=%s", res)
					e.App.RefreshSettings()
				}
			}
		}

		// Initialize cronjob
		c.AddFunc("*/1 * * * *", func() {

			nowTime := time.Now().UTC().Add(30 * time.Minute).Format(types.DefaultDateLayout)
			log.Printf("Starting cronjob: OAuthToken refresh with time=%s", nowTime)
			oAuthTokenCollection, err := app.Dao().FindCollectionByNameOrId(migrations.OAuthTokenCollectionName)
			if err != nil {
				log.Println(err)
				return
			}
			expr := dbx.NewExp("expiry<{:now}", dbx.Params{"now": nowTime})
			oAuthTokenRecords, err := app.Dao().FindRecordsByExpr(oAuthTokenCollection, expr)
			if err != nil {
				log.Println(err)
				return
			}

			updated := []string{}
			for _, t := range oAuthTokenRecords {
				log.Println("------")
				log.Printf("Updating record: %s", t.Id)
				var newToken string
				newToken, err = getToken(t.GetStringDataValue("refresh_token"))
				if err != nil {
					log.Printf("Error getting Strava token: err=%s", err)
					return
				}
				log.Printf("New Token: %s", newToken)
				var claims StravaClaims
				claims, err = getStravaClaims(newToken)
				if err != nil {
					log.Printf("Error getting Strava claim: err=%s", err)
					return
				}
				log.Printf("Strava claims: %+v", claims)
				oauth2Token := claims.getToken()
				log.Printf("Oauth2 Token: %+v", oauth2Token)
				userId := t.GetStringDataValue("userId")
				providerUserId := t.GetStringDataValue("provideruserid")
				rec, err := newOAuthTokenRecord(app.Dao(), t, oAuthTokenCollection, &userId, &providerUserId, oauth2Token)
				if err != nil {
					log.Printf("Error updating recors: rec.id=%s err=%s", rec.Id, err)
					continue
				} else {
					log.Printf("Updated record: %s", t.Id)
					updated = append(updated, rec.Id)
				}
				log.Println("------")
			}
			log.Printf("Updated OAuthToken records: %s", updated)
		})
		c.Start()
		log.Printf("%+v", c.Entries())
		// serves static files from the provided public dir (if exists)
		subFs := echo.MustSubFS(e.Router.Filesystem, "pb_public")
		e.Router.GET("/*", apis.StaticDirectoryHandler(subFs, false))

		e.Router.AddRoute(echo.Route{
			Method: http.MethodGet,
			// Middlewares: []echo.MiddlewareFunc{
			// 	apis.RequireAdminAuth(),
			// },
			Path: "/api/strava/map/:aid",
			Handler: func(c echo.Context) error {

				saId := c.PathParam("aid")
				log.Printf("saId: %+v", saId)
				if len(saId) == 0 {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("No activity id", map[string]string{
						"path":  "activityid",
						"error": "missing path parameter",
					}))
				}

				aId, err := strconv.ParseInt(saId, 10, 64)
				if err != nil {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("No valid activity id", map[string]string{
						"path":  "activity",
						"error": "missing valid path parameter",
					}))
				}

				activityCollection, err := app.Dao().FindCollectionByNameOrId(migrations.ActivityCollectionName)
				if err != nil {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("Missing collection", map[string]string{
						"collection": migrations.ActivityCollectionName,
						"error":      "missing collection",
					}))
				}

				expr := dbx.NewExp("activityid = {:activityid}", dbx.Params{"activityid": aId})
				activityRecords, err := app.Dao().FindRecordsByExpr(activityCollection, expr)
				if err != nil {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("Missing collection", map[string]string{
						"collection": migrations.ActivityCollectionName,
						"error":      "sql expression record",
					}))
				}

				if len(activityRecords) != 1 {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("Too few / many activity records", map[string]string{
						"collection": migrations.ActivityCollectionName,
						"error":      "activity records length not one",
					}))
				}

				activity := activityRecords[0]
				buf := []byte(activity.GetStringDataValue(migrations.ActivityMapPolyline))
				coords, _, err := polyline.DecodeCoords(buf)
				if err != nil {
					log.Println(err)
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError(err.Error(), nil))
				}

				// latLngs := s2.LatLng{}
				// latLngs := make([]s2.LatLng, len(coords))
				// for i, point := range coords {
				// 	latLngs[i] = s2.LatLng{
				// 		Lat:  coords[0],
				// 		Long: coords[1],
				// 	}
				// }

				sheight := c.QueryParam("height")
				swidth := c.QueryParam("width")

				height, err := strconv.ParseInt(sheight, 10, 64)
				if err != nil {
					height = 1024
				}
				if height == 0 {
					height = 1024
				}

				width, err := strconv.ParseInt(swidth, 10, 64)
				if err != nil {
					width = 720
				}
				if height == 0 {
					width = 720
				}

				ctx := staticmaps.NewContext()
				ctx.SetSize(int(width), int(height))
				// ctx.SetZoom(12)
				// ctx.AddObject(
				// 	// sm.NewMarker(
				// 	// 	s2.LatLngFromDegrees(52.514536, 13.350151),
				// 	// 	color.RGBA{0xff, 0, 0, 0xff},
				// 	// 	16.0,
				// 	// ),
				// 	sm.NewPath(coords, color.RGBA{0xff, 0, 0, 0xff},
				// 		16.0),
				// )

				p := new(staticmaps.Path)
				p.Color = color.RGBA{0xff, 0, 0, 0xff}
				p.Weight = 6.0
				for _, trk := range coords {
					p.Positions = append(p.Positions, s2.LatLngFromDegrees(trk[0], trk[1]))
				}
				ctx.AddObject(p)
				ctx.SetBoundingBox(p.Bounds())

				img, err := ctx.Render()
				if err != nil {
					log.Println(err)
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError(err.Error(), nil))
				}

				buffer := new(bytes.Buffer)
				if err := png.Encode(buffer, img); err != nil {
					log.Println("unable to encode image.")
				}

				w := c.Response().Writer
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Content-Length", strconv.Itoa(len(buffer.Bytes())))
				if _, err := w.Write(buffer.Bytes()); err != nil {
					log.Println("unable to write image.")
				}
				return nil
				// return
				// // w.Write(img)

				// if err := gg.SavePNG("my-map.png", img); err != nil {
				// 	log.Println(err)
				// 	return c.JSON(http.StatusBadRequest, rest.NewBadRequestError(err.Error(), nil))
				// }

				// return c.JSON(200, activity)
			},
		})

		// "POST /api/strava/activities"
		// Request Query Params: after(int64), before(int64)
		e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/strava/activities",
			Handler: func(c echo.Context) error {
				user, ok := c.Get(ContextUserKey).(*models.User)
				if !ok {
					return c.JSON(http.StatusUnauthorized, rest.NewForbiddenError("no auth user", nil))
				}

				// Unmarshal
				var msg StravaActivitiesMsg
				afters := c.QueryParam("after")
				if len(afters) > 0 {
					log.Printf("Found query param after: %s", afters)
					afteri, err := strconv.ParseInt(afters, 10, 64)
					if err == nil {
						log.Printf("Found query param after (int64): %d", afteri)
						msg.After = afteri
					}
				}
				before := c.QueryParam("before")
				if len(before) > 0 {
					log.Printf("Found query param before: %s", before)
					beforei, err := strconv.ParseInt(before, 10, 64)
					if err == nil {
						log.Printf("Found query param after (int64): %d", beforei)
						msg.Before = beforei
					}
				}

				oAuthTokenCollection, err := app.Dao().FindCollectionByNameOrId(migrations.OAuthTokenCollectionName)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, rest.NewNotFoundError("collection not found", nil))
				}
				expr := dbx.HashExp{"provider": "strava", "userId": user.Id}
				oAuthTokenRecords, err := app.Dao().FindRecordsByExpr(oAuthTokenCollection, expr)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, rest.NewBadRequestError("no token", nil))
				}
				if len(oAuthTokenRecords) == 0 {
					return c.JSON(http.StatusUnauthorized, rest.NewBadRequestError("no token", nil))
				}
				if len(oAuthTokenRecords) > 1 {
					return c.JSON(http.StatusUnauthorized, rest.NewBadRequestError("more tokens than expected", nil))
				}
				oAuthTokenRecord := oAuthTokenRecords[0]
				accessToken, ok := oAuthTokenRecord.GetDataValue(migrations.OAuthTokenCollectionAccessToken).(string)
				if !ok {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("access_token not type valid", nil))
				}
				log.Printf("AccessToken: %s", accessToken)

				providerUserIdTmp, ok := oAuthTokenRecord.GetDataValue((migrations.OAuthTokenCollectionProviderUserId)).(string)
				if !ok {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("provider user id not type valid", nil))
				}
				providerUserId, err := strconv.ParseInt(providerUserIdTmp, 10, 64)
				if err != nil {
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("provider user id not type valid for request", nil))
				}

				client := strava.NewClient(accessToken)
				services := strava.NewAthletesService(client).ListActivities(providerUserId)
				if msg.Before != 0 {
					services.Before(msg.Before)
				}
				if msg.After != 0 {
					services.After(msg.After)
				}
				activities, err := services.Do()
				if err != nil {
					log.Printf("err=%s", err)
					return c.JSON(http.StatusUnauthorized, err)
				}

				createActivities(app, user.Id, activities)
				return c.JSON(200, activities)
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireAdminOrUserAuth(),
			},
		})

		return nil
	})

	app.OnSettingsBeforeUpdateRequest().Add(func(data *core.SettingsUpdateEvent) error {
		log.Printf("Settings data: %+v", data.NewSettings.StravaAuth)
		return nil
	})

	// app.OnUserAuthRequest().Add(func(e *core.UserAuthEvent) error {
	// 	// log.Printf("Login Meta: %+v", reflect.TypeOf(e.Meta))
	// 	meta, ok := e.Meta.(*auth.AuthUser)
	// 	if !ok {
	// 		log.Println("--- 1")
	// 		return nil
	// 	}
	// 	log.Printf("OAuth2Token found: %+v", meta.Token)
	// 	return nil
	// })

	// Event Hooks

	// app.OnUserAfterOauth2Register().Add(func(e *core.UserOauth2RegisterEvent) error {

	// 	dao := app.Dao()
	// 	profileCollection, err := dao.FindCollectionByNameOrId(models.ProfileCollectionName)
	// 	if err != nil{
	// 		log.Printf("Event hook -- OnUserAfterOauth2Register: No profile collection error=%s", err)
	// 		return nil
	// 	}

	// 	userProfile, err := dao.FindFirstRecordByData(
	// 		profileCollection,
	// 		models.ProfileCollectionUserFieldName,
	// 		e.AuthData.Id,
	// 	)
	// 	if err != nil{
	// 		log.Printf("Event hook -- OnUserAfterOauth2Register: Error find first record by data error=%s", errr)
	// 		return nil
	// 	}

	// 	userProfile.SetDataValue()
	// 	return nil

	// })

	app.OnUserAuthRequest().Add(func(e *core.UserAuthEvent) error {

		meta, ok := e.Meta.(*auth.AuthUser)
		if !ok {
			log.Printf("Event hook -- OnAuthRequest: No AuthUser in meta")
			return nil
		}

		oAuthTokenCollection, err := app.Dao().FindCollectionByNameOrId(migrations.OAuthTokenCollectionName)
		if err != nil {
			return err
		}
		expr := dbx.HashExp{"provider": "strava", "userId": e.User.Id}
		oAuthTokenRecords, err := app.Dao().FindRecordsByExpr(oAuthTokenCollection, expr)
		if err != nil {
			return err
		}
		log.Printf("Found OAuthTokenRecords: %+v", oAuthTokenRecords)
		// Create record "token"
		if len(oAuthTokenRecords) == 0 {
			_, err = newOAuthTokenRecord(app.Dao(), nil, oAuthTokenCollection, &e.User.Id, &meta.Id, &meta.Token)
		} else if len(oAuthTokenRecords) == 1 {
			// Update record
			_, err = newOAuthTokenRecord(app.Dao(), oAuthTokenRecords[0], nil, &e.User.Id, &meta.Id, &meta.Token)
		} else {
			// Problem: More than one record for the user/provider
			err = errors.New("more than one record for user/provider")
		}
		if err != nil {
			return err
		}

		// Request Strava activities for each AuthRequest
		stravaUserId, err := strconv.ParseInt(meta.Id, 10, 64)
		if err != nil {
			return err
		}
		var msg StravaActivitiesMsg
		client := strava.NewClient(meta.Token.AccessToken)
		services := strava.NewAthletesService(client).ListActivities(stravaUserId)
		if msg.Before != 0 {
			services.Before(msg.Before)
		}
		if msg.After != 0 {
			services.After(msg.After)
		}
		activities, err := services.Do()
		if err != nil {
			log.Printf("err=%s", err)
			return err
		}

		createActivities(app, e.User.Id, activities)

		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

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

func newOAuthTokenRecord(dao *daos.Dao, rec *models.Record, collection *models.Collection, userId *string, providerUserId *string, token *oauth2.Token) (*models.Record, error) {
	log.Printf("New OauthTokenRecord: userId=%s providerUserId=%s token=%+v", *userId, *providerUserId, token)
	if rec == nil {
		rec = models.NewRecord(collection)
	}
	provider := "strava"
	rec.SetDataValue(migrations.OAuthTokenCollectionNameProvider, provider)
	rec.SetDataValue(migrations.ProfileCollectionUserId, userId)
	rec.SetDataValue(migrations.OAuthTokenCollectionProviderUserId, providerUserId)
	rec.SetDataValue(migrations.OAuthTokenCollectionAccessToken, token.AccessToken)
	rec.SetDataValue(migrations.OAuthTokenCollectionRefreshToken, token.RefreshToken)
	rec.SetDataValue(migrations.OAuthTokenCollectionTokenType, token.TokenType)
	rec.SetDataValue(migrations.OAuthTokenCollectionExpiry, token.Expiry)
	err := dao.SaveRecord(rec)
	data := OAuthToken{
		Provider:       provider,
		UserId:         *userId,
		ProviderUserId: *providerUserId,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		TokenType:      token.TokenType,
		Expiry:         token.Expiry,
	}
	bdata, berr := json.Marshal(data)
	if berr == nil {
		kp.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &kptopic, Partition: kafka.PartitionAny},
			Key:            []byte(*providerUserId),
			Value:          bdata,
		}, nil)
	}
	return rec, err
}

type OAuthToken struct {
	Provider       string    `json,db:"provider"`
	UserId         string    `json,db:"userId"`
	ProviderUserId string    `json,db:"provideruserid"`
	AccessToken    string    `json,db:"access_token"`
	RefreshToken   string    `json,db:"refresh_token"`
	TokenType      string    `json,db:"token_type"`
	Expiry         time.Time `json,db:"expiry"`
}
