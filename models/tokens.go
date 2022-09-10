package models

import "time"

type OAuthToken struct {
	Provider       string    `json,db:"provider"`
	UserId         string    `json,db:"userId"`
	ProviderUserId string    `json,db:"provideruserid"`
	AccessToken    string    `json,db:"access_token"`
	RefreshToken   string    `json,db:"refresh_token"`
	TokenType      string    `json,db:"token_type"`
	Expiry         time.Time `json,db:"expiry"`
}
