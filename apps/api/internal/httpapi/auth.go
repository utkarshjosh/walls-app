package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/utkarshjoshi/walls/apps/api/internal/mail"
	"github.com/utkarshjoshi/walls/apps/api/internal/records"
)

// Signing in to the app, with no password.
//
// A person types their address; the server emails a six-digit code; they type
// the code back and the app gets a token. The first time an address does
// this it becomes a user, so signing up and signing in are the same thing.
//
// The app keeps the token and sends it as "Authorization: Bearer <token>". It
// is not a cookie, so a page on another site cannot make a signed-in request.
// The server stores only a hash of each code and each token.

const (
	// productName is what the email calls the app. It is a working name, so it
	// lives in one place.
	productName = "Still"

	codeTTL      = 10 * time.Minute
	codeAttempts = 5 // wrong guesses allowed against one code

	userSessionTTL = 180 * 24 * time.Hour
	// A sign-in in use is noted, and its end moved on, at most this often.
	sessionTouchEvery = time.Hour

	// How many codes may be asked for. Each one is an email, which costs
	// money and can be used to pester someone, so all three are low.
	codeResendWait       = 45 * time.Second
	codesPerEmailPerHour = 5
	codesPerIPPerHour    = 12
	codesPerHour         = 200 // across everyone

	verifiesPerIP = 40 // per fifteen minutes, right or wrong

	// What a signed-in app may report.
	eventPostsPerUserPerHour = 240
	maxEventsPerPost         = 50
	maxEventAge              = 7 * 24 * time.Hour // an app that was offline may report late
	maxEventSubject          = 64
	maxEventDetail           = 200
	maxClientLength          = 80
)

// SignInEnabled reports whether people can sign in: there must be somewhere
// to keep them and a way to send the code.
func (s *Server) SignInEnabled() bool { return s.Records != nil && s.Mail != nil }

func signInOff(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "signin_off", "Signing in is not available on this server.")
}

func hashOf(purpose string, parts ...string) string {
	sum := sha256.Sum256([]byte(purpose + "\n" + strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func codeHash(email, code string) string { return hashOf("walls sign-in code", email, code) }
func userTokenHash(token string) string  { return hashOf("walls user session", token) }

// newCode returns six random digits, every value equally likely.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func tooMany(w http.ResponseWriter, wait time.Duration, message string) {
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
	writeError(w, http.StatusTooManyRequests, "too_many", message)
}

// handleAuthStatus tells the app whether to offer signing in at all.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"signIn": s.SignInEnabled()})
}

// handleAuthStart emails a sign-in code to an address. The answer is the same
// whether or not the address has signed in before.
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.SignInEnabled() {
		signInOff(w)
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body, 4<<10) {
		return
	}
	email, ok := records.NormalizeEmail(body.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_email", "That does not look like an email address.")
		return
	}
	now := s.now()
	if !s.codeByIP.allow(clientKey(r), now) {
		tooMany(w, time.Hour, "Too many codes asked for from this connection. Try again in an hour.")
		return
	}
	if s.codeWait.full(email, now) {
		tooMany(w, codeResendWait, "A code was sent a moment ago. Check your mail, or wait a minute and ask again.")
		return
	}
	if s.codeByEmail.full(email, now) {
		tooMany(w, time.Hour, "Too many codes asked for this address. Try again in an hour.")
		return
	}
	if !s.codeAll.allow("all", now) {
		s.Log.Warn("sign-in paused: the hourly total of codes was reached")
		w.Header().Set("Retry-After", "3600")
		writeError(w, http.StatusServiceUnavailable, "busy", "Sign-in is busy right now. Try again in a little while.")
		return
	}
	s.codeWait.add(email, now)
	s.codeByEmail.add(email, now)

	done := func() {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "expiresInSeconds": int(codeTTL.Seconds())})
	}
	// Someone the owner has blocked is told the same thing and sent nothing.
	if u, err := s.Records.UserByEmail(r.Context(), email); err == nil && u.Status == records.UserBlocked {
		done()
		return
	} else if err != nil && !errors.Is(err, records.ErrNotFound) {
		s.fail(w, r, err)
		return
	}

	code, err := newCode()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Records.SaveLoginCode(r.Context(), email, codeHash(email, code), now, now.Add(codeTTL)); err != nil {
		s.fail(w, r, err)
		return
	}
	// Finish sending even if the person closes the window while waiting.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 25*time.Second)
	defer cancel()
	err = s.Mail.Send(ctx, mail.Message{
		To:      email,
		Subject: fmt.Sprintf("%s is your %s sign-in code", code, productName),
		Text: fmt.Sprintf("Your sign-in code for %s is\n\n    %s\n\n"+
			"It works once, for the next %d minutes. Type it into the app to finish signing in.\n\n"+
			"If you did not ask for this, ignore it: nobody can sign in without the code.\n",
			productName, code, int(codeTTL.Minutes())),
	})
	if err != nil {
		// The relay's answer can repeat the address; keep it out of the log.
		s.Log.Error("sign-in code not sent", "err", strings.ReplaceAll(err.Error(), email, "<address>"))
		writeError(w, http.StatusBadGateway, "mail_failed", "The code could not be sent. Try again in a moment.")
		return
	}
	done()
}

var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

type accountJSON struct {
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"`
}

func account(u records.User) accountJSON {
	return accountJSON{Email: u.Email, CreatedAt: iso(u.CreatedAt)}
}

// tidy keeps the printable characters of s, at most max of them.
func tidy(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if n == max {
			break
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// handleAuthVerify trades a code for a token.
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.SignInEnabled() {
		signInOff(w)
		return
	}
	var body struct {
		Email  string `json:"email"`
		Code   string `json:"code"`
		Client string `json:"client"` // what is signing in, e.g. "Still 0.1.0 macOS"; shown to the owner
	}
	if !readJSON(w, r, &body, 4<<10) {
		return
	}
	email, ok := records.NormalizeEmail(body.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_email", "That does not look like an email address.")
		return
	}
	// People copy codes with the space or dash a mail program showed them.
	code := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(body.Code))
	if !sixDigits.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid_code", "Type the six digits from the email.")
		return
	}
	now := s.now()
	if !s.verifyByIP.allow(clientKey(r), now) {
		tooMany(w, loginWindow, "Too many tries from this connection. Wait fifteen minutes and try again.")
		return
	}
	ctx := r.Context()
	good, err := s.Records.UseLoginCode(ctx, email, codeHash(email, code), now, codeAttempts)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !good {
		writeError(w, http.StatusUnauthorized, "wrong_code", "That code is not right, or it has run out. Ask for a new one.")
		return
	}
	user, _, err := s.Records.SignInUser(ctx, email, country(r), now)
	if errors.Is(err, records.ErrFull) {
		writeError(w, http.StatusServiceUnavailable, "busy", "New sign-ins are closed for the moment. Try again later.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if user.Status == records.UserBlocked {
		writeError(w, http.StatusForbidden, "blocked", "This account cannot sign in.")
		return
	}
	token, err := newToken()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	expires := now.Add(userSessionTTL)
	if err := s.Records.CreateUserSession(ctx, userTokenHash(token), user.ID, tidy(body.Client, maxClientLength), now, expires); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expiresAt": iso(expires), "user": account(user)})
}

// bearer returns the token a request carries, or "".
func bearer(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || len(token) > 128 {
		return ""
	}
	return token
}

// withUser wraps a handler that needs a signed-in user.
func (s *Server) withUser(next func(http.ResponseWriter, *http.Request, records.User, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.Records == nil {
			signInOff(w)
			return
		}
		token := bearer(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "signed_out", "Sign in to continue.")
			return
		}
		hash, now := userTokenHash(token), s.now()
		user, used, err := s.Records.UserBySession(r.Context(), hash, now)
		if errors.Is(err, records.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "signed_out", "Sign in to continue.")
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if user.Status == records.UserBlocked {
			writeError(w, http.StatusForbidden, "blocked", "This account cannot sign in.")
			return
		}
		if now.Sub(used) >= sessionTouchEvery {
			if err := s.Records.TouchUserSession(r.Context(), hash, user.ID, now, now.Add(userSessionTTL)); err != nil {
				s.Log.Warn("sign-in not refreshed", "err", err.Error())
			}
		}
		next(w, r, user, hash)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, user records.User, _ string) {
	writeJSON(w, http.StatusOK, account(user))
}

func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request, _ records.User, hash string) {
	if err := s.Records.DeleteUserSession(r.Context(), hash); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteMe removes the person's account and everything recorded about
// what they did. They asked; there is nothing to confirm on this side.
func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request, user records.User, _ string) {
	if err := s.Records.DeleteUser(r.Context(), user.ID); err != nil && !errors.Is(err, records.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var eventSubject = regexp.MustCompile(`^[A-Za-z0-9._-]*$`)

// handleEvents records what a signed-in person did in the app: which
// wallpapers they looked at, saved and set. This is what suggestions will be
// made from. Kinds this server does not know are dropped rather than refused,
// so a newer app can talk to an older server.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, user records.User, _ string) {
	var body struct {
		Events []struct {
			Kind    string `json:"kind"`
			Subject string `json:"subject"`
			Detail  string `json:"detail"`
			At      string `json:"at"`
		} `json:"events"`
	}
	if !readJSON(w, r, &body, 64<<10) {
		return
	}
	if len(body.Events) == 0 || len(body.Events) > maxEventsPerPost {
		writeError(w, http.StatusBadRequest, "invalid_events", fmt.Sprintf("Send between 1 and %d events.", maxEventsPerPost))
		return
	}
	now := s.now()
	if !s.eventsByUser.allow(strconv.FormatInt(user.ID, 10), now) {
		tooMany(w, time.Hour, "Too many reports from this account. Try again in an hour.")
		return
	}
	keep := make([]records.UserEvent, 0, len(body.Events))
	for _, e := range body.Events {
		if !records.EventKinds[e.Kind] || len(e.Subject) > maxEventSubject || !eventSubject.MatchString(e.Subject) ||
			(e.Subject == "" && records.EventNeedsSubject[e.Kind]) {
			continue
		}
		at := now
		// The app's own clock is believed only within reason.
		if t, err := time.Parse(time.RFC3339, e.At); err == nil && !t.After(now.Add(5*time.Minute)) && now.Sub(t) <= maxEventAge {
			at = t
		}
		keep = append(keep, records.UserEvent{At: at, Kind: e.Kind, Subject: e.Subject, Detail: tidy(e.Detail, maxEventDetail)})
	}
	if err := s.Records.AddUserEvents(r.Context(), user.ID, keep); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"accepted": len(keep)})
}
