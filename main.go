package main

import (
	"bytes"
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
	strava "github.com/strava/go.strava"
	"github.com/twpayne/go-polyline"

	staticmaps "github.com/flopp/go-staticmaps"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/mbecker/pocketstrava/db"
	"github.com/mbecker/pocketstrava/handler"
	internalmails "github.com/mbecker/pocketstrava/mails"
	"github.com/mbecker/pocketstrava/migrations"
	internalmodels "github.com/mbecker/pocketstrava/models"
	internalsettings "github.com/mbecker/pocketstrava/settings"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/pocketbase/pocketbase/tools/rest"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/robfig/cron/v3"
)

const ContextUserKey string = "user"

var ErrNoUser = errors.New("no user")

type StravaActivitiesMsg struct {
	Before int64 `json:"before"`
	After  int64 `json:"after"`
}

var KP *kafka.Producer
var KPTOPIC string

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	// Initialize app, cronjob, kafka
	app := pocketbase.New()

	c := cron.New()

	KPTOPIC = os.Getenv("KAFKA_TOPIC")
	// Produce a new record to the topic...
	KP, err = kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": os.Getenv("KAFKA_BOOTSTRAP_SERVER"),
		"sasl.mechanisms":   "PLAIN",
		"security.protocol": "SASL_PLAINTEXT",
		"sasl.username":     os.Getenv("KAFKA_USERNAME"),
		"sasl.password":     os.Getenv("KAFKA_PASSWORD")})
	if err != nil {
		log.Panic(err)
	} else {
		log.Println("Kafka Connect success")
	}

	defer KP.Close()

	// Delivery report handler for produced messages
	go func() {
		for e := range KP.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("Kafka Producer Error - Delivery failed: key=%s value=%s\n", ev.Key, string(ev.Value))
				} else {
					log.Printf("Kafka Producer Success - Delivery success: key=%s value=%s\n", ev.Key, string(ev.Value))
				}
			}
		}
	}()

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		/**
		* SETTINGS
		 */
		err := internalsettings.SetAppSettings(e)
		if err != nil {
			log.Panicf("(ERROR) Updating settings: err=%s", err)
		}

		/**
		* CRONJOB
		 */
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
				go db.CreateOAuthTokenRecord(app, &userId, &providerUserId, oauth2Token, KP, &KPTOPIC)

				log.Println("------")
			}
			log.Printf("Updated OAuthToken records: %s", updated)
		})
		c.Start()
		log.Printf("%+v", c.Entries())

		/**
		* Route: public
		* /public
		 */
		subFs := echo.MustSubFS(e.Router.Filesystem, "pb_public")
		e.Router.GET("/public/*", apis.StaticDirectoryHandler(subFs, false))

		/**
		 * Route: MAP
		 * /api/strava/map/:aid
		 */

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

		/**
		 * EMAIL Confirmation
		 * /api/email/confirmation
		 * /api/email/email
		 */

		// POST /api/email/confirmation
		e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/email/confirmation",
			Handler: func(c echo.Context) error {
				cErr := handler.CustomError{
					Code:        http.StatusOK,
					Level:       handler.INFO,
					Message:     "Email confirmed",
					ErrorDetail: nil,
				}

				// u, ok := c.Get(ContextUserKey).(*models.User)
				// if !ok {
				// 	return c.JSON(http.StatusUnauthorized, rest.NewForbiddenError("no auth user", nil))
				// }
				token := new(internalmodels.Token)
				err := c.Bind(token)
				if err != nil {
					cErr.Code = http.StatusBadRequest
					cErr.Level = handler.ERROR
					cErr.Message = "Error binding token request"
					cErr.ErrorDetail = err
					return c.JSON(cErr.Code, cErr)
				}
				rec, err := handler.ParseNotificationVerificationToken(app, token.Token)
				if err != nil {
					log.Println(err)
					cErr.Code = http.StatusBadRequest
					cErr.Level = handler.ERROR
					cErr.Message = "Error parsing token"
					cErr.ErrorDetail = err
					return c.JSON(cErr.Code, cErr)
				}

				err = internalmodels.EmailSetValid(app, rec)
				if err != nil {
					log.Println(err)
					cErr.Code = http.StatusBadRequest
					cErr.Level = handler.ERROR
					cErr.Message = "Error saving email confirmation"
					cErr.ErrorDetail = err
					return c.JSON(cErr.Code, cErr)
				}

				cErr.Message = fmt.Sprintf("Email confirmed: %s", rec.GetStringDataValue(migrations.NotificationsEmail))
				return c.JSON(cErr.Code, cErr)
			}, Middlewares: []echo.MiddlewareFunc{
				apis.RequireAdminOrUserAuth(),
			}})

		// "POST /api/email/email"
		e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/email/email",
			Handler: func(c echo.Context) error {
				cErr := handler.CustomError{
					Code:        http.StatusOK,
					Level:       handler.INFO,
					Message:     "Mail sent",
					ErrorDetail: nil,
				}

				u, ok := c.Get(ContextUserKey).(*models.User)
				if !ok {
					return c.JSON(http.StatusUnauthorized, rest.NewForbiddenError("no auth user", nil))
				}
				email := new(internalmodels.Email)
				// if err := c.(*handler.Context).BindValidate(email); err != nil {
				// 	log.Println(err)
				// 	if cErr, ok := err.(*handler.CustomError); ok {
				// 		return c.JSON(cErr.Code, cErr)
				// 	}
				// 	return c.JSON(http.StatusBadRequest, err)
				// }

				err := c.Bind(&email)
				if err != nil {
					cErr.Code = http.StatusBadRequest
					cErr.Level = handler.ERROR
					cErr.Message = "Error decoding body"
					cErr.ErrorDetail = err
					return c.JSON(cErr.Code, cErr)
				}

				// Insert and check database
				emailCollection, err := app.Dao().FindCollectionByNameOrId(migrations.NotificationsCollectionName)
				if err != nil {
					cErr.Code = http.StatusBadRequest
					cErr.Level = handler.ERROR
					cErr.Message = "Email collection not found"
					cErr.ErrorDetail = err
					return c.JSON(cErr.Code, cErr)
				}

				// If "email.retrigger" then check that the email is already in DB and that it's not valid
				var expr dbx.HashExp
				if email.Retrigger {
					expr = dbx.HashExp{"email": email.Email, models.ProfileCollectionUserFieldName: u.Id, migrations.NotificationsValid: false}
				} else {
					expr = dbx.HashExp{"email": email.Email, models.ProfileCollectionUserFieldName: u.Id}
				}

				emailRecords, err := app.Dao().FindRecordsByExpr(emailCollection, expr)
				if err != nil {
					cErr.Code = http.StatusBadRequest
					cErr.Level = handler.ERROR
					cErr.Message = "Email records query validation error"
					cErr.ErrorDetail = err
					return c.JSON(cErr.Code, cErr)
				}

				// If "email.retrigger" the expect result is exatcly len=1
				if email.Retrigger {
					if len(emailRecords) != 1 {
						cErr.Code = http.StatusBadRequest
						cErr.Level = handler.ERROR
						cErr.Message = "Email retrigger and not exactly one email"
						cErr.ErrorDetail = err
						return c.JSON(cErr.Code, cErr)
					}
				} else {
					if len(emailRecords) != 0 {
						cErr.Code = http.StatusBadRequest
						cErr.Level = handler.ERROR
						cErr.Message = "No unique email for user"
						cErr.ErrorDetail = err
						return c.JSON(cErr.Code, cErr)
					}

					rec := models.NewRecord(emailCollection)
					rec.SetDataValue(models.ProfileCollectionUserFieldName, u.Id)
					rec.SetDataValue(migrations.NotificationsEmail, email.Email)
					rec.SetDataValue(migrations.NotificationsValid, false)
					rec.SetDataValue(migrations.NotificationsPrimary, false)

					err = forms.NewRecordUpsert(app, rec).Submit()
					if err != nil {
						cErr.Code = http.StatusBadRequest
						cErr.Level = handler.ERROR
						cErr.Message = "Error saving record"
						cErr.ErrorDetail = err
						return c.JSON(cErr.Code, cErr)
					}
				}

				// Send the notification verification email
				err = internalmails.SendNotificationVerificationEmail(app, u.Id, email.Email)

				if err != nil {
					cErr = handler.CustomError{
						Code:        http.StatusBadRequest,
						Level:       handler.ERROR,
						Message:     "Error sending mail",
						ErrorDetail: err,
					}
				}
				log.Printf("Send mail for notification verification email: userId=%s email=%s error=%s", u.Id, email.Email, err)

				// TODO: Respond with the email record (see how the collection response work)
				return c.JSON(cErr.Code, cErr)
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireAdminOrUserAuth(),
			},
		})

		return nil
	})

	app.OnSettingsBeforeUpdateRequest().Add(func(data *core.SettingsUpdateEvent) error {
		return nil
	})

	/**
	* User Auth Request
	* Create / update strava.com ouath token in DB
	* Request activities at strava.com and save in DB
	 */

	app.OnUserAuthRequest().Add(func(e *core.UserAuthEvent) error {

		logStartHook("OnUserAuthRequest")

		// Create map template
		go db.CreateDefaultMapTemplate(app, &e.User.Id)

		// Update strava oauth access token

		meta, ok := e.Meta.(*auth.AuthUser)
		if !ok {
			log.Printf("Event hook -- OnAuthRequest: No AuthUser in meta")
			return nil
		}

		go db.CreateOAuthTokenRecord(app, &e.User.Id, &meta.Id, meta.Token, KP, &KPTOPIC)

		// Request Strava activities for each AuthRequest
		stravaUserId, err := strconv.ParseInt(meta.Id, 10, 64)
		if err != nil {
			log.Printf("Error parsing strava athlete id: %s", err)
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
			log.Printf("error requesting strava activities: %s", err)
			return err
		}

		go db.CreateActivities(app, e.User.Id, activities)

		logEndHook("OnUserAuthRequest")
		return nil
	})

	app.OnCollectionViewRequest().Add(func(e *core.CollectionViewEvent) error {
		log.Printf("Collection view hook: %+v", e.Collection.Name)
		return nil
	})

	app.OnCollectionsListRequest().Add(func(e *core.CollectionsListEvent) error {
		log.Printf("Collection list hook: %+v", e.Collections)
		return nil
	})

	app.OnRecordsListRequest().Add(func(e *core.RecordsListEvent) error {
		logStartHook("OnRecordsListRequest")
		log.Printf("Collection name: %s", e.Collection.Name)
		for _, r := range e.Records {
			log.Printf("Record: %+v", r.ColumnValueMap())
		}
		logEndHook("OnRecordsListRequest")
		return nil
	})

	app.OnRecordViewRequest().Add(func(e *core.RecordViewEvent) error {
		return nil
	})

	/**
	* App Start
	 */
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func logStartHook(hook string) {
	log.Printf("=== START Hook ::: %s ===", hook)
}

func logEndHook(hook string) {
	log.Printf("=== END Hook ::: %s ===", hook)
}
