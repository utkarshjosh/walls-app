
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

// A still can afford to walk the ray in smaller steps than a live scene.
#ifdef SK_STILL
const int STEPS = 160;
const float STRIDE = .34, SKIM = 1.;
#else
const int STEPS = 52;
const float STRIDE = .62, SKIM = .45;
#endif

vec4 warp;                                          // the slow motion of the two ribbons, set once per pixel

mat2 rot(float a){ float c = cos(a), s = sin(a); return mat2(c, -s, s, c); }

// A ribbon of glass along x: a flat band with rounded edges that turns about its own
// axis as it goes. Returns the distance, and in `across` where q is across and through the band.
float band(vec3 q, float turn, float ph, float wide, float thin, out vec2 across){
  across = rot(q.x * turn + ph) * q.yz;
  vec2 d = abs(across) - vec2(wide, thin);
  return (length(max(d, 0.)) + min(max(d.x, d.y), 0.) - .020) * STRIDE;
}

float map(vec3 q, out vec2 across, out float which){
  vec2 a2;
  vec3 a = q;
  a.xy = rot(.26) * a.xy;
  a.y -= .10 * sin(a.x * .85 + warp.x) - .10;
  a.z -= .16 * cos(a.x * .60 + warp.y);
  float d1 = band(a, 1.55, warp.z, .36, .026, across);
  vec3 b = q - vec3(0., -.08, -1.1);
  b.xy = rot(-.34) * b.xy;
  b.y -= .14 * sin(b.x * .7 + 1.7 - warp.y);
  float d2 = band(b, -1.9, 2.2 + warp.w, .22, .020, a2);
  which = d1 < d2 ? 0. : 1.;
  if (d2 < d1) across = a2;
  return min(d1, d2);
}

float dist(vec3 q){ vec2 a; float w; return map(q, a, w); }

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

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  warp = vec4(.5 * orbit(1., 0.).x, .5 * orbit(1., 1.9).y, .40 * orbit(1., .8).x + 1.25, .5 * orbit(1., 3.0).y);

  vec3 ro = vec3(0., 0., 3.4), rd = normalize(vec3(p, -2.3));
  vec3 acc = vec3(0.), tr = vec3(1.), n1 = vec3(0.);
  float t = 1.9, hits = 0., skim = 1., within = -1.;            // within: which ribbon the ray is passing through, if any
  bool outside = true;
  vec2 across; float which;
  for (int i = 0; i < STEPS; i++) {
    vec3 q = ro + rd * t;
    float d = map(q, across, which);
    if (d >= .0024 * STRIDE) {
      outside = true; within = -1.; skim = d;
      t += d;
      if (t > 6.2) break;
    } else if (outside || which != within) {
      const vec2 e = vec2(.0025, -.0025);
      vec3 n = normalize(e.xyy * dist(q + e.xyy) + e.yyx * dist(q + e.yyx) + e.yxy * dist(q + e.yxy) + e.xxx * dist(q + e.xxx));
      if (hits < .5) n1 = n;
      float c = clamp(-dot(n, rd), 0., 1.);
      float fres = .05 + .95 * pow(1. - c, 5.);
      float g = pow(1. - c, .85) + .12 * sin(q.x * 1.3 + which * 2.) + .08 + .012 * sin(across.x * 64.);
      vec3 fc = film(g);
      // The coating sends back the lamps in its own colour; the bare edge of the glass sends them back white.
      acc += tr * lamps(reflect(rd, n)) * mix(fc, vec3(1.), fres * .6) * (.34 + .66 * fres) * FILM;
      acc += tr * LAMPS * (mix(fc, vec3(1.), .75) * 1.1 * pow(1. - c, 7.) + fc * .045);
      // The way through: longer at a glancing angle, where the glass keeps more and its body glows.
      float len = min(.075 / max(c, .05), .8);
      tr *= mix(vec3(1.), 1.04 - .80 * fc, .60) * (1. - .55 * fres);
      vec3 bc = film(.34 + .30 * which);
      tr *= exp(-len * (1.1 - bc) * 2.2 * BODY);
      acc += tr * bc * BODY * .9 * (1. - exp(-max(len - .09, 0.) * 3.5));
      hits += 1.;
      if (hits > 2.5) break;
      outside = false; within = which;
      t += min(len, .05) + .01;
    } else {
      t += max(-d / STRIDE, .03);
    }
  }
  // A ray that ran out of steps skimming along a surface is looking at the very edge of
  // the glass, which is where it shines: without this the edge would be ragged.
  if (outside && t < 6.2) {
    float w = smoothstep(.010, .002, skim / STRIDE) * SKIM;
    acc += tr * LAMPS * mix(film(.95), vec3(1.), .75) * 1.1 * w;
    tr *= 1. - .5 * w;
  }
  // What is behind shows through, each colour bent a little differently by the first surface.
  vec2 bend = n1.xy * (hits > .5 ? 1. : 0.);
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
