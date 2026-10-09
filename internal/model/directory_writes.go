package model

import (
	"strconv"

	"heliosian/internal/access"
	"heliosian/internal/geocode"
	"heliosian/internal/store"
)

func validTagName(tag string) bool {
	return tag != "" && len(tag) <= maxTagLength
}

func (m *Directory) locate(actor access.Actor, found map[string]geocode.Point) []store.Op {
	ops := []store.Op{}
	for _, address := range m.unlocated {
		point, ok := found[address]
		if !ok {
			continue
		}
		ops = append(ops, store.Insert(geocodeTable, store.Row{
			geocodeAddress: address,
			geocodeLat:     strconv.FormatFloat(point.Lat, 'f', -1, 64),
			geocodeLng:     strconv.FormatFloat(point.Lng, 'f', -1, 64),
		}))
	}
	return ops
}
