package backup

import (
	"time"

	"github.com/luuuunet/owpanel/internal/scheduleutil"
)

func CronDueNow(schedule string, lastRun *time.Time, now time.Time) bool {
	return scheduleutil.DueNow(schedule, lastRun, now)
}

func cronDueNow(schedule string, lastRun *time.Time, now time.Time) bool {
	return scheduleutil.DueNow(schedule, lastRun, now)
}
