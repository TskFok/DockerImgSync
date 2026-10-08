package sync

import (
	"errors"
	"testing"
)

func TestSplitDecryptFailuresSkipsAndCollectsIDs(t *testing.T) {
	rows := []joinedTask{
		{ID: 1, SourceImage: "nginx:latest"},
		{ID: 2, SourceImage: "redis:latest"},
		{ID: 3, SourceImage: "alpine:3"},
	}
	tasks, failedIDs := splitDecryptFailures(rows, func(row joinedTask) (Task, error) {
		if row.ID == 2 {
			return Task{}, errors.New("cipher: message authentication failed")
		}
		return Task{ID: row.ID, SourceImage: row.SourceImage}, nil
	})
	if len(tasks) != 2 || tasks[0].ID != 1 || tasks[1].ID != 3 {
		t.Fatalf("应跳过解密失败的任务，得到 %+v", tasks)
	}
	if len(failedIDs) != 1 || failedIDs[0] != 2 {
		t.Fatalf("失败 id=%v，应只含 2，供一次 WHERE id IN 标记", failedIDs)
	}
	if decryptFailedMessage != "登录信息无法解密" {
		t.Fatalf("last_error=%q", decryptFailedMessage)
	}
}
