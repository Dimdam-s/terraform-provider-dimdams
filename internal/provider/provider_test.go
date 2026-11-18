package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRefusesPlainHTTPAndRedirects(t *testing.T) {
	if _, err := NewClient("http://www.dimdams.com", "k", "test"); err == nil {
		t.Fatal("plain HTTP must be refused: the key would travel in clear text")
	}
	if _, err := NewClient("http://127.0.0.1:3000", "k", "test"); err != nil {
		t.Fatalf("loopback HTTP is allowed for local development: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://evil.example/", http.StatusFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dim_live_x" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"success":false,"error":"Serveur introuvable"}`))
		case "/soft-error":
			w.Write([]byte(`{"success":false,"error":"Limite atteinte"}`))
		default:
			w.Write([]byte(`{"success":true,"value":42}`))
		}
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, "dim_live_x", "test")
	ctx := context.Background()
	var out struct{ Value int }
	if err := c.Do(ctx, http.MethodGet, "/ok", nil, &out); err != nil || out.Value != 42 {
		t.Fatalf("ok: %v %v", err, out)
	}
	if err := c.Do(ctx, http.MethodGet, "/missing", nil, nil); !IsNotFound(err) {
		t.Fatalf("404 must be reported as not found: %v", err)
	}
	if err := c.Do(ctx, http.MethodGet, "/soft-error", nil, nil); err == nil || IsNotFound(err) {
		t.Fatalf("success:false must be an error: %v", err)
	}
	if err := c.Do(ctx, http.MethodGet, "/redirect", nil, nil); err == nil {
		t.Fatal("redirects must not be followed")
	}
}

func TestSplitEnvKeepsKnownSecretsAndPlacement(t *testing.T) {
	remote := []apiEnvVar{
		{Key: "API_TOKEN", Value: maskedValue, IsSecret: true}, // masked for a member without the configuration right
		{Key: "PUBLIC_KEY_ID", Value: "abc", IsSecret: true},   // flagged secret by its name, configured as a variable
		{Key: "NEW_SECRET", Value: "s", IsSecret: true},        // added from the dashboard
		{Key: "PORT", Value: "3000"},
	}
	vars, secrets := splitEnv(remote, map[string]string{"PUBLIC_KEY_ID": "abc", "PORT": "8080"}, map[string]string{"API_TOKEN": "real"})
	if secrets["API_TOKEN"] != "real" {
		t.Errorf("masked secret must keep the known value, got %q", secrets["API_TOKEN"])
	}
	if vars["PUBLIC_KEY_ID"] != "abc" {
		t.Errorf("a key stays in the map the configuration put it in")
	}
	if secrets["NEW_SECRET"] != "s" || vars["PORT"] != "3000" {
		t.Errorf("drift must show: %v %v", vars, secrets)
	}
	list := envList(map[string]string{"B": "2"}, map[string]string{"A": "1"})
	data, _ := json.Marshal(list)
	if string(data) != `[{"key":"A","value":"1","isSecret":true},{"key":"B","value":"2","isSecret":false}]` {
		t.Errorf("envList = %s", data)
	}
}

func TestFindServiceAndNames(t *testing.T) {
	domain := "blog.example.com"
	hosts := []apiHost{
		{UniqueID: "web-1", Type: "web", Config: json.RawMessage(`"{\"name\":\"Blog\"}"`), Domain: &domain},
		{Unique: "bot-1", Type: "bot", Config: json.RawMessage(`{"botName":"Bot"}`)},
		{ID: "bot-2", Type: "bot", Config: json.RawMessage(`{"name":"bot"}`)},
	}
	if hosts[0].name() != "Blog" || hosts[1].name() != "Bot" || hosts[1].id() != "bot-1" {
		t.Fatalf("names: %q %q %q", hosts[0].name(), hosts[1].name(), hosts[1].id())
	}
	if h, err := FindService(hosts, "", "blog"); err != nil || h.id() != "web-1" {
		t.Fatalf("by name: %v %v", h, err)
	}
	if h, err := FindService(hosts, "bot-2", ""); err != nil || h.id() != "bot-2" {
		t.Fatalf("by id: %v %v", h, err)
	}
	if _, err := FindService(hosts, "", "BOT"); err == nil {
		t.Fatal("an ambiguous name must be refused")
	}
	if _, err := FindService(hosts, "nope", ""); err == nil {
		t.Fatal("unknown id")
	}
}
