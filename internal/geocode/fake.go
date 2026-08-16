package geocode

import "crypto/sha256"

type Fake struct{}

func (Fake) Lookup(address string) (Point, error) {
	sum := sha256.Sum256([]byte(address))
	return Point{
		Lat: 37.5 + (float64(sum[0])/255-0.5)*0.12,
		Lng: -122.45 + (float64(sum[1])/255-0.5)*0.12,
	}, nil
}
