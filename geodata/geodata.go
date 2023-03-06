package geodata

import (
	"fmt"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

type GeoData struct {
	Country struct {
		IsoCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
		Latitude       float64 `maxminddb:"latitude"`
		Longitude      float64 `maxminddb:"longitude"`
		MetroCode      uint    `maxminddb:"metro_code"`
		TimeZone       string  `maxminddb:"time_zone"`
	} `maxminddb:"location"`
}

type GeoIP2DB struct {
	db *maxminddb.Reader
}

func NewGeoIP2DB(databaseFilePath string) (*GeoIP2DB, error) {
	db, err := maxminddb.Open(databaseFilePath)
	if err != nil {
		return nil, err
	}
	return &GeoIP2DB{db}, nil
}

func (g *GeoIP2DB) Close() error {
	return g.db.Close()
}

func (g *GeoIP2DB) GetGeoDataFromIPAddress(ipAddress string) (*GeoData, error) {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipAddress)
	}

	var geoData GeoData
	err := g.db.Lookup(ip, &geoData)
	if err != nil {
		return nil, err
	}

	return &geoData, nil
}
