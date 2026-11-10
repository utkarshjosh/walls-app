package scenecheck

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// goodHead and goodBody make a scene the way the kit writes one. The
// comments deliberately mention forbidden things: words in comments are
// documentation, not code, and must not fail the check.
const goodHead = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
` + PolicyLine + `
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Quiet Clock</title>
<!--
  Notes for readers, none of it code: fetch, eval, XMLHttpRequest, parent.frames,
  <iframe>, <script src="x.js">, https://example.com, document.cookie, location = 1.
-->
<style>
/* a style comment may mention window.open, localStorage and http://example.com */
html,body{margin:0;height:100%;overflow:hidden;background:#1c2230}
.top.bar{top:0}
</style>
</head>
<body>
<canvas id="c"></canvas>
`

const goodBody = `<script id="fs" type="x-shader/x-fragment">
// shader comment: texelFetch is a real GLSL word, and "fetch" here is only prose
precision mediump float;
uniform float uT; /* eval nothing, postMessage nothing */
void main(){ float a = 1./3.; gl_FragColor = vec4(a / 2., uT, 0., 1.); } // top. of the range, parent. too
</script>
<script>
(function () {
  // Geometry arrives in the URL fragment; this scene never calls fetch or opens a WebSocket.
  var canvas = document.getElementById('c');
  var g = { cw: 1, ch: 1, vx: 0, vy: 0, paused: false };
  function readHash() {
    var o = {}, kv = location.hash.slice(1).split('&'); /* importScripts is not used */
    for (var i = 0; i < kv.length; i++) { var p = kv[i].split('='); o[p[0]] = +p[1]; }
    g.paused = o.paused === 1;
    if (location.hash == '' || location.hash === '#') { g.paused = false; }
  }
  var cores = navigator.hardwareConcurrency || 2, ua = navigator.userAgent;
  var rect = canvas.getBoundingClientRect(), y = rect.top.toFixed(1), el = { parent: { x: 1 } }, px = el.parent.x;
  var prefetch = 1, evaluate = 2, texelFetch = 3, $fetch = 4, topmost = 5, apparent = 6;
  var re = /[//]/, ratio = innerWidth / innerHeight; // a division, then a comment that says eval
  var t = Date.now() / 1000 % 240;
  window.addEventListener('hashchange', readHash);
  window.addEventListener('resize', readHash);
  readHash();
})();
</script>
</body>
</html>
`

const goodHTML = goodHead + goodBody

const goodMeta = `{"standard": 1, "title": "Quiet Clock", "tags": ["clock", "minimal"], "color": "#1C2230",
 "author": "Still Originals", "license": "Still Original", "sourceUrl": null, "layout": "per-display"}`

var goodPoster = sync.OnceValue(func() []byte { return jpegOf(PosterWidth, PosterHeight) })

func jpegOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(40 + 60*x/w), uint8(50 + 40*y/h), 80, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func codes(problems []Problem) []string {
	var out []string
	for _, p := range problems {
		if !slices.Contains(out, p.Code) {
			out = append(out, p.Code)
		}
	}
	slices.Sort(out)
	return out
}

func describe(problems []Problem) string {
	var b strings.Builder
	for _, p := range problems {
		b.WriteString("\n    " + p.String())
	}
	return b.String()
}

func TestGoodScenePasses(t *testing.T) {
	if problems := CheckHTML([]byte(goodHTML)); len(problems) != 0 {
		t.Fatalf("the model scene should pass, got:%s", describe(problems))
	}
}

// inScript puts one line of code into the scene's script.
func inScript(code string) string {
	return strings.Replace(goodHTML, "  readHash();\n", "  readHash();\n  "+code+"\n", 1)
}

// inBody puts markup at the start of the body.
func inBody(markup string) string {
	return strings.Replace(goodHTML, "<canvas id=\"c\"></canvas>\n", markup+"\n<canvas id=\"c\"></canvas>\n", 1)
}

func withPolicy(tag string) string { return strings.Replace(goodHTML, PolicyLine, tag, 1) }

func TestPolicyLine(t *testing.T) {
	const old = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'">`
	headless := strings.Replace(goodHTML, PolicyLine+"\n", "", 1)

	fail := []struct {
		name, html, code, mention string
	}{
		{"missing", headless, "policy-missing", "missing"},
		{"the earlier, weaker policy text", withPolicy(old), "policy-wrong", "differs"},
		{"one directive loosened", withPolicy(strings.Replace(PolicyLine, "default-src 'none'", "default-src *", 1)), "policy-wrong", "differs"},
		{"a directive added", withPolicy(strings.Replace(PolicyLine, `'none'">`, `'none'; media-src 'self'">`, 1)), "policy-wrong", "differs"},
		{"right text with a trailing semicolon", withPolicy(strings.Replace(PolicyLine, `'none'">`, `'none';">`, 1)), "policy-wrong", "differs"},
		{"no content at all", withPolicy(`<meta http-equiv="Content-Security-Policy">`), "policy-wrong", "differs"},
		{"two policies", withPolicy(PolicyLine + "\n" + PolicyLine), "policy-multiple", "2 Content-Security-Policy"},
		{"a second, looser policy in the body", inBody(`<meta http-equiv="Content-Security-Policy" content="default-src *">`), "policy-multiple", "exactly one"},
		{"a second policy inside a comment", inBody("<!-- " + PolicyLine + " -->"), "policy-multiple", "comments and strings included"},
		{"a second policy inside a script string", inScript("var s = '" + strings.ReplaceAll(PolicyLine, "'", `\'`) + "';"), "policy-multiple", "exactly one"},
		{"policy after a script", strings.Replace(goodHTML, PolicyLine, "<script>early()</script>\n"+PolicyLine, 1), "policy-order", "before any script"},
		{"policy inside a comment", withPolicy("<!-- " + PolicyLine + " -->"), "policy-place", "a comment comes before"},
		{"policy after a comment", withPolicy("<!-- hello -->\n" + PolicyLine), "policy-place", "a comment comes before"},
		{"policy inside a template", withPolicy("<template>" + PolicyLine + "</template>"), "policy-place", "<template> comes before"},
		{"policy after the title", withPolicy("<title>Clock</title>\n" + PolicyLine), "policy-place", "<title> comes before"},
		{"policy after a style", withPolicy("<style>body{margin:0}</style>\n" + PolicyLine), "policy-place", "<style> comes before"},
		{"policy after the viewport", withPolicy(`<meta name="viewport" content="width=device-width">` + "\n" + PolicyLine), "policy-place", "<meta> comes before"},
		{"policy after another charset", strings.Replace(goodHTML, `<meta charset="utf-8">`, `<meta charset="iso-2022-jp">`, 1), "policy-place", "<meta> comes before"},
		{"policy after text", withPolicy("hello " + PolicyLine), "policy-place", "text comes before"},
		{"policy in the body", "<!doctype html><html><head><meta charset=\"utf-8\"><title>t</title></head><body>" + PolicyLine + goodBody, "policy-place", "<title> comes before"},
		{"a decoy content before the real one", withPolicy(`<meta http-equiv="Content-Security-Policy" content="default-src *" content="` + Policy + `">`), "policy-wrong", "repeats an attribute"},
		{"a decoy content after the real one", withPolicy(`<meta http-equiv="Content-Security-Policy" content="` + Policy + `" content="default-src *">`), "policy-wrong", "repeats an attribute"},
		{"an extra attribute", withPolicy(`<meta http-equiv="Content-Security-Policy" content="` + Policy + `" name="x">`), "policy-wrong", "extra attribute (name)"},
		{"a non-ASCII byte before the tag", strings.Replace(goodHTML, `<html lang="en">`, "<html lang=\"あ\">", 1), "policy-place", "not plain ASCII"},
		{"a control byte before the tag", "\x00" + goodHTML, "policy-place", "comes before the policy line"},
	}
	for _, c := range fail {
		problems := CheckHTML([]byte(c.html))
		if got := codes(problems); !slices.Equal(got, []string{c.code}) {
			t.Errorf("%s: problem kinds %v, want only %q:%s", c.name, got, c.code, describe(problems))
			continue
		}
		if all := describe(problems); !strings.Contains(all, c.mention) {
			t.Errorf("%s: no reason mentions %q:%s", c.name, c.mention, all)
		}
	}

	// Two http-equiv values: a browser uses the first, so this is no policy.
	twoNames := withPolicy(`<meta http-equiv="refresh" http-equiv="Content-Security-Policy" content="` + Policy + `">`)
	if got := codes(CheckHTML([]byte(twoNames))); !slices.Equal(got, []string{"policy-missing"}) {
		t.Errorf("first http-equiv is not a policy: got %v", got)
	}

	// The same tag written other ways is still the same tag.
	pass := map[string]string{
		"attributes swapped":       `<meta content="` + Policy + `" http-equiv="Content-Security-Policy">`,
		"single-quoted http-equiv": `<meta http-equiv='Content-Security-Policy' content="` + Policy + `">`,
		"unquoted http-equiv":      `<meta http-equiv=Content-Security-Policy content="` + Policy + `">`,
		"upper-case names":         `<META HTTP-EQUIV="content-security-policy" CONTENT="` + Policy + `">`,
		"spread out, self-closed":  "<meta  http-equiv = \"Content-Security-Policy\"\n      content = \"" + Policy + "\" />",
	}
	for name, tag := range pass {
		if problems := CheckHTML([]byte(withPolicy(tag))); len(problems) != 0 {
			t.Errorf("%s should pass:%s", name, describe(problems))
		}
	}
	for name, opening := range map[string]string{
		"upper-case doctype and charset": "<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"UTF-8\">\n",
		"no html or head tags":           "<!doctype html><meta charset=utf-8>",
		"html with attributes":           "<html lang='en' class=\"scene\"><head>",
		"only a head":                    "\n\n  <head>\n\t<meta charset='utf8' />\n",
		"nothing at all":                 "",
		"a byte-order mark":              "\xEF\xBB\xBF<!doctype html>\n<html>\n<head>\n",
	} {
		html := opening + PolicyLine + "\n<title>t</title></head><body>\n" + goodBody
		if problems := CheckHTML([]byte(html)); len(problems) != 0 {
			t.Errorf("opening %q should pass:%s", name, describe(problems))
		}
	}
}

func TestForbiddenConstructs(t *testing.T) {
	cases := []struct {
		html    string
		mention string // what the reason must name
	}{
		// markup
		{inBody(`<script src="x.js"></script>`), "<script src>"},
		{inBody(`<SCRIPT type="module" SRC=x.js></SCRIPT>`), "<script src>"},
		{inBody(`<svg><script href="x.js"></script></svg>`), "<script src>"},
		{inBody(`<link rel="stylesheet" href="x.css">`), "<link>"},
		{inBody(`<iframe></iframe>`), "<iframe>"},
		{inBody(`<object data="x"></object>`), "<object>"},
		{inBody(`<embed>`), "<embed>"},
		{inBody(`<form></form>`), "<form>"},
		{inBody(`<base target="_blank">`), "<base>"},
		{inBody(`<a href="#x">x</a>`), "<a href>"},
		{inBody(`<svg><a xlink:href="#x"></a></svg>`), "<a href>"},
		{inScript(`document.body.innerHTML = '<iframe>';`), "<iframe>"},
		// addresses
		{inScript(`var u = "http://example.com/a";`), "http: URL"},
		{inScript(`var u = 'HTTPS://example.com/a';`), "https: URL"},
		{inScript(`var u = "ws://example.com";`), "ws: URL"},
		{inScript(`var u = "wss://example.com";`), "wss: URL"},
		{inBody(`<img src="https://example.com/a.png">`), "https: URL"},
		{strings.Replace(goodHTML, ".top.bar{top:0}", "body{background:url(http://example.com/a.png)}", 1), "http: URL"},
		// talking to the outside
		{inScript(`fetch('/x');`), "fetch"},
		{inScript(`self . fetch ('/x');`), "fetch"},
		{inScript(`var f = window["fetch"];`), "fetch"},
		{inScript(`var x = new XMLHttpRequest();`), "XMLHttpRequest"},
		{inScript(`var s = new WebSocket(u);`), "WebSocket"},
		{inScript(`var s = new EventSource(u);`), "EventSource"},
		{inScript(`navigator.sendBeacon(u, d);`), "sendBeacon"},
		{inScript(`importScripts('x.js');`), "importScripts"},
		{inScript(`import('./x.js');`), "import("},
		{inScript(`postMessage('x', '*');`), "postMessage"},
		{inScript(`new Notification('hello');`), "Notification"},
		// running text as code
		{inScript(`eval('1+1');`), "eval"},
		{inScript(`var e = (0, eval);`), "eval"},
		{inScript(`var f = new Function('return 1');`), "new Function"},
		{inScript(`var f = Function('return 1')();`), "new Function"},
		// storage
		{inScript(`localStorage.setItem('a', 1);`), "localStorage"},
		{inScript(`var s = sessionStorage;`), "sessionStorage"},
		{inScript(`indexedDB.open('x');`), "indexedDB"},
		{inScript(`var c = document.cookie;`), "document.cookie"},
		{inScript(`document . cookie = 'a=1';`), "document.cookie"},
		// leaving or reaching outside the page
		{inScript(`window.open('about:blank');`), "window.open"},
		{inScript(`location = '#x';`), "assignment to location"},
		{inScript(`window.location = u;`), "assignment to location"},
		{inScript(`location.href = u;`), "assignment to location.href"},
		{inScript(`location.hash = 'cw=1';`), "assignment to location.hash"},
		{inScript(`location.hash += 'x';`), "assignment to location.hash"},
		{inScript(`location['href'] = u;`), "assignment to location[…]"},
		{inScript(`location.assign(u);`), "location.assign()"},
		{inScript(`document.location.replace(u);`), "location.replace()"},
		{inScript(`var g2 = navigator.geolocation;`), "navigator.geolocation"},
		{inScript(`var l = navigator.language;`), "navigator.language"},
		{inScript(`var d = navigator.userAgentData;`), "navigator.userAgentData"},
		{inScript(`var l = navigator['language'];`), "navigator[…]"},
		{inScript(`var d = top.document;`), "top."},
		{inScript(`var d = parent.document;`), "parent."},
		{inScript(`if (window.top !== window) {}`), "window.top"},
		{inScript(`var d = self.parent.document;`), "window.parent"},
		// in an attribute handler, not only in <script>
		{inBody(`<svg onload="fetch('/x')"></svg>`), "fetch"},
	}
	for _, c := range cases {
		problems := CheckHTML([]byte(c.html))
		ok := false
		for _, p := range problems {
			if p.Code != "forbidden" {
				t.Errorf("want %q: unexpected problem %s", c.mention, p)
			}
			if p.Message == "forbidden: "+c.mention || strings.HasPrefix(p.Message, "forbidden: "+c.mention+" ") {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%q was not reported; got:%s", c.mention, describe(problems))
		}
	}
}

// Things that look alarming to a careless matcher but are fine.
func TestAllowedLookalikes(t *testing.T) {
	for _, code := range []string{
		`var h = location.hash;`,
		`var same = location.hash == other, same2 = location.hash === other, differ = location.hash != other;`,
		`var f = function (location) { return location.hash; };`,
		`var o = { location: 1, top: 2, parent: 3, open: 4, cookie: 5 };`,
		`var n = navigator.hardwareConcurrency + navigator.userAgent.length;`,
		`var y = rect.top.toFixed(2), z = node.parent.x, q = style.top;`,
		`var prefetched = 1, fetching = 2, refetch = 3, evals = 4, _eval = 5, WebSocketish = 6;`,
		`var s = "the top of the page", u = "data:image/png;base64,AAAA";`,
		`var a = b / c / d; // then a comment: fetch(eval(localStorage))`,
		`var a = (b + c) / 2; /* fetch */ var e = f[0] / 2; // XMLHttpRequest`,
		"var t = `ratio ${ innerWidth / innerHeight } // not a comment`;",
		`opener = null;`,
	} {
		if problems := CheckHTML([]byte(inScript(code))); len(problems) != 0 {
			t.Errorf("should pass: %s%s", code, describe(problems))
		}
	}
}

// The comment-stripping must never hide something a browser would run.
// Each of these files tries to tuck a fetch behind something that only looks
// like a comment; every one must be caught.
func TestCodeCannotHideBehindFakeComments(t *testing.T) {
	cases := map[string]string{
		"a // inside a string":                       inScript(`var u = 'x//'; fetch(u);`),
		"a /* inside a string":                       inScript("var u = \"/*\"; fetch(u); var v = \"*/\";"),
		"a // inside a regular expression":           inScript(`var re = /[//]/; fetch(re);`),
		"a /* inside a regular expression":           inScript(`var re = /[/*]/; fetch(re); /* x */`),
		"a // inside a template":                     inScript("var t = `a//b`; fetch(t);"),
		"a // inside a nested template":              inScript("var t = `${ `//` }`; fetch(t);"),
		"an HTML comment opener inside a title":      inBody(`<title><!-- x</title><script>fetch(1)</script><!-- -->`),
		"an HTML comment opener inside a textarea":   inBody(`<textarea><!--</textarea><script>fetch(1)</script><!-- --></textarea>`),
		"an HTML comment opener inside noscript":     inBody(`<noscript><!--</noscript><script>fetch(1)</script><!-- -->`),
		"an HTML comment opener in an attribute":     inBody(`<div title="<!--"></div><script>fetch(1)</script><p title="-->"></p>`),
		"an HTML comment opener in a bogus comment":  inBody(`<!x <!-- ><script>fetch(1)</script><!-- -->`),
		"an HTML comment opener after a doctype-ish": inBody(`<?php <!-- ?><script>fetch(1)</script><!-- -->`),
		"an empty comment that looks open":           inBody(`<!--><script>fetch(1)</script><!-- -->`),
		"a comment closed with --!>":                 inBody(`<!-- x --!><script>fetch(1)</script><!-- -->`),
		"CDATA inside SVG":                           inBody(`<svg><![CDATA[ > <!-- ]]><script>fetch(1)</script><!-- --></svg>`),
		"a self-closed SVG script":                   inBody(`<svg><script/></svg><br/> //<script>fetch(1)</script>`),
		"a self-closed SVG style":                    inBody(`<svg><style/></svg> /* <script>fetch(1)</script> <style>*/</style>`),
		"a script that only looks finished":          inBody("<script>var a = \"<!--<script>\"; </script>\n<!--\nfetch(1)\n-->\n</script>"),
		"a // in a style":                            strings.Replace(goodHTML, ".top.bar{top:0}", "a{b:url(//x)} b{background:url(http://e.example/z)}", 1),
		"a /* inside url()":                          strings.Replace(goodHTML, ".top.bar{top:0}", "a{background:url(data:x/*y)} b{background:url(http://e.example/z)} /* c */", 1),
		"a /* inside a style string":                 strings.Replace(goodHTML, ".top.bar{top:0}", "a{content:\"/*\"} b{background:url(http://e.example/z)} c{content:\"*/\"}", 1),
		"an odd space before the name":               inScript(" fetch('/x');"),
	}
	for name, html := range cases {
		problems := CheckHTML([]byte(html))
		caught := false
		for _, p := range problems {
			if p.Code == "forbidden" {
				caught = true
			}
		}
		if !caught {
			t.Errorf("%s: the hidden code was not reported; got:%s", name, describe(problems))
		}
	}
}

func TestBlankCommentsKeepsLinesAndCode(t *testing.T) {
	src := "<p>a</p><!-- one\ntwo --><script>var x = 1; // gone\n/* gone\n too */ var y = \"// kept\";</script><style>a{} /* gone */ b{}</style>"
	want := "<p>a</p>        \n       <script>var x = 1;        \n       \n        var y = \"// kept\";</script><style>a{}            b{}</style>"
	clean, scripts := blankComments([]byte(src))
	if len(scripts) != 1 || !strings.HasPrefix(src[scripts[0][0]:scripts[0][1]], "var x = 1;") || !strings.HasSuffix(src[scripts[0][0]:scripts[0][1]], "kept\";") {
		t.Errorf("script ranges = %v", scripts)
	}
	if got := string(clean); got != want {
		t.Errorf("blankComments:\n got %q\nwant %q", got, want)
	}
}

func TestLineNumbersAndGrouping(t *testing.T) {
	html := inScript("fetch(a);\n  var ok = 1;\n  fetch(b); fetch(c);\n  eval(d);")
	problems := CheckHTML([]byte(html))
	if len(problems) != 2 {
		t.Fatalf("want one reason per construct, got:%s", describe(problems))
	}
	line := 1 + strings.Count(html[:strings.Index(html, "fetch(a)")], "\n")
	if problems[0].Line != line || problems[0].Message != "forbidden: fetch (also on line "+itoa(line+2)+")" {
		t.Errorf("fetch reported as %q at line %d, want line %d and a mention of line %d", problems[0].Message, problems[0].Line, line, line+2)
	}
	if problems[1].Line != line+3 || problems[1].Message != "forbidden: eval" || problems[1].Where() != "scene.html:"+itoa(line+3) {
		t.Errorf("eval reported as %s", problems[1])
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestSceneMustReadHashAndHandlePaused(t *testing.T) {
	noHash := strings.NewReplacer("location.hash", "window.name").Replace(goodHTML)
	if got := codes(CheckHTML([]byte(noHash))); !slices.Equal(got, []string{"no-location-hash"}) {
		t.Errorf("no location.hash: got %v", got)
	}
	noPause := strings.ReplaceAll(goodHTML, "paused", "halted")
	if got := codes(CheckHTML([]byte(noPause))); !slices.Equal(got, []string{"no-paused"}) {
		t.Errorf("no paused: got %v", got)
	}
	// Mentioning them only in a comment does not count.
	onlyInComment := strings.Replace(noPause, "(function () {", "(function () { // paused is handled, honest", 1)
	if got := codes(CheckHTML([]byte(onlyInComment))); !slices.Equal(got, []string{"no-paused"}) {
		t.Errorf("paused only in a comment: got %v", got)
	}
}

func TestCheckMeta(t *testing.T) {
	meta, problems := CheckMeta([]byte(goodMeta))
	if len(problems) != 0 {
		t.Fatalf("good meta:%s", describe(problems))
	}
	if meta.Title != "Quiet Clock" || meta.Author != "Still Originals" || meta.License != "Still Original" ||
		meta.SourceURL != "" || !slices.Equal(meta.Tags, []string{"clock", "minimal"}) || meta.Color != "#1c2230" ||
		meta.Layout != LayoutPerDisplay {
		t.Errorf("meta = %+v", meta)
	}

	adapted := `{"standard":1,"title":"Slow Sea","author":"Ana Ruiz","license":"CC BY 4.0","sourceUrl":"https://example.com/sea","layout":"canvas"}`
	meta, problems = CheckMeta([]byte(adapted))
	if len(problems) != 0 || meta.SourceURL != "https://example.com/sea" || meta.Tags == nil || len(meta.Tags) != 0 || meta.Color != "" ||
		meta.Layout != LayoutCanvas {
		t.Errorf("adapted meta = %+v%s", meta, describe(problems))
	}

	// layout: required, and one of exactly two values.
	base := `{"standard":1,"title":"A","author":"B","license":"C"`
	for value, want := range map[string]string{
		`"canvas"`: LayoutCanvas, `"per-display"`: LayoutPerDisplay,
	} {
		meta, problems := CheckMeta([]byte(base + `,"layout":` + value + `}`))
		if len(problems) != 0 || meta.Layout != want {
			t.Errorf("layout %s should pass as %q: %+v%s", value, want, meta, describe(problems))
		}
	}
	for _, tail := range []string{
		`}`, `,"layout":null}`, // missing
		`,"layout":"stretch"}`, `,"layout":"Canvas"}`, `,"layout":"per display"}`, `,"layout":"perDisplay"}`,
		`,"layout":""}`, `,"layout":1}`, `,"layout":["canvas"]}`, `,"layout":true}`,
	} {
		meta, problems := CheckMeta([]byte(base + tail))
		all := describe(problems)
		if len(problems) != 1 || meta.Layout != "" || !strings.Contains(all, `"canvas" (one picture across displays)`) ||
			!strings.Contains(all, `"per-display" (drawn whole on each display)`) {
			t.Errorf("meta ending %s: want one plain reason naming both layouts, got %+v%s", tail, meta, all)
		}
	}

	bad := map[string]string{
		`{"standard": 1, "title": "A", "author": "B", "license":`: "not a JSON object",
		`["standard", 1]`: "not a JSON object",
		`{"title": "A", "author": "B", "license": "C"}`:                                            `no "standard"`,
		`{"standard": 2, "title": "A", "author": "B", "license": "C"}`:                             `"standard" must be the number 1`,
		`{"standard": "1", "title": "A", "author": "B", "license": "C"}`:                           `"standard" must be the number 1`,
		`{"standard": 1, "author": "B", "license": "C"}`:                                           `no "title"`,
		`{"standard": 1, "title": "A", "license": "C"}`:                                            `no "author"`,
		`{"standard": 1, "title": "A", "author": "B"}`:                                             `no "license"`,
		`{"standard": 1, "title": "A", "author": "  ", "license": "C"}`:                            `"author" is empty`,
		`{"standard": 1, "title": "A", "author": "B", "license": null}`:                            `no "license"`,
		`{"standard": 1, "title": 7, "author": "B", "license": "C"}`:                               `"title" must be text`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "sourceUrl": "ftp://x/y"}`:   `"sourceUrl" must be null or an http(s) link`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "sourceUrl": "example.com"}`: `"sourceUrl" must be null or an http(s) link`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "sourceUrl": ""}`:            `"sourceUrl" must be null or an http(s) link`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "tags": "clock"}`:            `"tags" must be a list of text`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "tags": ["Clock"]}`:          `tag "Clock" must be lowercase`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "tags": ["ok", " padded "]}`: `tag " padded " must be lowercase`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "color": "blue"}`:            `"color" must look like`,
		`{"standard": 1, "title": "A", "author": "B", "license": "C", "color": "#1c22"}`:           `"color" must look like`,
	}
	for data, want := range bad {
		_, problems := CheckMeta([]byte(data))
		if all := describe(problems); len(problems) == 0 || !strings.Contains(all, want) {
			t.Errorf("%s\n  want a reason mentioning %q, got:%s", data, want, all)
		}
	}

	// Several things wrong at once: every one is reported.
	_, problems = CheckMeta([]byte(`{"standard": 3, "tags": ["A"], "color": "red"}`))
	if len(problems) != 7 {
		t.Errorf("want 7 reasons (standard, title, author, license, layout, tag, color), got %d:%s", len(problems), describe(problems))
	}
}

func TestCheckPoster(t *testing.T) {
	if img, problems := CheckPoster(goodPoster()); len(problems) != 0 || img == nil {
		t.Fatalf("good poster:%s", describe(problems))
	}
	if _, problems := CheckPoster(jpegOf(1280, 720)); len(problems) != 1 || !strings.Contains(problems[0].Message, "is 1280×720; it must be exactly 1920×1200") {
		t.Errorf("small poster:%s", describe(problems))
	}
	if _, problems := CheckPoster(jpegOf(1200, 1920)); len(problems) != 1 {
		t.Errorf("portrait poster:%s", describe(problems))
	}
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, PosterWidth, PosterHeight)))
	if _, problems := CheckPoster(pngData.Bytes()); len(problems) != 1 || !strings.Contains(problems[0].Message, "not a JPEG") {
		t.Errorf("PNG named poster.jpg:%s", describe(problems))
	}
	if _, problems := CheckPoster(goodPoster()[:2000]); len(problems) != 1 || !strings.Contains(problems[0].Message, "does not decode") {
		t.Errorf("truncated poster:%s", describe(problems))
	}
}

func writeScene(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "quiet-clock")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if data == nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCheckDir(t *testing.T) {
	good := func() map[string][]byte {
		return map[string][]byte{"scene.html": []byte(goodHTML), "poster.jpg": goodPoster(), "meta.json": []byte(goodMeta)}
	}

	res := CheckDir(writeScene(t, good()))
	if !res.OK() {
		t.Fatalf("good scene:%s", describe(res.Problems))
	}
	if res.Name != "quiet-clock" || string(res.HTML) != goodHTML || !bytes.Equal(res.PosterData, goodPoster()) ||
		res.Poster == nil || res.Meta.Title != "Quiet Clock" || res.Meta.Layout != LayoutPerDisplay {
		t.Errorf("result does not carry what was checked: name %q, %d html bytes, meta %+v", res.Name, len(res.HTML), res.Meta)
	}

	// Exactly at the limit passes; one byte over does not. Padding goes in
	// the body, where it changes nothing else.
	pad := func(total int) []byte {
		return []byte(strings.Replace(goodHTML, "<canvas id=\"c\">", strings.Repeat(" ", total-len(goodHTML))+"<canvas id=\"c\">", 1))
	}
	atLimit := good()
	atLimit["scene.html"] = pad(MaxSceneBytes)
	if res := CheckDir(writeScene(t, atLimit)); !res.OK() {
		t.Errorf("a scene of exactly 512 KB should pass:%s", describe(res.Problems))
	}
	over := good()
	over["scene.html"] = pad(MaxSceneBytes + 1)
	res = CheckDir(writeScene(t, over))
	if got := codes(res.Problems); !slices.Equal(got, []string{"too-large"}) || !strings.Contains(describe(res.Problems), "the limit is 512 KB") {
		t.Errorf("oversize scene: %v%s", got, describe(res.Problems))
	}

	for name, change := range map[string]func(map[string][]byte){
		"missing": func(f map[string][]byte) { f["scene.html"] = nil },
		"poster":  func(f map[string][]byte) { f["poster.jpg"] = jpegOf(960, 600) },
		"meta": func(f map[string][]byte) {
			f["meta.json"] = []byte(`{"title":"Old style","tags":["x"],"color":"#a8676a"}`)
		},
		"forbidden": func(f map[string][]byte) { f["scene.html"] = []byte(inScript("fetch('/x')")) },
	} {
		files := good()
		change(files)
		res := CheckDir(writeScene(t, files))
		if got := codes(res.Problems); res.OK() || !slices.Equal(got, []string{name}) {
			t.Errorf("%s: got %v%s", name, got, describe(res.Problems))
		}
	}

	// Everything wrong at once: every reason is listed, across all three files.
	res = CheckDir(writeScene(t, map[string][]byte{
		"scene.html": []byte("<html><body><script src=x.js></script><script>fetch('http://e.example')</script></body></html>"),
		"poster.jpg": []byte("not a picture"),
		"meta.json":  []byte(`{"standard": 1, "title": "T"}`),
	}))
	want := []string{"forbidden", "meta", "no-location-hash", "no-paused", "policy-missing", "poster"}
	if got := codes(res.Problems); !slices.Equal(got, want) {
		t.Errorf("all-wrong scene: kinds %v, want %v%s", got, want, describe(res.Problems))
	}
	if len(res.Problems) != 10 { // 3 forbidden constructs, 3 meta fields, 4 single ones
		t.Errorf("all-wrong scene: %d reasons, want 10:%s", len(res.Problems), describe(res.Problems))
	}

	if res := CheckDir(filepath.Join(t.TempDir(), "nope")); res.OK() || res.Problems[0].Where() != "folder" {
		t.Errorf("missing folder: %+v", res.Problems)
	}
	if res := CheckDir(writeScene(t, map[string][]byte{})); len(res.Problems) != 3 {
		t.Errorf("empty folder should report three missing files:%s", describe(res.Problems))
	}
}

// A scene has to be able to build SVG from script, and that needs three
// fixed namespace names that look like web addresses. Exactly those strings
// are allowed, in the places a namespace name belongs; nothing else is.
func TestNamespaceNamesAreAllowedButOnlyExactly(t *testing.T) {
	const svg, xlink, xhtml = "http://www.w3.org/2000/svg", "http://www.w3.org/1999/xlink", "http://www.w3.org/1999/xhtml"

	pass := map[string]string{
		"createElementNS, double quotes": inScript(`var g = document.createElementNS("` + svg + `", "g");`),
		"createElementNS, single quotes": inScript(`var g = document.createElementNS('` + svg + `', 'g');`),
		"a constant, no spaces":          inScript(`var NS="` + svg + `",XL='` + xlink + `',XH="` + xhtml + `";`),
		"a template literal":             inScript("var NS = `" + svg + "`;"),
		"setAttributeNS with xlink":      inScript(`use.setAttributeNS("` + xlink + `", "href", "#hand");`),
		"svg markup inside a string":     inScript(`el.innerHTML = '<svg xmlns="` + svg + `" xmlns:xlink="` + xlink + `"><g/></svg>';`),
		"svg markup with escaped quotes": inScript(`el.innerHTML = "<svg xmlns=\"` + svg + `\"></svg>";`),
		"inline svg, quoted":             inBody(`<svg xmlns="` + svg + `" xmlns:xlink='` + xlink + `' viewBox="0 0 10 10"><circle r="4"/></svg>`),
		"inline svg, unquoted":           inBody(`<svg xmlns=` + svg + ` viewBox="0 0 10 10"></svg>`),
		"inline svg, unquoted, last":     inBody(`<svg viewBox="0 0 10 10" XMLNS = ` + svg + `></svg>`),
		"xhtml on a foreignObject child": inBody(`<svg><foreignObject><div xmlns="` + xhtml + `">12:00</div></foreignObject></svg>`),
		"an svg picture in a data: URL":  strings.Replace(goodHTML, ".top.bar{top:0}", `body{background:url("data:image/svg+xml,%3Csvg xmlns='`+svg+`' viewBox='0 0 1 1'/%3E")}`, 1),
		"an svg picture built in script": inScript(`img.src = "data:image/svg+xml," + encodeURIComponent("<svg xmlns='` + svg + `'/>");`),
	}
	for name, html := range pass {
		if problems := CheckHTML([]byte(html)); len(problems) != 0 {
			t.Errorf("%s should pass:%s", name, describe(problems))
		}
	}

	fail := map[string]string{
		"a longer host":                     inScript(`var u = "` + svg + `.evil.example/";`),
		"a longer path":                     inScript(`var u = "` + svg + `/collect";`),
		"a longer word":                     inScript(`var u = "` + svg + `z";`),
		"a query":                           inScript(`var u = '` + xlink + `?d=' + data;`),
		"a fragment":                        inScript(`var u = "` + xhtml + `#x";`),
		"a template that continues":         inScript("var u = `" + svg + "${path}`;"),
		"https instead of http":             inScript(`var NS = "https://www.w3.org/2000/svg";`),
		"another letter case":               inScript(`var NS = "HTTP://www.w3.org/2000/svg";`),
		"a different w3.org address":        inScript(`var u = "http://www.w3.org/2000/svgs";`),
		"another year":                      inScript(`var u = "http://www.w3.org/2001/svg";`),
		"some other address":                inScript(`var u = "http://example.com/2000/svg";`),
		"mismatched quotes":                 inScript(`var u = "` + svg + `'+x+'";`),
		"not quoted at all in script":       inScript(`var u = x ? ` + svg + ` : 0;`),
		"an image source":                   inBody(`<img src="` + svg + `">`),
		"an unquoted image source":          inBody(`<img src=` + svg + `>`),
		"a style url":                       strings.Replace(goodHTML, ".top.bar{top:0}", `body{background:url("`+svg+`")}`, 1),
		"an attribute that only ends xmlns": inBody(`<img data-xmlns="` + svg + `">`),
		"unquoted xmlns that continues":     inBody(`<svg xmlns=` + svg + `.evil.example/></svg>`),
		"unquoted xmlns with a slash":       inBody(`<svg xmlns=` + svg + `/></svg>`),
		"quoted xmlns that continues":       inBody(`<svg xmlns="` + svg + `.evil.example/"></svg>`),
		// Code in a handler attribute is markup to this check, so even the exact
		// name is refused there; the code belongs in a <script>.
		"the exact name in a handler attribute": inBody(`<svg onload="this.appendChild(document.createElementNS('` + svg + `','g'))"></svg>`),
		"a longer name in a handler attribute":  inBody(`<svg onload="this.appendChild(document.createElementNS('` + svg + `.evil.example','g'))"></svg>`),
		"plain text in the page":                inBody(`<p>` + svg + `</p>`),
	}
	for name, html := range fail {
		problems := CheckHTML([]byte(html))
		caught := false
		for _, p := range problems {
			if p.Code == "forbidden" && strings.HasPrefix(p.Message, "forbidden: http") {
				caught = true
			}
		}
		if !caught {
			t.Errorf("%s: the address was not reported; got:%s", name, describe(problems))
		}
	}

	// In a comment the names are documentation, like everything else there.
	if problems := CheckHTML([]byte(inScript("// see " + svg + ".evil.example/ for nothing"))); len(problems) != 0 {
		t.Errorf("a namespace name in a comment should pass:%s", describe(problems))
	}
}
