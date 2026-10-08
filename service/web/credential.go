package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TskFok/DockerImgSync/app/model"
	"github.com/go-chi/chi/v5"
)

type Credential struct {
	ID       int32
	Name     string
	Username string
	Password string
}

func (s *server) handleCredentialList(w http.ResponseWriter, r *http.Request) {
	s.renderCredentialList(w, r, "")
}

func (s *server) renderCredentialList(w http.ResponseWriter, r *http.Request, errMsg string) {
	list, err := s.deps.Store.ListCredentials(r.Context())
	if err != nil {
		http.Error(w, "读取登录信息失败", http.StatusInternalServerError)
		return
	}
	s.render(w, "credentials", pageData{
		CSRF:        sessionFromCtx(r.Context()).CSRF,
		Error:       errMsg,
		Credentials: list,
	})
}

func (s *server) handleCredentialNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "credential_form", pageData{
		CSRF:   sessionFromCtx(r.Context()).CSRF,
		Action: "/credentials",
	})
}

func (s *server) handleCredentialCreate(w http.ResponseWriter, r *http.Request) {
	c := Credential{
		Name:     r.PostForm.Get("name"),
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	if err := s.deps.Store.CreateCredential(r.Context(), c); err != nil {
		http.Error(w, "保存登录信息失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/credentials", http.StatusFound)
}

func (s *server) handleCredentialEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	c, err := s.deps.Store.GetCredential(r.Context(), id)
	if err != nil {
		http.Error(w, "登录信息不存在", http.StatusNotFound)
		return
	}
	s.render(w, "credential_form", pageData{
		CSRF:       sessionFromCtx(r.Context()).CSRF,
		Action:     "/credentials/" + strconv.FormatInt(int64(id), 10),
		IsEdit:     true,
		Credential: c,
	})
}

func (s *server) handleCredentialUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	c := Credential{
		ID:       id,
		Name:     r.PostForm.Get("name"),
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	if err := s.deps.Store.UpdateCredential(r.Context(), c); err != nil {
		http.Error(w, "保存登录信息失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/credentials", http.StatusFound)
}

func (s *server) handleCredentialDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := s.deps.Store.DeleteCredential(r.Context(), id)
	if errors.Is(err, model.ErrInUse) {
		s.renderCredentialList(w, r, err.Error())
		return
	}
	if err != nil {
		http.Error(w, "删除登录信息失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/credentials", http.StatusFound)
}

func parseID(w http.ResponseWriter, r *http.Request) (int32, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil {
		http.Error(w, "无效编号", http.StatusBadRequest)
		return 0, false
	}
	return int32(id), true
}
