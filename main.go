package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
	"github.com/mbecker/pocketstrava/migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/pocketbase/pocketbase/tools/rest"
	strava "github.com/strava/go.strava"
	"golang.org/x/oauth2"
)

const ContextUserKey string = "user"

func main() {
	app := pocketbase.New()

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {

		// serves static files from the provided public dir (if exists)
		subFs := echo.MustSubFS(e.Router.Filesystem, "pb_public")
		e.Router.GET("/*", apis.StaticDirectoryHandler(subFs, false))

		// add new "GET /api/hello" route to the app router (echo)
		e.Router.AddRoute(echo.Route{
			Method: http.MethodGet,
			Path:   "/api/strava/activities",
			Handler: func(c echo.Context) error {
				user, ok := c.Get(ContextUserKey).(*models.User)
				if !ok {
					return c.JSON(http.StatusUnauthorized, rest.NewForbiddenError("no auth user", nil))
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
				activities, err := services.Do()
				if err != nil {
					log.Printf("err=%s", err)
					return c.JSON(http.StatusBadRequest, rest.NewBadRequestError("strava request failed / error", nil))
				}

				file, _ := json.MarshalIndent(activities, "", " ")

				_ = ioutil.WriteFile("activities.json", file, 0644)
				return c.JSON(200, activities)
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireAdminOrUserAuth(),
			},
		})

		return nil
	})

	app.OnUserAuthRequest().Add(func(e *core.UserAuthEvent) error {
		// log.Printf("Login Meta: %+v", reflect.TypeOf(e.Meta))
		meta, ok := e.Meta.(*auth.AuthUser)
		if !ok {
			log.Println("--- 1")
			return nil
		}
		log.Printf("OAuth2Token found: %+v", meta.Token)
		return nil
	})

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
		var rec *models.Record
		if len(oAuthTokenRecords) == 0 {
			rec = newOAuthTokenRecord(nil, oAuthTokenCollection, &e.User.Id, &meta.Id, &meta.Token)
		} else if len(oAuthTokenRecords) == 1 {
			// Update record
			rec = newOAuthTokenRecord(oAuthTokenRecords[0], nil, &e.User.Id, &meta.Id, &meta.Token)
		} else {
			// Problem: More than one record for the user/provider
			return errors.New("more than one record for user/provider")
		}
		err = app.Dao().SaveRecord(rec)
		if err != nil {
			return err
		}
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func newOAuthTokenRecord(rec *models.Record, collection *models.Collection, userId *string, providerUserId *string, token *oauth2.Token) *models.Record {
	log.Printf("New OauthTokenRecord: userId=%s providerUserId=%s token=%+v", *userId, *providerUserId, token)
	if rec == nil {
		rec = models.NewRecord(collection)
	}
	rec.SetDataValue(migrations.OAuthTokenCollectionNameProvider, "strava")
	rec.SetDataValue(migrations.ProfileCollectionUserId, userId)
	rec.SetDataValue(migrations.OAuthTokenCollectionProviderUserId, providerUserId)
	rec.SetDataValue(migrations.OAuthTokenCollectionAccessToken, token.AccessToken)
	rec.SetDataValue(migrations.OAuthTokenCollectionRefreshToken, token.RefreshToken)
	rec.SetDataValue(migrations.OAuthTokenCollectionTokenType, token.TokenType)
	rec.SetDataValue(migrations.OAuthTokenCollectionExpiry, token.Expiry)
	return rec
}
