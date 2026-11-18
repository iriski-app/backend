package service

import "time"

func marketDateCutoff(now time.Time) time.Time {
	moscow := now.In(time.FixedZone("MSK", 3*60*60))
	year, month, day := moscow.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
}
