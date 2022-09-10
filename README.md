## Create Strava activities

```go
/**
* STRAVA ACTIVITIES
*/

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
```