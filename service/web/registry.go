package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/TskFok/DockerImgSync/app/model"
)

type Registry struct {
	ID             int32
	Name           string
	Address        string
	Namespace      string
	CredentialID   int32
	CredentialName string
}

type RegistryStore interface {
	ListRegistries(ctx context.Context) ([]Registry, error)
	GetRegistry(ctx context.Context, id int32) (Registry, error)
	CreateRegistry(ctx context.Context, r Registry) error
	UpdateRegistry(ctx context.Context, r Registry) error
	DeleteRegistry(ctx context.Context, id int32) error
}

func (s *server) handleRegistryList(w http.ResponseWriter, r *http.Request) {
	s.renderRegistryList(w, r, "")
}

func (s *server) renderRegistryList(w http.ResponseWriter, r *http.Request, errMsg string) {
	list, err := s.deps.Registries.ListRegistries(r.Context())
	if err != nil {
		http.Error(w, "读取目标仓库失败", http.StatusInternalServerError)
		return
	}
	s.render(w, "registries", pageData{
		CSRF:       sessionFromCtx(r.Context()).CSRF,
		Error:      errMsg,
		Registries: list,
	})
}

func (s *server) handleRegistryNew(w http.ResponseWriter, r *http.Request) {
	s.renderRegistryForm(w, r, pageData{
		CSRF:   sessionFromCtx(r.Context()).CSRF,
		Action: "/registries",
	})
}

func (s *server) handleRegistryCreate(w http.ResponseWriter, r *http.Request) {
	reg := registryFromForm(r)
	if errMsg := validateRegistry(reg); errMsg != "" {
		s.renderRegistryForm(w, r, pageData{
			CSRF:     sessionFromCtx(r.Context()).CSRF,
			Error:    errMsg,
			Action:   "/registries",
			Registry: reg,
		})
		return
	}
	if err := s.deps.Registries.CreateRegistry(r.Context(), reg); err != nil {
		http.Error(w, "保存目标仓库失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/registries", http.StatusFound)
}

func (s *server) handleRegistryEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	reg, err := s.deps.Registries.GetRegistry(r.Context(), id)
	if err != nil {
		http.Error(w, "目标仓库不存在", http.StatusNotFound)
		return
	}
	s.renderRegistryForm(w, r, pageData{
		CSRF:     sessionFromCtx(r.Context()).CSRF,
		Action:   "/registries/" + strconv.FormatInt(int64(id), 10),
		IsEdit:   true,
		Registry: reg,
	})
}

func (s *server) handleRegistryUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	reg := registryFromForm(r)
	reg.ID = id
	if errMsg := validateRegistry(reg); errMsg != "" {
		s.renderRegistryForm(w, r, pageData{
			CSRF:     sessionFromCtx(r.Context()).CSRF,
			Error:    errMsg,
			Action:   "/registries/" + strconv.FormatInt(int64(id), 10),
			IsEdit:   true,
			Registry: reg,
		})
		return
	}
	if err := s.deps.Registries.UpdateRegistry(r.Context(), reg); err != nil {
		http.Error(w, "保存目标仓库失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/registries", http.StatusFound)
}

func (s *server) handleRegistryDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := s.deps.Registries.DeleteRegistry(r.Context(), id)
	if errors.Is(err, model.ErrInUse) {
		s.renderRegistryList(w, r, err.Error())
		return
	}
	if err != nil {
		http.Error(w, "删除目标仓库失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/registries", http.StatusFound)
}

func (s *server) renderRegistryForm(w http.ResponseWriter, r *http.Request, data pageData) {
	creds, err := s.deps.Store.ListCredentials(r.Context())
	if err != nil {
		http.Error(w, "读取登录信息失败", http.StatusInternalServerError)
		return
	}
	data.Credentials = creds
	s.render(w, "registry_form", data)
}

func registryFromForm(r *http.Request) Registry {
	var credID int32
	if id, err := strconv.ParseInt(r.PostForm.Get("credential_id"), 10, 32); err == nil {
		credID = int32(id)
	}
	return Registry{
		Name:         r.PostForm.Get("name"),
		Address:      r.PostForm.Get("address"),
		Namespace:    r.PostForm.Get("namespace"),
		CredentialID: credID,
	}
}

func validateRegistry(reg Registry) string {
	if err := model.ValidateAddress(reg.Address); err != nil {
		return err.Error()
	}
	if err := model.ValidateNamespace(reg.Namespace); err != nil {
		return err.Error()
	}
	if reg.CredentialID == 0 {
		return "请选择登录信息"
	}
	return ""
}
