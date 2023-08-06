// Copyright (C) 2022, Chain4Travel AG. All rights reserved.
//
// This file contains helper functions

package cfg

import (
	"time"
)

func GetDatepartBasedOnDateParams(pStartTime time.Time, pEndTime time.Time) string {
	differenceInDays := int64(pEndTime.Sub(pStartTime).Hours() / 24)

	switch {
	case differenceInDays <= 1:
		return "day"
	case differenceInDays > 1 && differenceInDays <= 7:
		return "week"
	case differenceInDays > 7:
		return "month"
	default:
		return ""
	}
}
