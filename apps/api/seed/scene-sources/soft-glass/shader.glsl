
#define DITHER 1.0

// Colourways. The live scene is dichroic glass in a dark room; PAPER is the same glass in
// daylight against a pale wall, for stills.
#if defined(VARIANT_PAPER)
const vec3 W_TOP = vec3(.930, .915, .900), W_LOW = vec3(.985, .972, .955), W_POOL = vec3(1., .985, .965);
const float LAMPS = .55, FILM = 1.0, BODY = .35, BOKEH = 0.;
#elif defined(VARIANT_NOIR)
const vec3 W_TOP = vec3(.004, .004, .005), W_LOW = vec3(.010, .010, .012), W_POOL = vec3(.030, .030, .036);
const float LAMPS = 1., FILM = 1., BODY = .25, BOKEH = 0.;
#else
const vec3 W_TOP = vec3(.006, .007, .016), W_LOW = vec3(.020, .018, .050), W_POOL = vec3(.090, .060, .200);
const float LAMPS = 1., FILM = 1., BODY = 1., BOKEH = 1.;
#endif

// Each ribbon is found by looking along the ray for where it passes through the sheet,
// sampling at SCAN places; a still can afford more of them than a live scene.
#ifdef SK_STILL
const int SCAN = 48;
#else
const int SCAN = 18;
#endif
const float STRIDE = .5;                            // only for the soft shadow of the pale colourway

vec4 warp;                                          // the slow motion of the two ribbons, set once per pixel

mat2 rot(float a){ float c = cos(a), s = sin(a); return mat2(c, -s, s, c); }

// A ribbon of glass runs along x and turns about its own axis as it goes. For ribbon
// `which` (0 the near one, 1 the far one) this gives where q lies in the ribbon's own
// cross-section: x across its width, y out of its face. The sheet is where y is 0.
vec2 section(vec3 q, float which){
  if (which < .5) {
    q.xy = rot(.26) * q.xy;
    q.y -= .10 * sin(q.x * .85 + warp.x) - .10;
    q.z -= .16 * cos(q.x * .60 + warp.y);
    return rot(q.x * 1.55 + warp.z) * q.yz;
  }
  q -= vec3(0., -.08, -1.1);
  q.xy = rot(-.34) * q.xy;
  q.y -= .14 * sin(q.x * .7 + 1.7 - warp.y);
  return rot(q.x * -1.9 + 2.2 + warp.w) * q.yz;
}

// Distance to the two ribbons as solid bands, for the shadow they cast.
float dist(vec3 q){
  vec2 a = abs(section(q, 0.)) - vec2(.36, .026), b = abs(section(q, 1.)) - vec2(.22, .020);
  return min(length(max(a, 0.)) + min(max(a.x, a.y), 0.), length(max(b, 0.)) + min(max(b.x, b.y), 0.)) * STRIDE - .020 * STRIDE;
}

// The room as the glass reflects it: broad soft light from above and from the left, a
// long narrow lamp overhead and another low on the right for the sharp streaks.
vec3 lamps(vec3 d){
  vec3 c = vec3(.020, .026, .060);
  c += vec3(.62, .70, 1.) * .85 * smoothstep(-.2, .9, d.y);
  c += vec3(1., .80, .64) * .85 * smoothstep(.2, -.9, d.x) * smoothstep(-.7, .3, d.y);
  c += vec3(1., .98, .95) * 2.6 * smoothstep(.56, .62, d.y) * smoothstep(.74, .68, d.y) * smoothstep(.95, .5, abs(d.x));
  c += vec3(.70, .85, 1.) * 1.8 * smoothstep(.60, .65, d.x) * smoothstep(.76, .71, d.x) * smoothstep(.5, .0, abs(d.y + .15));
  return c * LAMPS;
}

// The wall far behind: near black with a pool of deep colour, and a few points of light
// far out of focus.
vec3 wall(vec2 x){
  vec3 c = mix(W_LOW, W_TOP, smoothstep(-.5, .5, x.y));
  vec2 k = (x - vec2(.14, -.10)) * vec2(.55, 1.);
  c = mix(c, W_POOL, exp(-dot(k, k) * 3.2));
  if (BOKEH > 0.) {
    for (int i = 0; i < 6; i++) {
      float fi = float(i);
      vec2 o = vec2(fract(fi * .618 + .13) * 4.4 - 2.2, .26 * sin(fi * 2.4 + .7) - .12);
      float r = .050 + .045 * fract(fi * .37);
      float disc = smoothstep(r, r * .45, length(x - o));
      c += mix(vec3(.42, .20, .70), vec3(.12, .36, .70), fract(fi * .5)) * disc * (.16 + .12 * fract(fi * .77));
    }
  }
  return c;
}

// The colour the coating gives back: teal seen straight on, through blue, violet and
// rose to amber at a glancing angle.
vec3 film(float x){
#if defined(VARIANT_NOIR)
  return mix(vec3(.30, .31, .34), vec3(1., .42, .10), smoothstep(.62, 1., x));    // black glass: grey, with one amber edge
#endif
  vec3 c = mix(vec3(.04, .58, .72), vec3(.14, .30, 1.), smoothstep(.0, .30, x));
  c = mix(c, vec3(.70, .22, .96), smoothstep(.24, .54, x));
  c = mix(c, vec3(1., .34, .46), smoothstep(.48, .78, x));
  return mix(c, vec3(1., .74, .32), smoothstep(.72, 1., x));
}

vec3 lamps(vec3 d);
vec3 film(float x);

// One ribbon along the ray ro + rd * t, between ta and tb: every place the ray goes
// through the sheet adds what the glass sends back to `acc` and takes from `tr` what it
// keeps. `fw` is how wide a pixel is per unit of t, so edges can be feathered by how much
// of the pixel the glass covers; a ray that only brushes past a fold gets the fold's shine.
void ribbon(float which, float wide, vec3 ro, vec3 rd, float ta, float tb, float fw,
            inout vec3 acc, inout vec3 tr, inout vec3 n1, inout float hits){
  float dt = (tb - ta) / float(SCAN);
  vec2 sp = section(ro + rd * ta, which);
  float tp = ta, fpp = sp.y;
  for (int i = 1; i <= SCAN; i++) {
    float t = ta + dt * float(i);
    vec2 sc = section(ro + rd * t, which);
    if (sp.y * sc.y < 0.) {
      // Through the sheet between the last sample and this one: close in on the place.
      float a = tp, b = t, fa = sp.y, fb = sc.y;
      for (int k = 0; k < 3; k++) {
        float m = a - fa * (b - a) / (fb - fa), fm = section(ro + rd * m, which).y;
        if (fa * fm < 0.) { b = m; fb = fm; } else { a = m; fa = fm; }
      }
      float th = a - fa * (b - a) / (fb - fa);
      vec3 q = ro + rd * th;
      vec2 at = section(q, which);
      const float e = .002;
      vec3 n = normalize(vec3(section(q + vec3(e, 0., 0.), which).y, section(q + vec3(0., e, 0.), which).y, section(q + vec3(0., 0., e), which).y) - at.y);
      if (dot(n, rd) > 0.) n = -n;
      float c = clamp(-dot(n, rd), 0., 1.);
      float pixel = fw * th;
      // How much of the pixel the ribbon covers at its two long edges.
      float cover = smoothstep(wide + pixel, wide - pixel, abs(at.x));
      if (cover > 0.) {
        if (hits < .5) n1 = n * cover;
        float rim = smoothstep(wide - .016, wide - .003, abs(at.x));        // the rounded edge of the glass
        float fres = .05 + .95 * pow(1. - c, 5.);
        float g = pow(1. - c, .85) + .12 * sin(q.x * 1.3 + which * 2.) + .08 + .012 * sin(at.x * 64.);
        vec3 fc = film(g);
        // The coating sends back the lamps in its own colour; the bare edge of the glass sends them back white.
        vec3 back = lamps(reflect(rd, n)) * mix(fc, vec3(1.), fres * .6) * (.34 + .66 * fres) * FILM;
        back += LAMPS * (mix(fc, vec3(1.), .75) * 1.1 * max(pow(1. - c, 7.), rim * .45) + fc * .045);
        acc += tr * back * cover;
        // The way through: longer at a glancing angle and at the edge, where the glass keeps more and its body glows.
        float len = min(.075 / max(c, .05) + .22 * rim, .8);
        vec3 bc = film(.34 + .30 * which);
        vec3 keep = mix(vec3(1.), 1.04 - .80 * fc, .60) * (1. - .55 * fres) * exp(-len * (1.1 - bc) * 2.2 * BODY);
        acc += tr * keep * bc * BODY * .9 * (1. - exp(-max(len - .09, 0.) * 3.5)) * cover;
        tr *= mix(vec3(1.), keep, cover);
        hits += 1.;
      }
    } else if (i > 1 && fpp * sp.y > 0. && abs(sp.y) <= abs(fpp) && abs(sp.y) < abs(sc.y)) {
      // The ray came nearest the sheet here without going through: a fold seen edge on.
      float y0 = abs(fpp), y1 = abs(sp.y), y2 = abs(sc.y);
      float near = max(y1 - (y0 - y2) * (y0 - y2) / (8. * max(y0 - 2. * y1 + y2, 1e-6)), 0.);
      float pixel = fw * tp;
      float w = smoothstep(1.5 * pixel, 0., near) * smoothstep(wide + pixel, wide - pixel, abs(sp.x));
      acc += tr * LAMPS * mix(film(.95), vec3(1.), .75) * 1.1 * w;
      tr *= 1. - .5 * w;
    }
    fpp = sp.y; sp = sc; tp = t;
  }
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  warp = vec4(.5 * orbit(1., 0.).x, .5 * orbit(1., 1.9).y, .40 * orbit(1., .8).x + 1.25, .5 * orbit(1., 3.0).y);

  vec3 ro = vec3(0., 0., 3.4), rd = normalize(vec3(p, -2.3));
  vec3 acc = vec3(0.), tr = vec3(1.), n1 = vec3(0.);
  float hits = 0., fw = px / 2.3 / -rd.z;
  // The near ribbon lies within .62 of z = 0 and the far one within .42 of z = -1.1, so the near one is always met first.
  ribbon(0., .38, ro, rd, (ro.z - .62) / -rd.z, (ro.z + .62) / -rd.z, fw, acc, tr, n1, hits);
  ribbon(1., .24, ro, rd, (ro.z + .68) / -rd.z, (ro.z + 1.52) / -rd.z, fw, acc, tr, n1, hits);
  // What is behind shows through, each colour bent a little differently by the first surface.
  vec2 bend = n1.xy;
  vec3 back = vec3(wall(p - bend * .085).r, wall(p - bend * .105).g, wall(p - bend * .128).b);
#if defined(VARIANT_PAPER)
  // On the pale wall the glass also casts a soft shadow, tinted by what it lets through.
  vec3 w = ro + rd * ((ro.z + 2.1) / -rd.z), sun = normalize(vec3(-.30, .50, 1.));
  float lit = 1., s = .25;
  for (int i = 0; i < 64; i++) {
    float d = dist(w + sun * s) / STRIDE;
    lit = min(lit, .5 + .5 * d / (s * .10));
    s += clamp(d * .5, .012, .07);
  }
  lit = smoothstep(0., 1., clamp(lit, 0., 1.));
  back *= mix(vec3(.80, .78, .86), vec3(1.), lit);
#endif
  return acc + tr * back;
}
