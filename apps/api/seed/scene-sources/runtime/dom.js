/* ---- scenekit runtime for per-display scenes. Shared by every such scene: change it in the kit, not here. ----

   Scene.start({ layout, minute, frame, fps })   begin; every part is optional
     layout(width, height)   the window's size in points; called first and on every resize
     minute(now)             called at once and then on each new minute, also while paused
     frame(now)              continuous drawing, at most `fps` times a second (30 at most);
                             stops while paused or hidden, after leaving one finished frame
   Scene.now()               the time to show: the wall clock, or the frozen &t= moment
   Scene.phase(seconds)      0..1, where the wall clock is inside a repeating period of that length
   Scene.backdrop(spec)      a smooth full-window gradient that never shows bands:
                             { colors: [upper, middle, lower], lights: [{ color, x, y, size, strength }] }
                             colours are '#rrggbb'; up to two soft lights; x, y are 0..1 across and
                             down the window; size is in window heights. Call it again to change it.

   The URL fragment carries paused=0|1 and, for stills, t=<seconds since local midnight>.
   The geometry values (cw, ch, vx, vy) are ignored on purpose: a per-display scene lays
   itself out whole in its own window, so each display shows the complete scene.            */
var Scene = (function () {
  'use strict';
  var hooks = {}, paused = false, frozen = null, started = false;
  var raf = 0, last = 0, frameMs = 1000 / 30, minuteTimer = 0, paint = null, shown = null;

  function read() {
    var o = {}, kv = location.hash.slice(1).split('&'), i, j;
    for (i = 0; i < kv.length; i++) {
      j = kv[i].indexOf('=');
      if (j > 0) o[kv[i].slice(0, j)] = parseFloat(kv[i].slice(j + 1));
    }
    paused = o.paused === 1;
    frozen = isFinite(o.t) ? o.t : null;
  }

  function now() {
    var d = new Date();
    if (frozen === null) return d;
    d.setHours(0, 0, 0, 0);
    return new Date(d.getTime() + frozen * 1000);
  }

  function phase(seconds) {
    var p = seconds * 1000, ms = frozen === null ? Date.now() : frozen * 1000;
    return ((ms % p) + p) % p / p;
  }

  // One complete, up-to-date picture.
  function refresh() {
    var d = now();
    if (hooks.minute) hooks.minute(d);
    if (hooks.frame) hooks.frame(d);
  }

  function tick(t) {
    raf = requestAnimationFrame(tick);
    if (t - last < frameMs - 2) return;
    last = t;
    hooks.frame(now());
  }

  function onMinute() {
    minuteTimer = 0;
    refresh();
    arm();
  }

  // Wake just after the next minute begins. Re-armed every time, so it never drifts.
  function arm() {
    clearTimeout(minuteTimer);
    minuteTimer = setTimeout(onMinute, 60000 - (Date.now() % 60000) + 30);
  }

  function sync() {
    if (!started) return;
    var visible = !document.hidden, still = frozen !== null;
    var run = visible && !paused && !still && !!hooks.frame;
    if (run) { if (!raf) raf = requestAnimationFrame(tick); }
    else if (raf) { cancelAnimationFrame(raf); raf = 0; }
    // The once-a-minute update keeps going while paused, but not while hidden or frozen.
    if (visible && !still && (hooks.minute || hooks.frame)) arm(); else { clearTimeout(minuteTimer); minuteTimer = 0; }
    if (visible) refresh();
  }

  function size() {
    if (hooks.layout) hooks.layout(Math.max(1, window.innerWidth || 1), Math.max(1, window.innerHeight || 1));
  }

  function start(o) {
    hooks = o || {};
    frameMs = 1000 / Math.max(0.2, Math.min(30, hooks.fps || 30));
    started = true;
    read();
    size();
    sync();
  }

  // ---- backdrop: a dithered gradient drawn by a small shader, with a plain CSS gradient as the fallback ----
  function rgb(hex) {
    var n = parseInt(String(hex).replace('#', ''), 16) || 0;
    return [(n >> 16 & 255) / 255, (n >> 8 & 255) / 255, (n & 255) / 255];
  }

  function makeBackdrop() {
    var c = document.createElement('canvas'), gl = null, u = {}, names = ['uW', 'uA', 'uM', 'uB', 'uG1', 'uH1', 'uG2', 'uH2'];
    c.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;z-index:-1;display:block';
    var was = '', plain = function (spec) {
      var css = 'linear-gradient(180deg,' + spec.colors.join(',') + ')';
      if (css !== was) { was = css; document.body.style.background = css; }
    };
    try { gl = c.getContext('webgl', { alpha: false, antialias: false, depth: false, stencil: false, powerPreference: 'low-power' }); }
    catch (e) { gl = null; }
    if (!gl) return plain;
    var fs = [
      '#ifdef GL_FRAGMENT_PRECISION_HIGH', 'precision highp float;', '#else', 'precision mediump float;', '#endif',
      'uniform vec4 uW, uG1, uG2; uniform vec3 uA, uM, uB, uH1, uH2;',
      'float h(vec2 p){ vec3 q = fract(vec3(p.xyx) * .1031); q += dot(q, q.yzx + 33.33); return fract((q.x + q.y) * q.z); }',
      'vec3 glow(vec3 c, vec2 q, float asp, vec4 g, vec3 k){ vec2 d = (q - vec2(g.x * asp, g.y)) / max(g.z, .001); return mix(c, k, g.w * exp(-dot(d, d))); }',
      'void main(){',
      '  vec2 uv = gl_FragCoord.xy / uW.zw; float y = 1. - uv.y, asp = uW.x / uW.y;',
      '  vec3 m = 2. * uM - .5 * (uA + uB);',                       // a curve through all three colours, with no kink
      '  vec3 c = clamp(mix(mix(uA, m, y), mix(m, uB, y), y), 0., 1.);',
      '  vec2 q = vec2(uv.x * asp, y);',
      '  c = glow(c, q, asp, uG1, uH1); c = glow(c, q, asp, uG2, uH2);',
      '  float n = h(gl_FragCoord.xy) + h(gl_FragCoord.xy + 31.7) - 1.;',
      '  gl_FragColor = vec4(c + n / 255., 1.);',
      '}'].join('\n');
    var make = function (type, src) { var s = gl.createShader(type); gl.shaderSource(s, src); gl.compileShader(s); return s; };
    var p = gl.createProgram();
    gl.attachShader(p, make(gl.VERTEX_SHADER, 'attribute vec2 a;void main(){gl_Position=vec4(a,0.,1.);}'));
    gl.attachShader(p, make(gl.FRAGMENT_SHADER, fs));
    gl.bindAttribLocation(p, 0, 'a');
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) return plain;
    gl.useProgram(p);
    gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
    names.forEach(function (n) { u[n] = gl.getUniformLocation(p, n); });
    // The canvas sits behind the page; the body's own colour would cover it, so that is cleared.
    document.body.insertBefore(c, document.body.firstChild);
    document.body.style.background = 'none';
    var gone = false;
    c.addEventListener('webglcontextlost', function (e) { e.preventDefault(); gone = true; if (shown) plain(shown); });
    c.addEventListener('webglcontextrestored', function () { c.remove(); paint = null; if (shown) backdrop(shown); });
    return function (spec) {
      if (gone) return;
      var w = Math.max(1, window.innerWidth || 1), h = Math.max(1, window.innerHeight || 1);
      var s = Math.min(window.devicePixelRatio || 1, 960 / Math.max(w, h));     // smooth content: a small buffer is plenty
      var bw = Math.max(1, Math.round(w * s)), bh = Math.max(1, Math.round(h * s));
      if (c.width !== bw || c.height !== bh) { c.width = bw; c.height = bh; gl.viewport(0, 0, bw, bh); }
      var k = spec.colors, g = spec.lights || [], i, a;
      gl.uniform4f(u.uW, w, h, bw, bh);
      a = rgb(k[0]); gl.uniform3f(u.uA, a[0], a[1], a[2]);
      a = rgb(k[1]); gl.uniform3f(u.uM, a[0], a[1], a[2]);
      a = rgb(k[2]); gl.uniform3f(u.uB, a[0], a[1], a[2]);
      for (i = 0; i < 2; i++) {
        var q = g[i] || { color: '#000000', x: 0, y: 0, size: 1, strength: 0 };
        a = rgb(q.color);
        gl.uniform4f(u['uG' + (i + 1)], q.x, q.y, q.size, q.strength);
        gl.uniform3f(u['uH' + (i + 1)], a[0], a[1], a[2]);
      }
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    };
  }

  function backdrop(spec) {
    shown = spec;
    if (!paint) paint = makeBackdrop();
    paint(spec);
  }

  window.addEventListener('hashchange', function () { read(); sync(); });
  window.addEventListener('resize', function () { if (shown) backdrop(shown); if (started) { size(); refresh(); } });
  document.addEventListener('visibilitychange', sync);

  return { start: start, now: now, phase: phase, backdrop: backdrop };
})();
/* ---- end of the scenekit runtime ---- */
