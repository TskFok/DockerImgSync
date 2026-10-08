package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TskFok/DockerImgSync/app/model"
	syncsvc "github.com/TskFok/DockerImgSync/service/sync"
)

type SyncTask struct {
	ID                 int32
	Name               string
	SourceImage        string
	SourceCredentialID *int32
	RegistryID         int32
	DestRepository     string
	DestTag            string
	IntervalSeconds    int
	Enabled            bool
	LastDigest         string
	LastStatus         string
	LastError          string
	LastSyncedAt       *time.Time
	NextRunAt          *time.Time
}

func (t SyncTask) SourceCredentialValue() int32 {
	if t.SourceCredentialID == nil {
		return 0
	}
	return *t.SourceCredentialID
}

func (t SyncTask) DestRepositoryLabel() string {
	if t.DestRepository == "" {
		return "与源相同"
	}
	return t.DestRepository
}

func (t SyncTask) DestTagLabel() string {
	if t.DestTag == "" {
		return "与源相同"
	}
	return t.DestTag
}

type SyncLog struct {
	Trigger      string
	Status       string
	SourceDigest string
	Message      string
	StartedAt    time.Time
	FinishedAt   time.Time
}

type TaskStore interface {
	ListTasks(ctx context.Context) ([]SyncTask, error)
	GetTask(ctx context.Context, id int32) (SyncTask, error)
	ListLogs(ctx context.Context, taskID int32) ([]SyncLog, error)
	CreateTask(ctx context.Context, task SyncTask, now time.Time) error
	UpdateTask(ctx context.Context, task SyncTask, now time.Time) error
	DeleteTask(ctx context.Context, id int32) error
	SetEnabled(ctx context.Context, id int32, enabled bool, now time.Time) error
}

type Syncer interface {
	SyncNow(ctx context.Context, id int32) error
}

func (s *server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	s.renderTaskList(w, r, "")
}

func (s *server) renderTaskList(w http.ResponseWriter, r *http.Request, errMsg string) {
	list, err := s.deps.Tasks.ListTasks(r.Context())
	if err != nil {
		http.Error(w, "读取同步任务失败", http.StatusInternalServerError)
		return
	}
	s.render(w, "tasks", pageData{
		CSRF:  sessionFromCtx(r.Context()).CSRF,
		Error: errMsg,
		Tasks: list,
	})
}

func (s *server) handleTaskNew(w http.ResponseWriter, r *http.Request) {
	s.renderTaskForm(w, r, pageData{
		Action: "/tasks",
		Task:   SyncTask{Enabled: true},
	})
}

func (s *server) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	task, errMsg := taskFromForm(r)
	if errMsg != "" {
		s.renderTaskForm(w, r, pageData{Error: errMsg, Action: "/tasks", Task: task})
		return
	}
	now := time.Now()
	task, errMsg = prepareTask(task, nil, now)
	if errMsg != "" {
		s.renderTaskForm(w, r, pageData{Error: errMsg, Action: "/tasks", Task: task})
		return
	}
	if err := s.deps.Tasks.CreateTask(r.Context(), task, now); err != nil {
		http.Error(w, "保存同步任务失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tasks", http.StatusFound)
}

func (s *server) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	task, err := s.deps.Tasks.GetTask(r.Context(), id)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	logs, err := s.deps.Tasks.ListLogs(r.Context(), id)
	if err != nil {
		http.Error(w, "读取同步记录失败", http.StatusInternalServerError)
		return
	}
	s.render(w, "task_detail", pageData{
		CSRF:    sessionFromCtx(r.Context()).CSRF,
		Task:    task,
		Logs:    logs,
		Message: r.URL.Query().Get("msg"),
	})
}

func (s *server) handleTaskEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	task, err := s.deps.Tasks.GetTask(r.Context(), id)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	s.renderTaskForm(w, r, pageData{
		Action: "/tasks/" + strconv.FormatInt(int64(id), 10),
		IsEdit: true,
		Task:   task,
	})
}

func (s *server) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	existing, err := s.deps.Tasks.GetTask(r.Context(), id)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	task, errMsg := taskFromForm(r)
	if errMsg != "" {
		task.ID = id
		s.renderTaskForm(w, r, pageData{
			Error:  errMsg,
			Action: "/tasks/" + strconv.FormatInt(int64(id), 10),
			IsEdit: true,
			Task:   task,
		})
		return
	}
	task.ID = id
	task.LastDigest = existing.LastDigest
	task.LastStatus = existing.LastStatus
	task.LastError = existing.LastError
	task.LastSyncedAt = existing.LastSyncedAt
	now := time.Now()
	task, errMsg = prepareTask(task, &existing, now)
	if errMsg != "" {
		s.renderTaskForm(w, r, pageData{
			Error:  errMsg,
			Action: "/tasks/" + strconv.FormatInt(int64(id), 10),
			IsEdit: true,
			Task:   task,
		})
		return
	}
	if err := s.deps.Tasks.UpdateTask(r.Context(), task, now); err != nil {
		http.Error(w, "保存同步任务失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tasks/"+strconv.FormatInt(int64(id), 10), http.StatusFound)
}

func (s *server) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	task, err := s.deps.Tasks.GetTask(r.Context(), id)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	if err := model.CanDeleteTask(task.LastStatus); err != nil {
		s.renderTaskList(w, r, err.Error())
		return
	}
	if err := s.deps.Tasks.DeleteTask(r.Context(), id); err != nil {
		http.Error(w, "删除同步任务失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tasks", http.StatusFound)
}

func (s *server) handleTaskSync(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	// 请求上下文会在响应结束后取消，复制必须脱离该上下文。
	err := s.deps.Syncer.SyncNow(context.Background(), id)
	loc := "/tasks/" + strconv.FormatInt(int64(id), 10)
	if errors.Is(err, syncsvc.ErrBusy) {
		q := url.Values{}
		q.Set("msg", syncsvc.ErrBusy.Error())
		http.Redirect(w, r, loc+"?"+q.Encode(), http.StatusFound)
		return
	}
	if err != nil {
		http.Error(w, "立即同步失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, loc, http.StatusFound)
}

func (s *server) handleTaskToggle(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	enabled := r.PostForm.Get("enabled") == "1"
	if err := s.deps.Tasks.SetEnabled(r.Context(), id, enabled, time.Now()); err != nil {
		http.Error(w, "更新启用状态失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tasks/"+strconv.FormatInt(int64(id), 10), http.StatusFound)
}

func (s *server) renderTaskForm(w http.ResponseWriter, r *http.Request, data pageData) {
	data.CSRF = sessionFromCtx(r.Context()).CSRF
	creds, err := s.deps.Store.ListCredentials(r.Context())
	if err != nil {
		http.Error(w, "读取登录信息失败", http.StatusInternalServerError)
		return
	}
	regs, err := s.deps.Registries.ListRegistries(r.Context())
	if err != nil {
		http.Error(w, "读取目标仓库失败", http.StatusInternalServerError)
		return
	}
	data.Credentials = creds
	data.Registries = regs
	s.render(w, "task_form", data)
}

func taskFromForm(r *http.Request) (SyncTask, string) {
	interval, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("interval_seconds")))
	if err != nil {
		return SyncTask{
			Name:               r.PostForm.Get("name"),
			SourceImage:        strings.TrimSpace(r.PostForm.Get("source_image")),
			SourceCredentialID: optionalInt32(r.PostForm.Get("source_credential_id")),
			RegistryID:         formInt32(r.PostForm.Get("registry_id")),
			DestRepository:     strings.TrimSpace(r.PostForm.Get("dest_repository")),
			DestTag:            strings.TrimSpace(r.PostForm.Get("dest_tag")),
			Enabled:            r.PostForm.Get("enabled") == "1",
		}, "检查间隔无效"
	}
	return SyncTask{
		Name:               r.PostForm.Get("name"),
		SourceImage:        strings.TrimSpace(r.PostForm.Get("source_image")),
		SourceCredentialID: optionalInt32(r.PostForm.Get("source_credential_id")),
		RegistryID:         formInt32(r.PostForm.Get("registry_id")),
		DestRepository:     strings.TrimSpace(r.PostForm.Get("dest_repository")),
		DestTag:            strings.TrimSpace(r.PostForm.Get("dest_tag")),
		IntervalSeconds:    interval,
		Enabled:            r.PostForm.Get("enabled") == "1",
	}, ""
}

func prepareTask(task SyncTask, previous *SyncTask, now time.Time) (SyncTask, string) {
	normalized, _, _, err := syncsvc.ParseSource(task.SourceImage)
	if err != nil {
		return task, err.Error()
	}
	task.SourceImage = normalized
	if err := model.ValidateInterval(task.IntervalSeconds); err != nil {
		return task, err.Error()
	}
	if previous == nil {
		task.NextRunAt = nil
		if task.IntervalSeconds > 0 {
			task.NextRunAt = &now
		}
		if task.LastStatus == "" {
			task.LastStatus = "idle"
		}
		return task, ""
	}
	task.NextRunAt = previous.NextRunAt
	if task.IntervalSeconds == 0 {
		task.NextRunAt = nil
	} else if previous.IntervalSeconds == 0 || (!previous.Enabled && task.Enabled) {
		task.NextRunAt = &now
	}
	return task, ""
}

func optionalInt32(raw string) *int32 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || id == 0 {
		return nil
	}
	v := int32(id)
	return &v
}

func formInt32(raw string) int32 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0
	}
	return int32(id)
}
