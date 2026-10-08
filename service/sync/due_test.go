package sync

import (
	"testing"
	"time"
)

func TestIsDue(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	if !IsDue(true, 60, &past, "success", now) {
		t.Fatal("到期任务应被选中")
	}
	if !IsDue(true, 60, &now, "failed", now) {
		t.Fatal("刚好到期应被选中")
	}
	if IsDue(false, 60, &past, "success", now) ||
		IsDue(true, 0, &past, "success", now) ||
		IsDue(true, 60, &future, "success", now) ||
		IsDue(true, 60, &past, "running", now) ||
		IsDue(true, 60, nil, "idle", now) {
		t.Fatal("未启用、间隔为 0、未到期、正在运行或没有下次时间的任务不应到期")
	}
}

func TestInterrupted(t *testing.T) {
	status, message, trigger, changed := Interrupted("running")
	if !changed || status != "failed" || message != InterruptedMessage || trigger != "startup" {
		t.Fatalf("got %s %s %s %v", status, message, trigger, changed)
	}
	if _, _, _, changed := Interrupted("success"); changed {
		t.Fatal("非 running 不应改写")
	}
}
