package db

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/mbecker/pocketstrava/migrations"
	internalmodels "github.com/mbecker/pocketstrava/models"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
	"golang.org/x/oauth2"
)

func CreateDefaultMapTemplate(app core.App, userId *string) {
	var defaultMaptemplate map[string]interface{}
	err := json.Unmarshal([]byte(`{"version":"5.1.0","objects":[{"type":"circle","version":"5.1.0","originX":"left","originY":"top","left":150,"top":150,"width":100,"height":100,"fill":"#0B61A4","stroke":null,"strokeWidth":1,"strokeDashArray":null,"strokeLineCap":"butt","strokeDashOffset":0,"strokeLineJoin":"miter","strokeUniform":false,"strokeMiterLimit":4,"scaleX":1,"scaleY":1,"angle":0,"flipX":false,"flipY":false,"opacity":1,"shadow":null,"visible":true,"backgroundColor":"","fillRule":"nonzero","paintFirst":"fill","globalCompositeOperation":"source-over","skewX":0,"skewY":0,"radius":50,"startAngle":0,"endAngle":360}]}`), &defaultMaptemplate)
	if err != nil {
		log.Printf("Error unmarshalling default map template: %s", err)
		return
	}
	mapTemplatesModel, err := app.Dao().FindCollectionByNameOrId(migrations.MapTemplatesCollectionName)
	if err != nil {
		log.Printf("Error finding collection `%s`: %s", migrations.MapTemplatesCollectionName, err)
		return
	}
	log.Printf("Creating map template for user: %s", *userId)
	rec := models.NewRecord(mapTemplatesModel)
	rec.SetDataValue(models.ProfileCollectionUserFieldName, userId)
	rec.SetDataValue(migrations.MapTemplatesPrimary, true)
	rec.SetDataValue(migrations.MapTemplatesJson, defaultMaptemplate)
	rec.SetDataValue(migrations.MapTemplatesUrl, fmt.Sprintf("%s/public/maps/default.png", app.Settings().Meta.AppUrl))

	formsRecordUpsert := forms.NewRecordUpsert(app, rec)
	err = formsRecordUpsert.Submit()
	if err != nil {
		log.Printf("Error saving default map template for new user: %s", err)
	}
}

func CreateOAuthTokenRecord(app core.App, userId *string, providerUserId *string, token *oauth2.Token, kafkaProducer *kafka.Producer, kafkaTopic *string) error {
	provider := "strava"
	oauthtokensCollection, err := app.Dao().FindCollectionByNameOrId(migrations.OAuthTokenCollectionName)
	if err != nil {
		return err

	}
	// expr := dbx.HashExp{"provider": "strava", "userId": e.User.Id}
	// oAuthTokenRecords, err := app.Dao().FindRecordsByExpr(oauthtokensCollection, expr)
	// if err != nil {
	// 	return err
	// }

	// Create record "token"
	// if len(oAuthTokenRecords) == 0 {
	// 	_, err = newOAuthTokenRecord(app.Dao(), nil, stravaModel, &e.User.Id, &meta.Id, meta.Token)
	// } else if len(oAuthTokenRecords) == 1 {
	// 	// Update record
	// 	_, err = newOAuthTokenRecord(app.Dao(), oAuthTokenRecords[0], nil, &e.User.Id, &meta.Id, meta.Token)
	// } else {
	// 	// Problem: More than one record for the user/provider
	// 	err = errors.New("more than one record for user/provider")
	// }
	// if err != nil {
	// 	log.Printf("Error creating / updating oauthtoken record: %s", err)
	// 	return err
	// }

	// rec := models.NewRecord(oauthtokensCollection)

	rec, err := app.Dao().FindFirstRecordByData(oauthtokensCollection, "userId", userId)
	if err != nil {
		log.Printf("Error finding first record by data: %s", err)
		rec = models.NewRecord(oauthtokensCollection)
	}

	rec.SetDataValue(migrations.OAuthTokenCollectionNameProvider, provider)
	rec.SetDataValue(migrations.ProfileCollectionUserId, userId)
	rec.SetDataValue(migrations.OAuthTokenCollectionProviderUserId, providerUserId)
	rec.SetDataValue(migrations.OAuthTokenCollectionAccessToken, token.AccessToken)
	rec.SetDataValue(migrations.OAuthTokenCollectionRefreshToken, token.RefreshToken)
	rec.SetDataValue(migrations.OAuthTokenCollectionTokenType, token.TokenType)
	rec.SetDataValue(migrations.OAuthTokenCollectionExpiry, token.Expiry)
	err = app.Dao().Save(rec)
	if err != nil {
		log.Printf("Error forms new record upset for oauthtokensCollection: %s", err)
		return err
	}
	log.Printf("Success saving new / updated oauth token for provider: %s", provider)
	data := internalmodels.OAuthToken{
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
		kafkaProducer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: kafkaTopic, Partition: kafka.PartitionAny},
			Key:            []byte(*providerUserId),
			Value:          bdata,
		}, nil)
	}
	return nil
}
