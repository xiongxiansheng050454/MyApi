package cachekeys

import "strconv"

// RateLimitBase 是滑动窗口的基础键；计数器会在其后追加 ":<bucket>"。
func RateLimitBase(targetType, targetValue, metric string, window int64) string {
	return "rl:" + metric + ":" + targetType + ":" + targetValue + ":" + strconv.FormatInt(window, 10)
}

func Balance(uid int64) string     { return "bal:" + strconv.FormatInt(uid, 10) }
func BalanceLock(uid int64) string { return "bal:lock:" + strconv.FormatInt(uid, 10) }

func StatsDelta(date string) string { return "stats:delta:" + date }
func StatsFlushing(date string, ts int64) string {
	return "stats:delta:flushing:" + date + ":" + strconv.FormatInt(ts, 10)
}
func StatsDates() string { return "stats:delta:dates" }

func Affinity(uid int64, session, model string) string {
	return "affinity:" + strconv.FormatInt(uid, 10) + ":" + session + ":" + model
}
