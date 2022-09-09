package models

type Email struct {
	Email     string `json:"email" validate:"required,email"`
	Retrigger bool   `json:"retrigger"`
}

type Token struct {
	Token string `json:"token" validate:"required,token"`
}
