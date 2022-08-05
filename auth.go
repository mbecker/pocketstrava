package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"golang.org/x/oauth2"
)

const (
	REFRESH   = "refresh_token"
	DELTASECS = 5
)

type StravaClaims struct {
	jwt.StandardClaims
	TokenType    string `json:"token_type"`
	ExpiresAt    int64  `json:"expires_at"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	// StravaUser   StravaUser `json:"athlete"`
}

func (s *StravaClaims) getToken() *oauth2.Token {
	x := oauth2.Token{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		TokenType:    s.TokenType,
		Expiry:       time.Unix(s.ExpiresAt, 0),
	}
	return &x
}

func ParseToken(rawToken []byte) (*oauth2.Token, error) {
	tok := &oauth2.Token{}
	err := json.Unmarshal(rawToken, tok)
	if err != nil {
		return tok, err
	}
	msi := map[string]interface{}{}
	err = json.Unmarshal(rawToken, &msi)
	if err != nil {
		return tok, err
	}
	return tok.WithExtra(msi), nil
}

func getStravaClaims(token string) (StravaClaims, error) {

	// tkt, _ := jwt.ParseWithClaims(token, &StravaClaims{}, nil)
	// fmt.Printf("Strava claims: %#v\n", tkt)

	// if claims, ok := tkt.Claims.(*StravaClaims); ok {
	// 	return *claims, nil
	// } else {
	// 	return *claims, errors.New("Token not valid")
	// }

	claims := StravaClaims{}
	json.Unmarshal([]byte(token), &claims)

	// var claims StravaClaims
	// jwt.ParseWithClaims(token, &claims, nil)

	// if len(claims.StravaUser.Username) == 0 {
	// 	return claims, errors.New("Token not valid")
	// }
	return claims, nil
}

//wrapper to set accept header
func jsonPost(url string, body io.Reader) (resp *http.Response, err error) {
	var client = &http.Client{
		Timeout: time.Second * 10,
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return client.Do(req)
}

//get a token from authorization endpoint
func getToken(code string) (result string, err error) {
	rParams := map[string]string{
		"client_id":     os.Getenv("CLIENT_ID"),
		"client_secret": os.Getenv("CLIENT_SECRET"),
		"refresh_token": code,
		"grant_type":    REFRESH,
	}

	var resp *http.Response
	var requestBody []byte
	requestBody, err = json.Marshal(rParams)
	if err != nil {
		return
	}
	resp, err = jsonPost(os.Getenv("TOKEN_EDNPOINT"), bytes.NewBuffer(requestBody))
	if err != nil {
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 {
		err = errors.New(string(body))
		return
	}
	//check for expires_at
	var tokMap map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	err = decoder.Decode(&tokMap)
	if err != nil {
		err = errors.New("decoder.Decode: " + err.Error())
		return
	}
	expire, exists := tokMap["expires_at"]

	if exists {
		result = string(body)
		return
	}
	var expiresIn int64
	expire, exists = tokMap["expires_in"]
	if !exists { //no expiration, so make it a year
		expiresIn = 31536000
	} else {
		expiresIn, err = expire.(json.Number).Int64()
	}
	tokMap["expires_at"] = epochSeconds() + expiresIn - DELTASECS
	b, err := json.Marshal(tokMap)
	if err != nil {
		err = errors.New("json.Marshal: " + err.Error())
		return
	}
	result = string(b)
	return
}

func epochSeconds() int64 {
	now := time.Now()
	secs := now.Unix()
	return secs
}
