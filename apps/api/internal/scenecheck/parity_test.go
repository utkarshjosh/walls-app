package scenecheck

import (
	"strings"
	"testing"
)

// The Mac app re-checks the policy line when a scene arrives
// (apps/desktop/src-tauri/src/scene.rs). The gate must never pass a scene
// the app would then refuse, so this test runs the app's own test cases
// through the gate and expects the same verdict for each.

// appVerdict reduces the gate's policy findings to the single answer the app
// gives: it stops at the first of these that applies.
func appVerdict(html string) string {
	seen := map[string]bool{}
	for _, p := range CheckHTML([]byte(html)) {
		seen[p.Code] = true
	}
	for _, code := range []string{"policy-missing", "policy-multiple", "policy-wrong", "policy-order", "policy-place"} {
		if seen[code] {
			return code
		}
	}
	return "ok"
}

// appScene is a scene laid out the way the app's tests lay one out.
func appScene(policy string) string {
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n" + policy + "\n" +
		"<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n<title>Quiet Clock</title>\n" +
		"<!-- A live wallpaper scene. -->\n<style>html,body{margin:0;background:#1c2230}</style>\n</head>\n" +
		"<body>\n<canvas id=\"c\"></canvas>\n<script>\n(function () { var h = location.hash; })();\n</script>\n</body>\n</html>\n"
}

func TestPolicyVerdictsMatchTheMacApp(t *testing.T) {
	const (
		ok       = "ok"
		none     = "policy-missing"  // NoPolicy
		several  = "policy-multiple" // SeveralPolicies
		wrong    = "policy-wrong"    // WrongPolicy
		script   = "policy-order"    // PolicyAfterScript
		notFirst = "policy-place"    // PolicyNotFirst
	)
	tag := func(content string) string {
		return `<meta http-equiv="Content-Security-Policy" content="` + content + `">`
	}
	cases := []struct {
		name, html, want string
	}{
		// the_exact_line_passes
		{"exact line", appScene(PolicyLine), ok},
		{"barest file", PolicyLine + "<script>draw()</script>", ok},
		{"byte-order mark", "\xEF\xBB\xBF" + appScene(PolicyLine), ok},
		// attribute_order_quotes_and_case_do_not_matter
		{"swapped", appScene(`<meta content="` + Policy + `" http-equiv="Content-Security-Policy">`), ok},
		{"mixed quotes", appScene(`<meta http-equiv='Content-Security-Policy' content="` + Policy + `">`), ok},
		{"unquoted", appScene(`<meta http-equiv=Content-Security-Policy content="` + Policy + `">`), ok},
		{"shouting", appScene(`<META HTTP-EQUIV="content-security-policy" CONTENT="` + Policy + `">`), ok},
		{"spaced", appScene("<meta  http-equiv = \"Content-Security-Policy\"\n      content = \"" + Policy + "\" />"), ok},
		// the_content_itself_must_match_exactly
		{"earlier policy", appScene(tag("default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")), wrong},
		{"default-src *", appScene(tag(strings.Replace(Policy, "default-src 'none'", "default-src *", 1))), wrong},
		{"directive dropped", appScene(tag(strings.Replace(Policy, "; form-action 'none'", "", 1))), wrong},
		{"trailing semicolon", appScene(tag(Policy + ";")), wrong},
		{"leading space", appScene(tag(" " + Policy)), wrong},
		{"upper case", appScene(tag(strings.ToUpper(Policy))), wrong},
		{"directive added", appScene(tag(Policy + "; connect-src https:")), wrong},
		{"entities", appScene(tag(strings.ReplaceAll(Policy, "'", "&#39;"))), wrong},
		{"empty content", appScene(tag("")), wrong},
		{"no content", appScene(`<meta http-equiv="Content-Security-Policy">`), wrong},
		// a_missing_policy_fails
		{"no policy", appScene(""), none},
		{"only a script", "<script>fetch('https://example.com')</script>", none},
		{"empty file", "", none},
		{"policy as text", "<p>" + Policy + "</p>", none},
		{"another header", appScene(`<meta http-equiv="X-Content-Security-Policy" content="x">`), none},
		// two_policies_fail
		{"twice", appScene(PolicyLine + "\n" + PolicyLine), several},
		{"loose second", appScene(PolicyLine + "\n" + tag("default-src *")), several},
		{"second in body", strings.Replace(appScene(PolicyLine), "<canvas", PolicyLine+"<canvas", 1), several},
		// a_policy_after_a_script_fails
		{"script first", "<!doctype html><html><head><script>early()</script>" + PolicyLine + "</head></html>", script},
		{"upper-case script", "<SCRIPT >early()</SCRIPT>" + PolicyLine, script},
		{"not really a script tag", "<scripture></scripture>" + PolicyLine, notFirst},
		// a_file_that_is_not_html_fails
		{"a JPEG", "\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01", none},
		{"JSON", `{"title": "Quiet Clock"}`, none},
		{"binary junk first", "\x00\x1B\x24\x42" + appScene(PolicyLine), notFirst},
		// a_policy_the_engine_would_ignore_fails
		{"inside a comment", "<!-- " + PolicyLine + " --><script>run()</script>", notFirst},
		{"behind a comment that only looks open", "<!--><img src=x onerror=run()><!-- -->" + PolicyLine, notFirst},
		{"in the body", "<p>hello</p>" + PolicyLine + "<script>run()</script>", notFirst},
		{"inside a title", "<title>" + PolicyLine + "</title><script>run()</script>", notFirst},
		{"inside a template", "<template>" + PolicyLine + "</template><script>run()</script>", notFirst},
		{"inside a style", "<style>" + PolicyLine + "</style><script>run()</script>", notFirst},
		{"after an element with a handler", "<svg onload=run()></svg>" + PolicyLine, notFirst},
		{"after a redirect", `<meta http-equiv="refresh" content="0;url=https://example.com">` + PolicyLine, notFirst},
		{"after another charset", `<meta charset="iso-2022-jp">` + PolicyLine, notFirst},
		{"after text", "hello " + PolicyLine, notFirst},
		{"after a viewport", `<meta name="viewport" content="width=device-width">` + PolicyLine, notFirst},
		{"non-ASCII in an allowed tag", "<html lang=\"あ\">" + PolicyLine, notFirst},
		// a_repeated_or_extra_attribute_on_the_policy_fails
		{"decoy first", appScene(`<meta http-equiv="Content-Security-Policy" content="default-src *" content="` + Policy + `">`), wrong},
		{"decoy last", appScene(`<meta http-equiv="Content-Security-Policy" content="` + Policy + `" content="default-src *">`), wrong},
		{"extra attribute", appScene(`<meta http-equiv="Content-Security-Policy" content="` + Policy + `" name="x">`), wrong},
		{"two http-equiv", appScene(`<meta http-equiv="refresh" http-equiv="Content-Security-Policy" content="` + Policy + `">`), none},
		// the_usual_openings_are_accepted
		{"opening 1", "<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"UTF-8\">\n" + PolicyLine + "\n<title>t</title><script>draw()</script>", ok},
		{"opening 2", "<!doctype html><meta charset=utf-8>" + PolicyLine + "\n<title>t</title><script>draw()</script>", ok},
		{"opening 3", "<html lang='en' class=\"scene\"><head>" + PolicyLine + "\n<title>t</title><script>draw()</script>", ok},
		{"opening 4", "\n\n  <head>\n\t<meta charset='utf8' />\n" + PolicyLine + "\n<title>t</title><script>draw()</script>", ok},
		{"opening 5", PolicyLine + "\n<title>t</title><script>draw()</script>", ok},
		// a_tag_cut_off_by_the_end_of_the_file_is_not_a_policy
		{"cut before >", PolicyLine[:len(PolicyLine)-1], none},
		{"cut inside content", `<meta http-equiv="Content-Security-Policy" content="`, none},
		{"cut after name", "<meta", none},
		{"a lone <", "<", none},
	}
	for _, c := range cases {
		if got := appVerdict(c.html); got != c.want {
			t.Errorf("%s: gate says %s, the app says %s", c.name, got, c.want)
		}
	}
}
