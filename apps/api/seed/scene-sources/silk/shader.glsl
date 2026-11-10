
#define DITHER 1.0

// Colourways. The first is the live scene: ink blue turning to plum. The others are for
// stills (scenekit.py still silk --variant NAME). Each is one colour family from its deepest
// shadow (C_INK) to its highlight (C_PALE), with a second, warmer set the cloth drifts into.
#if defined(VARIANT_EMBER)
const vec3 C_INK = vec3(.050, .010, .014), C_DEEP = vec3(.250, .030, .040), C_MID = vec3(.560, .110, .070);
const vec3 C_LITE = vec3(.930, .480, .300), C_PALE = vec3(1., .900, .800);
const vec3 W_INK = vec3(.045, .012, .030), W_DEEP = vec3(.230, .028, .090), W_MID = vec3(.520, .080, .150);
const vec3 C_SIDE = vec3(.420, .060, .130), C_FOLD = vec3(.110, .010, .040);
#elif defined(VARIANT_JADE)
const vec3 C_INK = vec3(.006, .034, .036), C_DEEP = vec3(.016, .150, .140), C_MID = vec3(.050, .340, .300);
const vec3 C_LITE = vec3(.420, .780, .700), C_PALE = vec3(.880, 1., .960);
const vec3 W_INK = vec3(.006, .030, .048), W_DEEP = vec3(.014, .120, .180), W_MID = vec3(.040, .270, .380);
const vec3 C_SIDE = vec3(.040, .250, .330), C_FOLD = vec3(.004, .050, .070);
#elif defined(VARIANT_NOIR)
const vec3 C_INK = vec3(.004, .004, .005), C_DEEP = vec3(.016, .016, .019), C_MID = vec3(.050, .051, .058);
const vec3 C_LITE = vec3(.400, .410, .440), C_PALE = vec3(1., .985, .96);
const vec3 W_INK = vec3(.005, .004, .004), W_DEEP = vec3(.019, .016, .015), W_MID = vec3(.058, .050, .046);
const vec3 C_SIDE = vec3(.020, .200, .340), C_FOLD = vec3(.002, .020, .040);
#elif defined(VARIANT_PEARL)
const vec3 C_INK = vec3(.420, .350, .320), C_DEEP = vec3(.660, .590, .550), C_MID = vec3(.830, .780, .740);
const vec3 C_LITE = vec3(.945, .915, .880), C_PALE = vec3(1., .990, .970);
const vec3 W_INK = vec3(.430, .330, .330), W_DEEP = vec3(.690, .580, .570), W_MID = vec3(.860, .770, .750);
const vec3 C_SIDE = vec3(.800, .690, .660), C_FOLD = vec3(.520, .420, .400);
#else
const vec3 C_INK = vec3(.010, .016, .058), C_DEEP = vec3(.036, .068, .275), C_MID = vec3(.105, .155, .530);
const vec3 C_LITE = vec3(.540, .460, .900), C_PALE = vec3(1., .860, .930);
const vec3 W_INK = vec3(.030, .010, .050), W_DEEP = vec3(.105, .050, .270), W_MID = vec3(.300, .150, .520);
const vec3 C_SIDE = vec3(.420, .100, .420), C_FOLD = vec3(.130, .026, .150);
#endif

vec2 weft;                                          // where the last point asked about lies in the weave: across and along the folds

// Height of the cloth at p. Folds are round arches lying side by side that meet in narrow
// valleys, as satin does when it is gathered. They fan out from a point far off the upper
// right corner, in three sizes, and each swells and dies away along its length.
float cloth(vec2 p, vec2 o){
  vec2 w = vec2(gn(p * .34 + vec2(1.7, 9.2) + .20 * o), gn(p * .34 + vec2(8.3, 2.8) - .20 * o.yx));
  vec2 c = p + 1.05 * w - vec2(4.6, 3.9);
  float r = length(c), th = atan(c.y, c.x);
  float e1 = .62 + .8 * gn(vec2(th * 13., r * .42) + .18 * o);
  float e2 = smoothstep(-.22, .62, gn(vec2(th * 22. + 4., r * .60) - .18 * o.yx));
  float a = sin(th * 44. + .9 * sin(r * 1.0 + .6 * o.x) + 1.2 * w.x);
  float b = sin(th * 77. + r * 1.3 + 2.1 + .5 * o.y);
  float d = sin(th * 139. - r * 2.2 + 3. * w.y);
  weft = vec2(th, r);
  return e1 * sqrt(a * a + .03) + .36 * e2 * sqrt(b * b + .05) + .10 * e1 * e2 * d + .5 * w.y;
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  vec2 o = orbit(1., .6);
  vec2 q = p * 1.1 + vec2(.3, -.1);
  const float e = .006;
  const float S = .16;                              // how tall the folds are
  float hx = cloth(q + vec2(e, 0.), o), hy = cloth(q + vec2(0., e), o);
  float h0 = cloth(q, o);
  vec2 wf = weft;
  hx = (hx - h0) / e; hy = (hy - h0) / e;
  vec3 n = normalize(vec3(-hx * S, -hy * S, 1.));

  vec3 L = normalize(vec3(-.55, .52, .65));
  vec3 H = normalize(L + vec3(0., 0., 1.));

  // Shadow: walk toward the light and see whether the cloth rises above the ray.
  float sh = 1.;
  for (int i = 1; i <= 5; i++) {
    float t = .016 * float(i * i) + .012;
    float rise = (cloth(q + L.xy * t, o) - h0) * S - L.z * t * .55;
    sh = min(sh, 1. - clamp(rise * 26., 0., 1.));
  }
  sh = mix(.25, 1., sh);

  float dif = max(dot(n, L), 0.);
  float nh = max(dot(n, H), 0.);
  float broad = pow(nh, 9.);
  float tight = pow(nh, 70.);
  float graze = 1. - n.z;                            // 0 on flat cloth, toward 1 on a steep side

  // The light is a soft pool right of centre; the upper edge of the picture stays quiet.
  vec2 pc = (uv - vec2(.60, .62)) * vec2(1., .85);
  float pool = exp(-dot(pc, pc) * 2.6) * smoothstep(-.05, .30, uv.y);

  // The weave: threads far finer than a live scene can show, there for a still seen close.
  float fine = smoothstep(.00042, .00016, px);
  float thread = sin(wf.x * 31400.) * sin(wf.y * 5700. + wf.x * 9000.);
  broad *= 1. + .16 * thread * fine;
  tight *= 1. + .55 * thread * fine;

  float I = (.07 + .50 * dif + .50 * broad) * sh;
  I *= .40 + .60 * smoothstep(.0, .9, h0);          // the valleys keep less light
  I *= .30 + .78 * pool;

  // The cooler colours where the light is thin, the warmer ones on steep sides and lower right.
  float warm = smoothstep(-.35, .45, gn(q * .33 + vec2(5.1, 2.2) + .12 * o) + .55 * (uv.x - .5) + .45 * (uv.y - .5));
  vec3 ink = mix(C_INK, W_INK, warm * .6), deep = mix(C_DEEP, W_DEEP, warm * .8), mid = mix(C_MID, W_MID, warm * .8);
  vec3 col = mix(ink, deep, smoothstep(.0, .26, I));
  col = mix(col, mid, smoothstep(.20, .58, I));
  col = mix(col, C_LITE, smoothstep(.52, 1.0, I));
  col = mix(col, mix(C_FOLD, C_SIDE, smoothstep(.05, .5, I)), smoothstep(.10, .55, graze) * (.25 + .45 * warm));
  col += C_PALE * tight * sh * (.10 + .30 * pool);
  return col;
}
