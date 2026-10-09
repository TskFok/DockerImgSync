package web

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/TskFok/DockerImgSync/app/model"
)

type fakeRegistryStore struct {
	registries  []Registry
	created     []Registry
	createCount int
	updateCount int
	deleteCount int
	deleteErr   error
}

func (f *fakeRegistryStore) ListRegistries(ctx context.Context) ([]Registry, error) {
	return f.registries, nil
}

func (f *fakeRegistryStore) GetRegistry(ctx context.Context, id int32) (Registry, error) {
	for _, r := range f.registries {
		if r.ID == id {
			return r, nil
		}
	}
	return Registry{}, nil
}

func (f *fakeRegistryStore) CreateRegistry(ctx context.Context, r Registry) error {
	f.createCount++
	f.created = append(f.created, r)
	return nil
}

func (f *fakeRegistryStore) UpdateRegistry(ctx context.Context, r Registry) error {
	f.updateCount++
	return nil
}

func (f *fakeRegistryStore) DeleteRegistry(ctx context.Context, id int32) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleteCount++
	return nil
}

func testRegistryRouter(regs *fakeRegistryStore) http.Handler {
	return NewRouter(Deps{
		Store:         &fakeCredentialStore{creds: []Credential{{ID: 1, Name: "阿里云"}}},
		Registries:    regs,
		AdminUser:     "admin",
		AdminPassword: "admin-pass",
		SessionSecret: "session-secret-must-be-32-characters-min",
	})
}

func loggedInCSRF(t *testing.T, h http.Handler, cookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/credentials", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /credentials status=%d body=%s", rec.Code, rec.Body.String())
	}
	return csrfFromHTML(t, rec.Body.String())
}

func TestRegistryCreateRejectsAddressWithProtocol(t *testing.T) {
	store := &fakeRegistryStore{}
	h := testRegistryRouter(store)
	cookie := doLogin(t, h)
	csrf := loggedInCSRF(t, h, cookie)
	form := url.Values{}
	form.Set("csrf", csrf)
	form.Set("name", "杭州")
	form.Set("address", "https://registry.example.com")
	form.Set("namespace", "myns")
	form.Set("credential_id", "1")
	req := httptest.NewRequest(http.MethodPost, "/registries", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "地址不能带协议或路径") {
		t.Fatalf("正文应含地址错误: %s", rec.Body.String())
	}
	if store.createCount != 0 {
		t.Fatalf("创建次数=%d，应为 0", store.createCount)
	}
}

func TestRegistryCreateStoresValidRecord(t *testing.T) {
	store := &fakeRegistryStore{}
	h := testRegistryRouter(store)
	cookie := doLogin(t, h)
	csrf := loggedInCSRF(t, h, cookie)
	form := url.Values{}
	form.Set("csrf", csrf)
	form.Set("name", "杭州")
	form.Set("address", "registry.example.com")
	form.Set("namespace", "myns")
	form.Set("credential_id", "1")
	req := httptest.NewRequest(http.MethodPost, "/registries", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.createCount != 1 {
		t.Fatalf("创建次数=%d，应为 1", store.createCount)
	}
	got := store.created[0]
	if got.Name != "杭州" || got.Address != "registry.example.com" || got.Namespace != "myns" || got.CredentialID != 1 {
		t.Fatalf("假存储记录=%+v", got)
	}
}

func TestRegistryRejectsZeroCredential(t *testing.T) {
	store := &fakeRegistryStore{}
	h := testRegistryRouter(store)
	cookie := doLogin(t, h)
	csrf := loggedInCSRF(t, h, cookie)
	for _, path := range []string{"/registries", "/registries/1"} {
		form := url.Values{}
		form.Set("csrf", csrf)
		form.Set("name", "杭州")
		form.Set("address", "registry.example.com")
		form.Set("namespace", "myns")
		form.Set("credential_id", "0")
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "请选择登录信息") {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if store.createCount != 0 || store.updateCount != 0 {
		t.Fatalf("不应调用存储 create=%d update=%d", store.createCount, store.updateCount)
	}
}

func TestRegistryDeleteInUse(t *testing.T) {
	store := &fakeRegistryStore{deleteErr: model.ErrInUse}
	h := testRegistryRouter(store)
	cookie := doLogin(t, h)
	csrf := loggedInCSRF(t, h, cookie)
	form := url.Values{}
	form.Set("csrf", csrf)
	req := httptest.NewRequest(http.MethodPost, "/registries/1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "仍被引用，不能删除") {
		t.Fatalf("正文应含引用错误: %s", rec.Body.String())
	}
	if store.deleteCount != 0 {
		t.Fatalf("删除计数=%d，应为 0", store.deleteCount)
	}
}

func TestRegistryListClipsOverflowFields(t *testing.T) {
	name := `杭州仓库 <b>` + strings.Repeat("名", 60)
	address := "registry.example.com/" + strings.Repeat("region-", 20)
	namespace := strings.Repeat("team_", 40) + `<ns>`
	credential := `推送账号 <i>` + strings.Repeat("凭据", 40)
	h := testRegistryRouter(&fakeRegistryStore{registries: []Registry{{
		ID:             2,
		Name:           name,
		Address:        address,
		Namespace:      namespace,
		CredentialName: credential,
	}}})
	cookie := doLogin(t, h)
	req := httptest.NewRequest(http.MethodGet, "/registries", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="registry-list"`,
		`class="clip-visible"`,
		`class="clip-pop" role="tooltip"`,
		`.registry-list { table-layout: fixed; }`,
		html.EscapeString(name),
		html.EscapeString(address),
		html.EscapeString(namespace),
		html.EscapeString(credential),
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("列表应包含 %q", want)
		}
	}
	if strings.Contains(body, "<b>") || strings.Contains(body, "<ns>") || strings.Contains(body, "<i>") {
		t.Fatal("超长字段中的 HTML 应被转义")
	}
	for _, raw := range []string{name, address, namespace, credential} {
		if strings.Count(body, html.EscapeString(raw)) != 2 {
			t.Fatalf("%q 应同时保留可见文本与悬停全文", raw)
		}
	}
}
