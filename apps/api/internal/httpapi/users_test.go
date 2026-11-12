package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/mail"
	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

// outbox stands in for the mail relay and keeps what would have been sent.
type outbox struct {
	mu   sync.Mutex
	sent []mail.Message
	fail error
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.fail != nil {
		return o.fail
	}
	o.sent = append(o.sent, m)
	return nil
}

var codeInMail = regexp.MustCompile(`\b[0-9]{6}\b`)

// code returns the sign-in code in the newest message.
func (o *outbox) code(t *testing.T) string {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.sent) == 0 {
		t.Fatal("no mail was sent")
	}
	m := o.sent[len(o.sent)-1]
	code := codeInMail.FindString(m.Text)
	if code == "" || !strings.Contains(m.Subject, code) {
		t.Fatalf("no code in the mail: %q / %q", m.Subject, m.Text)
	}
	return code
}

func (o *outbox) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.sent)
}

// withUsers is withRecords plus a mail relay and somewhere to keep builds.
func withUsers(t *testing.T) (*fixture, *time.Time, *outbox) {
	t.Helper()
	f, now := withRecords(t)
	box := &outbox{}
	f.srv.Mail, f.srv.ReleasesDir = box, filepath.Join(t.TempDir(), "releases")
	f.handler = f.srv.Handler()
	return f, now, box
}

// notCode returns a six-digit code that is not the right one.
func notCode(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

// signInUser takes an address through both steps and returns its token.
func signInUser(t *testing.T, f *fixture, box *outbox, email string) string {
	t.Helper()
	if rec := f.send("POST", "/v1/auth/start", `{"email":"`+email+`"}`); rec.Code != 200 {
		t.Fatalf("start for %s = %d %s", email, rec.Code, rec.Body)
	}
	rec := f.send("POST", "/v1/auth/verify", `{"email":"`+email+`","code":"`+box.code(t)+`","client":"Still 0.1.0 macOS"}`)
	if rec.Code != 200 {
		t.Fatalf("verify for %s = %d %s", email, rec.Code, rec.Body)
	}
	token, _ := decode(t, rec)["token"].(string)
	if len(token) < 40 {
		t.Fatalf("token = %q", token)
	}
	return token
}

func TestSignInWithACode(t *testing.T) {
	f, now, box := withUsers(t)
	if st := decode(t, f.do("GET", "/v1/auth/status")); st["signIn"] != true {
		t.Fatalf("status = %v", st)
	}

	rec := f.send("POST", "/v1/auth/start", `{"email":"  Ana@Example.com "}`, "CF-IPCountry", "in")
	if rec.Code != 200 || decode(t, rec)["ok"] != true || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	if box.count() != 1 || box.sent[0].To != "ana@example.com" {
		t.Fatalf("mail = %+v", box.sent)
	}
	code := box.code(t)
	verify := func(code string) *httptest.ResponseRecorder {
		return f.send("POST", "/v1/auth/verify", `{"email":"ana@example.com","code":"`+code+`"}`, "CF-IPCountry", "in")
	}
	if rec := verify(notCode(code)); rec.Code != 401 || decode(t, rec)["error"] != "wrong_code" {
		t.Errorf("wrong code = %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{"12345", "1234567", "abcdef", ""} {
		if rec := verify(bad); rec.Code != 400 {
			t.Errorf("code %q = %d, want 400", bad, rec.Code)
		}
	}
	// Typed the way a mail program shows it, with a space in the middle.
	rec = verify(code[:3] + " " + code[3:])
	if rec.Code != 200 {
		t.Fatalf("right code = %d %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	token, _ := body["token"].(string)
	if user := body["user"].(map[string]any); user["email"] != "ana@example.com" || len(token) < 40 {
		t.Fatalf("verify = %v", body)
	}
	if rec := verify(code); rec.Code != 401 {
		t.Errorf("the same code twice = %d, want 401", rec.Code)
	}
	u, err := f.srv.Records.UserByEmail(t.Context(), "ana@example.com")
	if err != nil || u.Country != "IN" || u.Status != records.UserActive {
		t.Fatalf("stored user = %+v, %v", u, err)
	}

	me := func(token string) *httptest.ResponseRecorder {
		if token == "" {
			return f.do("GET", "/v1/me")
		}
		return f.do("GET", "/v1/me", "Authorization", "Bearer "+token)
	}
	if rec := me(token); rec.Code != 200 || decode(t, rec)["email"] != "ana@example.com" {
		t.Errorf("me = %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{"", "nonsense", token + "x"} {
		if rec := me(bad); rec.Code != 401 {
			t.Errorf("me with token %q = %d, want 401", bad, rec.Code)
		}
	}
	// A cookie is not a way in: only the header counts.
	if rec := f.do("GET", "/v1/me", "Cookie", "token="+token); rec.Code != 401 {
		t.Errorf("me with a cookie = %d, want 401", rec.Code)
	}

	// Signing in again is the same person, not a second one; another address is.
	*now = now.Add(time.Hour)
	again := signInUser(t, f, box, "ana@example.com")
	if n, _ := f.srv.Records.UserTotals(t.Context()); n.Total != 1 || again == token {
		t.Errorf("users after ana signed in twice = %d, want 1 (and a new token)", n.Total)
	}
	signInUser(t, f, box, "other@example.com")
	if n, _ := f.srv.Records.UserTotals(t.Context()); n.Total != 2 {
		t.Errorf("users = %d, want 2", n.Total)
	}
	// Both of ana's sign-ins work until each is ended.
	if rec := me(again); rec.Code != 200 {
		t.Errorf("me with the second token = %d", rec.Code)
	}

	if rec := f.send("POST", "/v1/auth/signout", "", "Authorization", "Bearer "+token); rec.Code != 204 {
		t.Errorf("sign out = %d", rec.Code)
	}
	if rec := me(token); rec.Code != 401 {
		t.Errorf("me after signing out = %d, want 401", rec.Code)
	}
}

func TestSignInOffWithoutMail(t *testing.T) {
	f, _ := withRecords(t)
	if st := decode(t, f.do("GET", "/v1/auth/status")); st["signIn"] != false {
		t.Errorf("status = %v", st)
	}
	for _, target := range []string{"/v1/auth/start", "/v1/auth/verify"} {
		if rec := f.send("POST", target, `{"email":"ana@example.com","code":"123456"}`); rec.Code != 503 || decode(t, rec)["error"] != "signin_off" {
			t.Errorf("%s with no mail = %d %s", target, rec.Code, rec.Body)
		}
	}
}

func TestCodeLimits(t *testing.T) {
	f, now, box := withUsers(t)
	start := func(email, ip string) int {
		return f.send("POST", "/v1/auth/start", `{"email":"`+email+`"}`, "CF-Connecting-IP", ip).Code
	}
	verify := func(code string) int {
		return f.send("POST", "/v1/auth/verify", `{"email":"ana@example.com","code":"`+code+`"}`).Code
	}

	if code := start("ana@example.com", "203.0.113.5"); code != 200 {
		t.Fatalf("first code = %d", code)
	}
	if code := start("ana@example.com", "203.0.113.6"); code != 429 {
		t.Errorf("a second code straight away = %d, want 429", code)
	}
	// Five wrong guesses use the code up, even if the sixth is right.
	code := box.code(t)
	for i := range codeAttempts {
		if got := verify(notCode(code)); got != 401 {
			t.Fatalf("wrong guess %d = %d", i+1, got)
		}
	}
	if got := verify(code); got != 401 {
		t.Errorf("the right code after %d wrong ones = %d, want 401", codeAttempts, got)
	}

	// A code runs out.
	*now = now.Add(codeResendWait + time.Second)
	if code := start("ana@example.com", "203.0.113.5"); code != 200 {
		t.Fatalf("a code after waiting = %d", code)
	}
	code = box.code(t)
	*now = now.Add(codeTTL + time.Second)
	if got := verify(code); got != 401 {
		t.Errorf("a code past its time = %d, want 401", got)
	}

	// So many an hour for one address, however long they wait in between.
	for i := 2; i < codesPerEmailPerHour; i++ {
		if code := start("ana@example.com", "203.0.113.5"); code != 200 {
			t.Fatalf("code %d = %d", i+1, code)
		}
		*now = now.Add(codeResendWait + time.Second)
	}
	if code := start("ana@example.com", "203.0.113.7"); code != 429 {
		t.Errorf("past the hourly allowance for one address = %d, want 429", code)
	}
	sent := box.count()
	if sent != codesPerEmailPerHour {
		t.Errorf("mail sent = %d, want %d", sent, codesPerEmailPerHour)
	}

	// And so many from one connection, whatever the address.
	g, _, _ := withUsers(t)
	for i := range codesPerIPPerHour {
		if rec := g.send("POST", "/v1/auth/start", fmt.Sprintf(`{"email":"p%d@example.com"}`, i), "CF-Connecting-IP", "198.51.100.9"); rec.Code != 200 {
			t.Fatalf("code %d from one connection = %d", i+1, rec.Code)
		}
	}
	if rec := g.send("POST", "/v1/auth/start", `{"email":"extra@example.com"}`, "CF-Connecting-IP", "198.51.100.9"); rec.Code != 429 {
		t.Errorf("past the allowance for one connection = %d, want 429", rec.Code)
	}

	// A relay that refuses is reported, and the relay's answer is not passed on.
	h, _, hbox := withUsers(t)
	hbox.fail = errors.New("550 mailbox ana@example.com unavailable")
	rec := h.send("POST", "/v1/auth/start", `{"email":"ana@example.com"}`)
	if rec.Code != 502 || strings.Contains(rec.Body.String(), "550") {
		t.Errorf("relay failure = %d %s", rec.Code, rec.Body)
	}
}

func TestEvents(t *testing.T) {
	f, now, box := withUsers(t)
	token := signInUser(t, f, box, "ana@example.com")
	id := f.ids["Low Tide"]
	post := func(body string, headers ...string) *httptest.ResponseRecorder {
		if headers == nil {
			headers = []string{"Authorization", "Bearer " + token}
		}
		return f.send("POST", "/v1/events", body, headers...)
	}

	if rec := post(`{"events":[{"kind":"save","subject":"`+id+`"}]}`, "X-None", "1"); rec.Code != 401 {
		t.Errorf("events signed out = %d, want 401", rec.Code)
	}
	old := now.Add(-2 * time.Hour).Format(time.RFC3339)
	rec := post(`{"events":[
		{"kind":"app_open","subject":"0.1.0","detail":"macOS 15.1"},
		{"kind":"save","subject":"` + id + `","at":"` + old + `"},
		{"kind":"set","subject":"` + id + `","at":"2031-01-01T00:00:00Z"},
		{"kind":"search","detail":"  quiet\u0007 blue  "},
		{"kind":"made_up","subject":"x"},
		{"kind":"save","subject":"not a subject!"},
		{"kind":"save"}
	]}`)
	if rec.Code != 200 || decode(t, rec)["accepted"] != float64(4) {
		t.Fatalf("events = %d %s", rec.Code, rec.Body)
	}
	u, _ := f.srv.Records.UserByEmail(t.Context(), "ana@example.com")
	got, err := f.srv.Records.UserEvents(t.Context(), u.ID, 10)
	if err != nil || len(got) != 4 {
		t.Fatalf("stored events = %+v, %v", got, err)
	}
	byKind := map[string]records.UserEvent{}
	for _, e := range got {
		byKind[e.Kind] = e
	}
	if e := byKind["save"]; e.Subject != id || !e.At.Equal(now.Add(-2*time.Hour)) {
		t.Errorf("save = %+v, want the time the app gave", e)
	}
	if e := byKind["set"]; !e.At.Equal(*now) {
		t.Errorf("set = %+v, want now: a time in the future is not believed", e)
	}
	if e := byKind["search"]; e.Detail != "quiet blue" {
		t.Errorf("search detail = %q", e.Detail)
	}

	for body, want := range map[string]int{
		`{"events":[]}`: 400,
		`{}`:            400,
		`not json`:      400,
		`{"events":[` + strings.TrimSuffix(strings.Repeat(`{"kind":"view"},`, maxEventsPerPost+1), ",") + `]}`: 400,
	} {
		if rec := post(body); rec.Code != want {
			t.Errorf("events %.30s = %d, want %d", body, rec.Code, want)
		}
	}

	// Asking to be forgotten removes the person and what they did.
	if rec := f.send("DELETE", "/v1/me", "", "Authorization", "Bearer "+token); rec.Code != 204 {
		t.Fatalf("delete me = %d", rec.Code)
	}
	if _, err := f.srv.Records.UserByEmail(t.Context(), "ana@example.com"); !errors.Is(err, records.ErrNotFound) {
		t.Errorf("user after deleting = %v, want not found", err)
	}
	if left, _ := f.srv.Records.UserEvents(t.Context(), u.ID, 10); len(left) != 0 {
		t.Errorf("events after deleting = %d", len(left))
	}
	if rec := post(`{"events":[{"kind":"view"}]}`); rec.Code != 401 {
		t.Errorf("events after deleting = %d, want 401", rec.Code)
	}
}

func TestAppMayPostAcrossOrigins(t *testing.T) {
	f, _, _ := withUsers(t)
	rec := f.do("OPTIONS", "/v1/events", "Origin", "tauri://localhost",
		"Access-Control-Request-Method", "POST", "Access-Control-Request-Headers", "authorization, content-type")
	h := rec.Header()
	if rec.Code != 204 || h.Get("Access-Control-Allow-Origin") != "tauri://localhost" ||
		!strings.Contains(h.Get("Access-Control-Allow-Methods"), "POST") || h.Get("Access-Control-Allow-Headers") != "authorization, content-type" {
		t.Errorf("preflight = %d %v", rec.Code, h)
	}
	// Cookies are never allowed across origins.
	if h.Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("credentials allowed across origins: %v", h)
	}
}

func TestAdminUsers(t *testing.T) {
	f, now, box := withUsers(t)
	cookie := signIn(t, f)
	admin := func(method, target, body string) *httptest.ResponseRecorder {
		return f.send(method, target, body, "Cookie", cookie)
	}
	ana := signInUser(t, f, box, "ana@example.com")
	*now = now.Add(time.Hour)
	ben := signInUser(t, f, box, "ben@example.com")
	id := f.ids["Low Tide"]
	f.send("POST", "/v1/events", `{"events":[{"kind":"save","subject":"`+id+`"},{"kind":"pack_save","subject":"quiet-gradients"},{"kind":"search","detail":"blue"}]}`,
		"Authorization", "Bearer "+ana)
	f.send("POST", "/v1/events", `{"events":[{"kind":"save","subject":"`+id+`"},{"kind":"set","subject":"`+id+`"}]}`,
		"Authorization", "Bearer "+ben)

	if rec := f.do("GET", "/v1/admin/users"); rec.Code != 401 {
		t.Fatalf("users signed out = %d, want 401", rec.Code)
	}
	// An app account is not a way into the admin portal.
	if rec := f.do("GET", "/v1/admin/users", "Authorization", "Bearer "+ana); rec.Code != 401 {
		t.Fatalf("users with an app token = %d, want 401", rec.Code)
	}
	list := decode(t, admin("GET", "/v1/admin/users", ""))
	users := list["users"].([]any)
	if list["total"] != float64(2) || len(users) != 2 || users[0].(map[string]any)["email"] != "ben@example.com" {
		t.Fatalf("users = %v", list)
	}
	first := users[1].(map[string]any)
	if first["email"] != "ana@example.com" || first["events"] != float64(3) || first["status"] != "active" {
		t.Errorf("ana = %v", first)
	}
	anaID := fmt.Sprint(int64(first["id"].(float64)))
	if found := decode(t, admin("GET", "/v1/admin/users?q=BEN", ""))["users"].([]any); len(found) != 1 {
		t.Errorf("search found %d", len(found))
	}
	if rec := admin("GET", "/v1/admin/users?status=nope", ""); rec.Code != 400 {
		t.Errorf("bad status filter = %d", rec.Code)
	}

	// One person, and what they did, with names in place of ids.
	one := decode(t, admin("GET", "/v1/admin/users/"+anaID, ""))
	events := one["events"].([]any)
	if len(events) != 3 || one["counts"].(map[string]any)["save"] != float64(1) {
		t.Fatalf("user detail = %v", one)
	}
	titles := map[string]string{}
	for _, e := range events {
		e := e.(map[string]any)
		titles[e["kind"].(string)] = e["title"].(string)
	}
	if titles["save"] != "Low Tide" || titles["pack_save"] != "Quiet gradients" {
		t.Errorf("event titles = %v", titles)
	}
	if rec := admin("GET", "/v1/admin/users/999", ""); rec.Code != 404 {
		t.Errorf("a user that does not exist = %d", rec.Code)
	}

	// What people saved shows on the overview and beside each wallpaper,
	// counted in people.
	over := decode(t, admin("GET", "/v1/admin/overview", ""))
	if u := over["users"].(map[string]any); u["total"] != float64(2) || u["active7"] != float64(2) {
		t.Errorf("overview users = %v", u)
	}
	if top := over["topSaved"].([]any); len(top) != 1 || top[0].(map[string]any)["count"] != float64(2) {
		t.Errorf("most saved = %v", over["topSaved"])
	}
	days := over["days"].([]any)
	if last := days[len(days)-1].(map[string]any); last["newUsers"] != float64(2) || last["activeUsers"] != float64(2) {
		t.Errorf("today = %v", last)
	}
	for _, it := range decode(t, admin("GET", "/v1/admin/wallpapers", ""))["items"].([]any) {
		it := it.(map[string]any)
		want := map[bool][2]float64{true: {2, 1}, false: {0, 0}}[it["id"] == id]
		if it["savedBy"] != want[0] || it["setBy"] != want[1] {
			t.Errorf("%v saved by %v, set by %v", it["title"], it["savedBy"], it["setBy"])
		}
	}

	// A note; then blocking signs the person out and keeps them out.
	if rec := admin("PATCH", "/v1/admin/users/"+anaID, `{"note":"  early tester "}`); rec.Code != 204 {
		t.Fatalf("note = %d %s", rec.Code, rec.Body)
	}
	if rec := admin("PATCH", "/v1/admin/users/"+anaID, `{"status":"blocked"}`); rec.Code != 204 {
		t.Fatalf("block = %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/v1/me", "Authorization", "Bearer "+ana); rec.Code != 401 {
		t.Errorf("a blocked person's token = %d, want 401", rec.Code)
	}
	*now = now.Add(time.Hour)
	before := box.count()
	if rec := f.send("POST", "/v1/auth/start", `{"email":"ana@example.com"}`); rec.Code != 200 || box.count() != before {
		t.Errorf("a blocked person asking for a code = %d, mail sent %d", rec.Code, box.count()-before)
	}
	blocked := decode(t, admin("GET", "/v1/admin/users?status=blocked", ""))
	if got := blocked["users"].([]any); len(got) != 1 || got[0].(map[string]any)["note"] != "early tester" || blocked["blocked"] != float64(1) {
		t.Errorf("blocked = %v", blocked)
	}
	for body, want := range map[string]int{`{"status":"gone"}`: 400, `{}`: 400, `{"note":"` + strings.Repeat("x", 501) + `"}`: 400} {
		if rec := admin("PATCH", "/v1/admin/users/"+anaID, body); rec.Code != want {
			t.Errorf("update %.24s = %d, want %d", body, rec.Code, want)
		}
	}

	csv := admin("GET", "/v1/admin/users.csv", "")
	if csv.Code != 200 || !strings.HasPrefix(csv.Body.String(), "email,joined,last_seen,country,status,events,note\nana@example.com,") ||
		!strings.Contains(csv.Body.String(), "ben@example.com") {
		t.Errorf("csv = %d %s", csv.Code, csv.Body)
	}

	// Removing someone removes what was recorded about them.
	if rec := admin("DELETE", "/v1/admin/users/"+anaID, ""); rec.Code != 204 {
		t.Fatalf("remove = %d", rec.Code)
	}
	if rec := admin("DELETE", "/v1/admin/users/"+anaID, ""); rec.Code != 404 {
		t.Errorf("remove twice = %d, want 404", rec.Code)
	}
	if top := decode(t, admin("GET", "/v1/admin/overview", ""))["topSaved"].([]any); top[0].(map[string]any)["count"] != float64(1) {
		t.Errorf("most saved after removing someone = %v", top)
	}
	// The log says what was done and never names anyone.
	for _, e := range decode(t, admin("GET", "/v1/admin/overview", ""))["events"].([]any) {
		if line := fmt.Sprint(e); strings.Contains(line, "@") {
			t.Errorf("the log names someone: %s", line)
		}
	}
}

func TestReleases(t *testing.T) {
	f, now, _ := withUsers(t)
	cookie := signIn(t, f)
	admin := func(method, target, body string) *httptest.ResponseRecorder {
		return f.send(method, target, body, "Cookie", cookie)
	}
	upload := func(target string, data []byte, headers ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", target, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/octet-stream")
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec
	}

	// Nothing published: the website is told so, in the usual error shape.
	for _, target := range []string{"/v1/releases/latest", "/v1/download/mac", "/v1/releases/files/1/Still.dmg"} {
		if rec := f.do("GET", target); rec.Code != 404 {
			t.Errorf("%s with nothing published = %d, want 404", target, rec.Code)
		}
	}

	for _, c := range []struct {
		body string
		want int
	}{
		{`{"version":"one"}`, 400},
		{`{"version":"1.0"}`, 400},
		{`{"version":"0.1.0","channel":"nightly"}`, 400},
		{`{"version":"v0.1.0","notes":"First beta."}`, 201},
		{`{"version":"0.1.0"}`, 409},
	} {
		if rec := admin("POST", "/v1/admin/releases", c.body); rec.Code != c.want {
			t.Errorf("create %s = %d, want %d", c.body, rec.Code, c.want)
		}
	}
	list := decode(t, admin("GET", "/v1/admin/releases", ""))
	rel := list["releases"].([]any)[0].(map[string]any)
	if rel["version"] != "0.1.0" || rel["channel"] != "beta" || rel["publishedAt"] != nil || rel["latest"] != false {
		t.Fatalf("release = %v", rel)
	}
	base := fmt.Sprintf("/v1/admin/releases/%d", int64(rel["id"].(float64)))

	if rec := admin("PATCH", base, `{"published":true}`); rec.Code != 409 {
		t.Errorf("publishing with no builds = %d, want 409", rec.Code)
	}

	arm, intel := bytes.Repeat([]byte("arm build "), 4000), bytes.Repeat([]byte("intel build "), 3000)
	if rec := upload(base+"/builds?platform=mac-arm64&filename=Still_0.1.0_aarch64.dmg", arm); rec.Code != 401 {
		t.Fatalf("upload signed out = %d, want 401", rec.Code)
	}
	for target, want := range map[string]int{
		base + "/builds?platform=amiga&filename=Still.dmg":                    400,
		base + "/builds?platform=mac-arm64&filename=Still.sh":                 400,
		base + "/builds?platform=mac-arm64&filename=../../etc.dmg":            400,
		base + "/builds?platform=mac-arm64&filename=":                         400,
		"/v1/admin/releases/999/builds?platform=mac-arm64&filename=Still.dmg": 404,
	} {
		if rec := upload(target, arm, "Cookie", cookie); rec.Code != want {
			t.Errorf("upload %s = %d, want %d", target, rec.Code, want)
		}
	}
	if rec := upload(base+"/builds?platform=mac-arm64&filename=Still.dmg", nil, "Cookie", cookie); rec.Code != 400 {
		t.Errorf("empty upload = %d, want 400", rec.Code)
	}
	// A page on another site cannot upload, even with the cookie.
	if rec := upload(base+"/builds?platform=mac-arm64&filename=Still.dmg", arm, "Cookie", cookie, "Origin", "https://evil.example"); rec.Code != 403 {
		t.Errorf("upload from another site = %d, want 403", rec.Code)
	}

	rec := upload(base+"/builds?platform=mac-arm64&filename=old-name.dmg", []byte("an earlier upload"), "Cookie", cookie)
	if rec.Code != 201 {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	// Uploading again for the same system replaces the file.
	rec = upload(base+"/builds?platform=mac-arm64&filename=Still_0.1.0_aarch64.dmg", arm, "Cookie", cookie)
	sum := sha256.Sum256(arm)
	if b := decode(t, rec); rec.Code != 201 || b["sha256"] != hex.EncodeToString(sum[:]) || b["bytes"] != float64(len(arm)) {
		t.Fatalf("second upload = %d %s", rec.Code, rec.Body)
	}
	if rec := upload(base+"/builds?platform=mac-x64&filename=Still_0.1.0_x64.dmg", intel, "Cookie", cookie); rec.Code != 201 {
		t.Fatalf("intel upload = %d %s", rec.Code, rec.Body)
	}
	if files, _ := os.ReadDir(f.srv.ReleasesDir); len(files) != 2 {
		t.Errorf("%d files kept, want 2: the replaced one and any leftovers should be gone", len(files))
	}

	// Still a draft: not offered, and its files cannot be fetched.
	built := decode(t, admin("GET", "/v1/admin/releases", ""))["releases"].([]any)[0].(map[string]any)["builds"].([]any)
	armURL := built[0].(map[string]any)["url"].(string)
	if rec := f.do("GET", "/v1/releases/latest"); rec.Code != 404 {
		t.Errorf("latest while a draft = %d, want 404", rec.Code)
	}
	if rec := f.do("GET", armURL); rec.Code != 404 {
		t.Errorf("a draft's file = %d, want 404", rec.Code)
	}

	if rec := admin("PATCH", base, `{"published":true,"notes":"First public beta."}`); rec.Code != 204 {
		t.Fatalf("publish = %d %s", rec.Code, rec.Body)
	}
	latest := decode(t, f.do("GET", "/v1/releases/latest"))
	builds := latest["builds"].([]any)
	if latest["version"] != "0.1.0" || latest["channel"] != "beta" || latest["notes"] != "First public beta." || len(builds) != 2 {
		t.Fatalf("latest = %v", latest)
	}
	first := builds[0].(map[string]any)
	if first["platform"] != "mac-arm64" || first["label"] != "macOS, Apple silicon" || first["url"] != "/v1/download/mac-arm64" ||
		first["filename"] != "Still_0.1.0_aarch64.dmg" || first["sha256"] != hex.EncodeToString(sum[:]) {
		t.Errorf("first build = %v", first)
	}

	// The download address counts, then sends the browser to the file.
	rec = f.do("GET", "/v1/download/mac-arm64", "CF-Connecting-IP", "203.0.113.5")
	if rec.Code != 302 || rec.Header().Get("Location") != armURL || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("download = %d %v", rec.Code, rec.Header())
	}
	// "mac" picks the build that suits most Macs; there is nothing for Windows.
	if rec := f.do("GET", "/v1/download/mac", "CF-Connecting-IP", "203.0.113.6"); rec.Code != 302 || rec.Header().Get("Location") != armURL {
		t.Errorf("download for any Mac = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := f.do("GET", "/v1/download/windows"); rec.Code != 404 || decode(t, rec)["error"] != "no_build" {
		t.Errorf("download for Windows = %d %s", rec.Code, rec.Body)
	}
	file := f.do("GET", armURL)
	if file.Code != 200 || !bytes.Equal(file.Body.Bytes(), arm) || file.Header().Get("Content-Type") != "application/x-apple-diskimage" ||
		file.Header().Get("Content-Disposition") != `attachment; filename="Still_0.1.0_aarch64.dmg"` {
		t.Errorf("file = %d %v (%d bytes)", file.Code, file.Header(), file.Body.Len())
	}
	if rec := f.do("GET", strings.Replace(armURL, "Still_0.1.0_aarch64", "other", 1)); rec.Code != 404 {
		t.Errorf("a file under another name = %d, want 404", rec.Code)
	}

	// The same connection fetching over and over is counted up to a limit.
	for range downloadsCountedPerIPPerHour + 5 {
		f.do("GET", "/v1/download/mac-x64", "CF-Connecting-IP", "198.51.100.1")
	}
	rel = decode(t, admin("GET", "/v1/admin/releases", ""))["releases"].([]any)[0].(map[string]any)
	if rel["latest"] != true || rel["publishedAt"] == nil {
		t.Errorf("published release = %v", rel)
	}
	counts := map[string]float64{}
	for _, b := range rel["builds"].([]any) {
		b := b.(map[string]any)
		counts[b["platform"].(string)] = b["downloadsAll"].(float64)
	}
	if counts["mac-arm64"] != 2 || counts["mac-x64"] != downloadsCountedPerIPPerHour {
		t.Errorf("download counts = %v", counts)
	}
	days := decode(t, admin("GET", "/v1/admin/overview", ""))["days"].([]any)
	if today := days[len(days)-1].(map[string]any); today["downloads"] != float64(2+downloadsCountedPerIPPerHour) {
		t.Errorf("downloads today = %v", today["downloads"])
	}

	// A newer release takes over as soon as it is published.
	*now = now.Add(24 * time.Hour)
	admin("POST", "/v1/admin/releases", `{"version":"0.2.0"}`)
	next := decode(t, admin("GET", "/v1/admin/releases", ""))["releases"].([]any)[0].(map[string]any)
	nextBase := fmt.Sprintf("/v1/admin/releases/%d", int64(next["id"].(float64)))
	upload(nextBase+"/builds?platform=mac-universal&filename=Still_0.2.0_universal.dmg", []byte("universal"), "Cookie", cookie)
	admin("PATCH", nextBase, `{"published":true}`)
	if latest := decode(t, f.do("GET", "/v1/releases/latest")); latest["version"] != "0.2.0" {
		t.Errorf("latest after a newer release = %v", latest["version"])
	}
	// Taking it back puts the earlier one on offer again.
	admin("PATCH", nextBase, `{"published":false}`)
	if latest := decode(t, f.do("GET", "/v1/releases/latest")); latest["version"] != "0.1.0" {
		t.Errorf("latest after taking the newer one back = %v", latest["version"])
	}

	// Removing a build, then the release, removes the files.
	armID := fmt.Sprint(int64(built[0].(map[string]any)["id"].(float64)))
	if rec := admin("DELETE", "/v1/admin/builds/"+armID, ""); rec.Code != 204 {
		t.Fatalf("remove a build = %d", rec.Code)
	}
	if rec := f.do("GET", armURL); rec.Code != 404 {
		t.Errorf("a removed build's file = %d, want 404", rec.Code)
	}
	if rec := admin("DELETE", base, ""); rec.Code != 204 {
		t.Fatalf("remove a release = %d", rec.Code)
	}
	if rec := admin("DELETE", nextBase, ""); rec.Code != 204 {
		t.Fatalf("remove the other release = %d", rec.Code)
	}
	if files, _ := os.ReadDir(f.srv.ReleasesDir); len(files) != 0 {
		t.Errorf("%d files left after removing every release", len(files))
	}
	if rec := f.do("GET", "/v1/releases/latest"); rec.Code != 404 {
		t.Errorf("latest with everything removed = %d, want 404", rec.Code)
	}
}
