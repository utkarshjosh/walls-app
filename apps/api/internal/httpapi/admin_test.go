package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

const testPassword = "correct horse battery"

// withRecords gives the fixture a records file, an admin password and a clock
// the test controls.
func withRecords(t *testing.T) (*fixture, *time.Time) {
	t.Helper()
	f := newFixture(t)
	rec, err := records.Open(filepath.Join(t.TempDir(), "records.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rec.Close() })
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	f.srv.Records, f.srv.AdminPassword, f.srv.StorageCap = rec, testPassword, 7_000_000_000
	f.srv.Now = func() time.Time { return now }
	f.handler = f.srv.Handler()
	return f, &now
}

// send makes a request with a JSON body (or none, when body is "").
func (f *fixture) send(method, target, body string, headers ...string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func signIn(t *testing.T, f *fixture) string {
	t.Helper()
	rec := f.send("POST", "/v1/admin/login", `{"password":"`+testPassword+`"}`)
	if rec.Code != 200 {
		t.Fatalf("sign-in = %d %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("sign-in set %d cookies", len(cookies))
	}
	c := cookies[0]
	if c.Name != sessionCookie || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/v1/admin" || !c.Secure || c.MaxAge <= 0 {
		t.Errorf("session cookie = %+v", c)
	}
	return c.Name + "=" + c.Value
}

func TestSignup(t *testing.T) {
	f, _ := withRecords(t)

	rec := f.send("POST", "/v1/signups", `{"email":"  Ana@Example.com "}`, "CF-IPCountry", "in")
	if rec.Code != 200 || decode(t, rec)["ok"] != true || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("sign-up = %d %s", rec.Code, rec.Body)
	}
	// The same address again gets the same answer and adds nothing.
	if again := f.send("POST", "/v1/signups", `{"email":"ana@example.com"}`); again.Code != 200 || again.Body.String() != rec.Body.String() {
		t.Errorf("repeat sign-up = %d %s", again.Code, again.Body)
	}
	// A script that fills in the hidden field is told it worked and is not stored.
	if bot := f.send("POST", "/v1/signups", `{"email":"bot@example.com","website":"http://spam.example"}`); bot.Code != 200 {
		t.Errorf("honeypot = %d", bot.Code)
	}
	list, _ := f.srv.Records.ListSignups(t.Context(), records.SignupQuery{})
	if len(list) != 1 || list[0].Email != "ana@example.com" || list[0].Country != "IN" || list[0].Source != records.SourceSite {
		t.Fatalf("stored = %+v", list)
	}

	for body, want := range map[string]int{
		`{"email":"not an address"}`: 400,
		`{"email":""}`:               400,
		`{}`:                         400,
		`not json`:                   400,
		`{"email":"` + strings.Repeat("a", 5000) + `@example.com"}`: 413,
	} {
		if rec := f.send("POST", "/v1/signups", body); rec.Code != want {
			t.Errorf("sign-up %.40s = %d, want %d", body, rec.Code, want)
		}
	}
	// A form on another site cannot post here: it cannot send JSON.
	req := httptest.NewRequest("POST", "/v1/signups", strings.NewReader("email=x@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out := httptest.NewRecorder()
	f.handler.ServeHTTP(out, req)
	if out.Code != 415 {
		t.Errorf("form post = %d, want 415", out.Code)
	}
}

func TestSignupLimits(t *testing.T) {
	f, now := withRecords(t)
	post := func(email, ip string) int {
		return f.send("POST", "/v1/signups", `{"email":"`+email+`"}`, "CF-Connecting-IP", ip).Code
	}
	for i := range signupsPerIPPerHour {
		if code := post("p"+string(rune('a'+i))+"@example.com", "203.0.113.5"); code != 200 {
			t.Fatalf("sign-up %d from one connection = %d", i+1, code)
		}
	}
	if code := post("extra@example.com", "203.0.113.5"); code != 429 {
		t.Errorf("past the per-connection allowance = %d, want 429", code)
	}
	if code := post("other@example.com", "203.0.113.6"); code != 200 {
		t.Errorf("another connection = %d, want 200", code)
	}
	*now = now.Add(time.Hour)
	if code := post("later@example.com", "203.0.113.5"); code != 200 {
		t.Errorf("an hour later = %d, want 200", code)
	}

	// The total across everyone holds even when every request claims a new address.
	g, _ := withRecords(t)
	for i := range signupsPerHour {
		ip := "198.51.100." + string(rune('0'+i%10)) + string(rune('a'+i/10%26)) + string(rune('a'+i/260))
		if rec := g.send("POST", "/v1/signups", `{"email":"u`+ip[11:]+`@example.com"}`, "CF-Connecting-IP", ip); rec.Code != 200 {
			t.Fatalf("sign-up %d = %d %s", i+1, rec.Code, rec.Body)
		}
	}
	if rec := g.send("POST", "/v1/signups", `{"email":"last@example.com"}`, "CF-Connecting-IP", "192.0.2.99"); rec.Code != 503 {
		t.Errorf("past the hourly total = %d, want 503", rec.Code)
	}
}

func TestSignupsOffWithoutRecords(t *testing.T) {
	f := newFixture(t)
	if rec := f.send("POST", "/v1/signups", `{"email":"ana@example.com"}`); rec.Code != 503 {
		t.Errorf("sign-up with no records file = %d, want 503", rec.Code)
	}
	if rec := f.send("POST", "/v1/visits", ""); rec.Code != 204 {
		t.Errorf("visit with no records file = %d, want 204", rec.Code)
	}
	if rec := f.do("GET", "/v1/packs"); rec.Code != 200 {
		t.Errorf("catalog with no records file = %d", rec.Code)
	}
}

func TestCounting(t *testing.T) {
	f, _ := withRecords(t)
	id := f.ids["Low Tide"]
	video := f.ids["Slow Waves"]

	f.send("POST", "/v1/visits", "")
	f.send("POST", "/v1/visits", "")
	f.do("GET", "/v1/packs")
	f.do("GET", "/v1/packs/quiet-gradients")
	f.do("GET", "/v1/packs/no-such-pack")
	f.do("GET", "/v1/files/"+id+"/original")
	f.do("GET", "/v1/files/"+id+"/original")
	f.do("GET", "/v1/files/"+id+"/thumb")                              // a thumbnail is not a fetch
	f.do("HEAD", "/v1/files/"+id+"/original")                          // nor is asking about the file
	f.do("GET", "/v1/files/"+video+"/original", "Range", "bytes=0-99") // the start of a video is
	f.do("GET", "/v1/files/"+video+"/original", "Range", "bytes=100-") // a later part of it is not

	cookie := signIn(t, f)
	rec := f.send("GET", "/v1/admin/overview", "", "Cookie", cookie)
	if rec.Code != 200 {
		t.Fatalf("overview = %d %s", rec.Code, rec.Body)
	}
	var overview struct {
		Days []dayJSON `json:"days"`
		Top  []topJSON `json:"topWallpapers"`
	}
	json.Unmarshal(rec.Body.Bytes(), &overview)
	if len(overview.Days) != activityDays || overview.Days[0].Day != "2026-10-12" {
		t.Fatalf("days = %d starting %s", len(overview.Days), overview.Days[0].Day)
	}
	if today := overview.Days[activityDays-1]; today != (dayJSON{Day: "2026-11-10", Visits: 2, Discover: 1, Files: 3}) {
		t.Errorf("today = %+v", today)
	}
	if len(overview.Top) != 2 || overview.Top[0].ID != id || overview.Top[0].Count != 2 || overview.Top[0].Title != "Low Tide" {
		t.Errorf("top = %+v", overview.Top)
	}

	rec = f.send("GET", "/v1/admin/wallpapers", "", "Cookie", cookie)
	var walls struct {
		Items []adminItemJSON `json:"items"`
		Packs []adminPackJSON `json:"packs"`
		Cap   int64           `json:"capBytes"`
		Used  int64           `json:"storedBytes"`
	}
	json.Unmarshal(rec.Body.Bytes(), &walls)
	if rec.Code != 200 || len(walls.Items) != 4 || walls.Cap != 7_000_000_000 || walls.Used <= 0 {
		t.Fatalf("wallpapers = %d, %d items, cap %d, used %d", rec.Code, len(walls.Items), walls.Cap, walls.Used)
	}
	for _, it := range walls.Items {
		want := map[string]int64{id: 2, video: 1}[it.ID]
		if it.Week != want || it.Month != want || it.AllTime != want {
			t.Errorf("%s fetched %d/%d/%d, want %d", it.Title, it.Week, it.Month, it.AllTime, want)
		}
	}
	if len(walls.Packs) != 2 || walls.Packs[0].ID != "quiet-gradients" || walls.Packs[0].Month != 1 || walls.Packs[1].Month != 0 {
		t.Errorf("packs = %+v", walls.Packs)
	}
}

func TestAdminNeedsSignIn(t *testing.T) {
	f, now := withRecords(t)

	if s := decode(t, f.do("GET", "/v1/admin/session")); s["enabled"] != true || s["signedIn"] != false {
		t.Errorf("session before sign-in = %v", s)
	}
	routes := [][2]string{
		{"GET", "/v1/admin/overview"}, {"GET", "/v1/admin/signups"}, {"GET", "/v1/admin/signups.csv"},
		{"GET", "/v1/admin/wallpapers"}, {"POST", "/v1/admin/signups"}, {"POST", "/v1/admin/signups/status"},
		{"PATCH", "/v1/admin/signups/1"}, {"DELETE", "/v1/admin/signups/1"}, {"POST", "/v1/admin/logout"},
	}
	for _, route := range routes {
		for name, cookie := range map[string]string{"no cookie": "", "made-up cookie": sessionCookie + "=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
			rec := f.send(route[0], route[1], `{}`, "Cookie", cookie)
			if rec.Code != 401 || decode(t, rec)["error"] != "signed_out" {
				t.Errorf("%s %s with %s = %d %s", route[0], route[1], name, rec.Code, rec.Body)
			}
		}
	}

	if rec := f.send("POST", "/v1/admin/login", `{"password":"wrong"}`); rec.Code != 401 || len(rec.Result().Cookies()) != 0 {
		t.Errorf("wrong password = %d, cookies %v", rec.Code, rec.Result().Cookies())
	}
	cookie := signIn(t, f)
	if s := decode(t, f.send("GET", "/v1/admin/session", "", "Cookie", cookie)); s["signedIn"] != true {
		t.Errorf("session after sign-in = %v", s)
	}
	if rec := f.send("GET", "/v1/admin/signups", "", "Cookie", cookie); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("signed in = %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}

	// A change must come from the admin page itself, even with the cookie.
	for _, h := range [][2]string{{"Origin", "https://evil.example"}, {"Sec-Fetch-Site", "cross-site"}, {"Sec-Fetch-Site", "same-site"}} {
		rec := f.send("POST", "/v1/admin/signups", `{"email":"x@example.com"}`, "Cookie", cookie, h[0], h[1])
		if rec.Code != 403 {
			t.Errorf("change with %s: %s = %d, want 403", h[0], h[1], rec.Code)
		}
	}
	if rec := f.send("POST", "/v1/admin/signups", `{"email":"x@example.com"}`, "Cookie", cookie,
		"Origin", "http://example.com", "Sec-Fetch-Site", "same-origin"); rec.Code != 201 {
		t.Errorf("change from the page itself = %d %s", rec.Code, rec.Body)
	}
	if rec := f.send("POST", "/v1/admin/login", `{"password":"`+testPassword+`"}`, "Origin", "https://evil.example"); rec.Code != 403 {
		t.Errorf("sign-in from another site = %d, want 403", rec.Code)
	}

	// Changing the password signs every browser out.
	f.srv.AdminPassword = "a different password"
	if rec := f.send("GET", "/v1/admin/signups", "", "Cookie", cookie); rec.Code != 401 {
		t.Errorf("old session after a password change = %d, want 401", rec.Code)
	}
	f.srv.AdminPassword = testPassword

	// Signing out ends the session on the server, not just in the browser.
	if rec := f.send("POST", "/v1/admin/logout", "", "Cookie", cookie); rec.Code != 204 {
		t.Fatalf("sign-out = %d", rec.Code)
	}
	if rec := f.send("GET", "/v1/admin/signups", "", "Cookie", cookie); rec.Code != 401 {
		t.Errorf("after sign-out = %d, want 401", rec.Code)
	}

	// And a session runs out by itself.
	cookie = signIn(t, f)
	*now = now.Add(sessionTTL + time.Minute)
	if rec := f.send("GET", "/v1/admin/signups", "", "Cookie", cookie); rec.Code != 401 {
		t.Errorf("after the session ran out = %d, want 401", rec.Code)
	}
}

func TestAdminOffWithoutPassword(t *testing.T) {
	f, _ := withRecords(t)
	f.srv.AdminPassword = ""
	if s := decode(t, f.do("GET", "/v1/admin/session")); s["enabled"] != false || s["signedIn"] != false {
		t.Errorf("session = %v", s)
	}
	// An empty password must never sign anyone in.
	for _, body := range []string{`{"password":""}`, `{}`} {
		if rec := f.send("POST", "/v1/admin/login", body); rec.Code != 404 || len(rec.Result().Cookies()) != 0 {
			t.Errorf("sign-in %s with the portal off = %d", body, rec.Code)
		}
	}
	if rec := f.do("GET", "/v1/admin/signups"); rec.Code != 404 {
		t.Errorf("list with the portal off = %d", rec.Code)
	}
	// Sign-ups still work: the list is collected whether or not anyone can read it yet.
	if rec := f.send("POST", "/v1/signups", `{"email":"ana@example.com"}`); rec.Code != 200 {
		t.Errorf("sign-up with the portal off = %d", rec.Code)
	}
}

func TestLoginLockout(t *testing.T) {
	f, now := withRecords(t)
	try := func(password, ip string) int {
		return f.send("POST", "/v1/admin/login", `{"password":"`+password+`"}`, "CF-Connecting-IP", ip).Code
	}
	for i := range loginFailuresPerIP {
		if code := try("guess", "203.0.113.9"); code != 401 {
			t.Fatalf("wrong password %d = %d", i+1, code)
		}
	}
	// Locked: even the right password is refused from that connection for now.
	if code := try(testPassword, "203.0.113.9"); code != 429 {
		t.Errorf("right password while locked = %d, want 429", code)
	}
	if code := try(testPassword, "203.0.113.10"); code != 200 {
		t.Errorf("right password from elsewhere = %d, want 200", code)
	}
	*now = now.Add(loginWindow)
	if code := try(testPassword, "203.0.113.9"); code != 200 {
		t.Errorf("after the wait = %d, want 200", code)
	}
}

func TestAdminPeople(t *testing.T) {
	f, now := withRecords(t)
	cookie := signIn(t, f)
	admin := func(method, target, body string) *httptest.ResponseRecorder {
		return f.send(method, target, body, "Cookie", cookie)
	}

	f.send("POST", "/v1/signups", `{"email":"ana@example.com"}`, "CF-IPCountry", "IN")
	*now = now.Add(time.Hour)
	f.send("POST", "/v1/signups", `{"email":"=cmd@example.com"}`)
	if rec := admin("POST", "/v1/admin/signups", `{"email":"Friend@Example.com","note":"from college"}`); rec.Code != 201 {
		t.Fatalf("add by hand = %d %s", rec.Code, rec.Body)
	}
	if rec := admin("POST", "/v1/admin/signups", `{"email":"friend@example.com"}`); rec.Code != 409 {
		t.Errorf("add twice = %d, want 409", rec.Code)
	}
	if rec := admin("POST", "/v1/admin/signups", `{"email":"nope"}`); rec.Code != 400 {
		t.Errorf("add a bad address = %d, want 400", rec.Code)
	}

	type listing struct {
		People  []personJSON `json:"people"`
		Total   int          `json:"total"`
		Invited int          `json:"invited"`
	}
	list := func(query string) listing {
		t.Helper()
		rec := admin("GET", "/v1/admin/signups"+query, "")
		if rec.Code != 200 {
			t.Fatalf("list%s = %d %s", query, rec.Code, rec.Body)
		}
		var out listing
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	all := list("")
	if all.Total != 3 || len(all.People) != 3 || all.People[0].Email != "friend@example.com" ||
		all.People[0].Source != "manual" || all.People[0].Note != "from college" || all.People[2].Country != "IN" {
		t.Fatalf("list = %+v", all)
	}
	friend, ana := all.People[0].ID, all.People[2].ID

	// Mark two as sent the build.
	rec := admin("POST", "/v1/admin/signups/status", `{"ids":[`+itoa(friend)+`,`+itoa(ana)+`],"status":"invited"}`)
	if rec.Code != 200 || decode(t, rec)["changed"] != float64(2) {
		t.Fatalf("mark = %d %s", rec.Code, rec.Body)
	}
	if got := list("?status=invited"); len(got.People) != 2 || got.Invited != 2 || got.People[0].InvitedAt == nil {
		t.Errorf("sent = %+v", got)
	}
	if got := list("?status=new"); len(got.People) != 1 || got.People[0].Email != "=cmd@example.com" {
		t.Errorf("not sent = %+v", got)
	}
	if got := list("?q=COLLEGE"); len(got.People) != 1 {
		t.Errorf("search = %+v", got)
	}
	for _, bad := range []string{`{"ids":[],"status":"invited"}`, `{"ids":[1],"status":"banned"}`} {
		if rec := admin("POST", "/v1/admin/signups/status", bad); rec.Code != 400 {
			t.Errorf("mark %s = %d, want 400", bad, rec.Code)
		}
	}
	if rec := admin("GET", "/v1/admin/signups?status=banned", ""); rec.Code != 400 {
		t.Errorf("list with a bad status = %d", rec.Code)
	}

	if rec := admin("PATCH", "/v1/admin/signups/"+itoa(ana), `{"note":"  @home  "}`); rec.Code != 200 || decode(t, rec)["note"] != "@home" {
		t.Errorf("note = %d %s", rec.Code, rec.Body)
	}
	if rec := admin("PATCH", "/v1/admin/signups/"+itoa(ana), `{"note":"`+strings.Repeat("x", maxNoteLength+1)+`"}`); rec.Code != 400 {
		t.Errorf("long note = %d, want 400", rec.Code)
	}
	if rec := admin("PATCH", "/v1/admin/signups/99999", `{"note":"x"}`); rec.Code != 404 {
		t.Errorf("note on nobody = %d, want 404", rec.Code)
	}

	// The spreadsheet: oldest first, and nothing a spreadsheet would run as a formula.
	rec = admin("GET", "/v1/admin/signups.csv", "")
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		rec.Header().Get("Content-Disposition") != `attachment; filename="signups-2026-11-10.csv"` {
		t.Fatalf("csv = %d %v", rec.Code, rec.Header())
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	want := []string{
		"email,signed_up,source,country,status,sent,note",
		"ana@example.com,2026-11-10T12:00:00Z,site,IN,invited,2026-11-10T13:00:00Z,'@home",
		"'=cmd@example.com,2026-11-10T13:00:00Z,site,,new,,",
		"friend@example.com,2026-11-10T13:00:00Z,manual,,invited,2026-11-10T13:00:00Z,from college",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("csv =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}

	if rec := admin("DELETE", "/v1/admin/signups/"+itoa(ana), ""); rec.Code != 204 {
		t.Fatalf("remove = %d", rec.Code)
	}
	if rec := admin("DELETE", "/v1/admin/signups/"+itoa(ana), ""); rec.Code != 404 {
		t.Errorf("remove twice = %d, want 404", rec.Code)
	}
	if got := list(""); got.Total != 2 {
		t.Errorf("after removing, total = %d", got.Total)
	}

	// The log says what was done, and never whose address it was.
	var overview struct {
		Events []eventJSON `json:"events"`
	}
	json.Unmarshal(admin("GET", "/v1/admin/overview", "").Body.Bytes(), &overview)
	if len(overview.Events) < 5 || overview.Events[0].Action != "Removed a person" {
		t.Errorf("log = %+v", overview.Events)
	}
	for _, e := range overview.Events {
		if strings.Contains(e.Action+e.Detail, "@") {
			t.Errorf("the log holds an address: %+v", e)
		}
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestAdminPages(t *testing.T) {
	f := newFixture(t) // the pages are served even with the portal off: they explain how to turn it on
	for path, ctype := range map[string]string{
		"/admin": "text/html", "/admin/": "text/html", "/admin/admin.css": "text/css", "/admin/admin.js": "text/javascript",
	} {
		rec := f.do("GET", path)
		h := rec.Header()
		if rec.Code != 200 || !strings.HasPrefix(h.Get("Content-Type"), ctype) || rec.Body.Len() < 500 {
			t.Errorf("%s = %d %s (%d bytes)", path, rec.Code, h.Get("Content-Type"), rec.Body.Len())
		}
		csp := h.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") || strings.Contains(csp, "unsafe") {
			t.Errorf("%s policy = %q", path, csp)
		}
		if h.Get("X-Robots-Tag") == "" {
			t.Errorf("%s may be indexed by search engines", path)
		}
	}
	for _, path := range []string{"/admin/records.db", "/admin/../server.go", "/admin/adminui/index.html"} {
		if rec := f.do("GET", path); rec.Code == 200 {
			t.Errorf("%s was served", path)
		}
	}
	// The page has no inline script or style, which its policy would block.
	page := f.do("GET", "/admin").Body.String()
	if strings.Contains(page, "<style") || strings.Contains(page, "style=") || strings.Contains(page, "onclick") {
		t.Error("the admin page has inline style or script")
	}
}
