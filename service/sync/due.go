package sync

import "time"

const InterruptedMessage = "进程重启，同步中断"

func IsDue(enabled bool, intervalSeconds int, nextRunAt *time.Time, lastStatus string, now time.Time) bool {
	if !enabled || intervalSeconds <= 0 || lastStatus == "running" || nextRunAt == nil {
		return false
	}
	return !nextRunAt.After(now)
}

func Interrupted(lastStatus string) (status, message, trigger string, changed bool) {
	if lastStatus != "running" {
		return "", "", "", false
	}
	return "failed", InterruptedMessage, "startup", true
}
