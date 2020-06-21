// +build !confonly

package router

import (
	"github.com/oschwald/maxminddb-golang"

	"v2ray.com/core/common/net"
	"v2ray.com/core/common/platform"
)

var geoipDB *maxminddb.Reader

func lookupGeoIP(ip net.IP) (string, error) {
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}

	if geoipDB == nil {
		var err error
		geoipDB, err = maxminddb.Open(platform.GetAssetLocation("geo.mmdb"))
		if err != nil {
			return "", err
		}
	}

	err := geoipDB.Lookup(ip, &record)
	if err != nil {
		return "", err
	}

	return record.Country.ISOCode, nil
}
