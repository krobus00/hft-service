package entity

import "time"

type KlineSubscription struct {
	ID        string    `db:"id" json:"id"`
	Exchange  string    `db:"exchange" json:"exchange"`
	Symbol    string    `db:"symbol" json:"symbol"`
	Interval  string    `db:"interval" json:"interval"`
	Payload   string    `db:"payload" json:"payload"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
