package model

import (
	"errors"
	"strings"
)

var ErrInUse = errors.New("仍被引用，不能删除")
var ErrTaskRunning = errors.New("任务正在同步，不能删除")

func CanDeleteCredential(registryCount, taskCount int) error {
	if registryCount > 0 || taskCount > 0 {
		return ErrInUse
	}
	return nil
}

func CanDeleteRegistry(taskCount int) error {
	if taskCount > 0 {
		return ErrInUse
	}
	return nil
}

func CanDeleteTask(lastStatus string) error {
	if lastStatus == "running" {
		return ErrTaskRunning
	}
	return nil
}

func ValidateAddress(address string) error {
	if address == "" || strings.Contains(address, "://") || strings.Contains(address, "/") {
		return errors.New("地址不能带协议或路径")
	}
	return nil
}

func ValidateNamespace(namespace string) error {
	if namespace == "" || strings.Contains(namespace, "/") {
		return errors.New("命名空间不能含 /")
	}
	return nil
}

func ValidateInterval(seconds int) error {
	if seconds == 0 || seconds >= 60 {
		return nil
	}
	return errors.New("检查间隔至少 60 秒")
}
