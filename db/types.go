package db

import (
	"time"
)

type NodeData struct {
	UpdatedAt   time.Time `json:"updated_at"`
	IPAddress   string    `json:"-"`
	GeoData     GeoData   `json:"geo_data"`
	NodeVersion string    `json:"node_version"`
	OperatorID  string    `json:"operator_id"`
}

type GeoData struct {
	CountryCode    string  `json:"country_code"`
	CountryName    string  `json:"country_name"`
	City           string  `json:"city"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	AccuracyRadius uint16  `json:"accuracy_radius"`
}
