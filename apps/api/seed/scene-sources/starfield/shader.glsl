// Starfield - a slow drift through sparse stars, with a faint nebula far behind them.
// Written for Still in the common web style (mainImage, iTime, iResolution).
// Everything repeats exactly every LOOP seconds, so it can run for ever without a jump.

const float LOOP = 480.;
const float TAU = 6.2831853;
const int SHEETS = 7;

float h1(vec2 p){ vec3 q = fract(vec3(p.xyx) * .1031); q += dot(q, q.yzx + 33.33); return fract((q.x + q.y) * q.z); }
vec2 h2(vec2 p){ vec3 q = fract(vec3(p.xyx) * vec3(.1031, .1030, .0973)); q += dot(q, q.yzx + 33.33); return fract((q.xx + q.yz) * q.zy); }

float gn(vec2 p){
  vec2 i = floor(p), f = fract(p), u = f * f * f * (f * (f * 6. - 15.) + 10.);
  float a = dot(h2(i) * 2. - 1., f);
  float b = dot(h2(i + vec2(1., 0.)) * 2. - 1., f - vec2(1., 0.));
  float c = dot(h2(i + vec2(0., 1.)) * 2. - 1., f - vec2(0., 1.));
  float d = dot(h2(i + vec2(1., 1.)) * 2. - 1., f - vec2(1., 1.));
  return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
}

float fbm(vec2 p){
  float s = 0., a = .5;
  for (int i = 0; i < 4; i++) { s += a * gn(p); p = mat2(.8, -.6, .6, .8) * p * 2.03 + vec2(3.1, 7.7); a *= .5; }
  return s;
}

// Seven thin sheets of stars at different depths. Each one swells slowly as it comes
// nearer, fades in far away and fades out before it passes: a gentle forward drift.
vec3 stars(vec2 p, float pt){
  vec3 sum = vec3(0.);
  for (int i = 0; i < SHEETS; i++) {
    float fi = float(i);
    float z = fract(fi / float(SHEETS) + iTime / LOOP);          // 0 far .. 1 near
    float s = .62 * exp(1.15 * z);                                // how much this sheet is magnified
    float a = smoothstep(0., .18, z) * smoothstep(1., .78, z);
    float ca = cos(fi * 1.1), sa = sin(fi * 1.1);
    vec2 q = mat2(ca, -sa, sa, ca) * p / s * 7. + fi * 17.31;     // which cell of the sheet, and where in it
    vec2 id = floor(q), f = fract(q);
    float keep = step(.74, h1(id + .5));                          // most cells are empty
    vec2 pos = .12 + .76 * h2(id);
    float b = h1(id + 7.7); b *= b;
    float d = length(f - pos) * s / 7.;                           // distance on screen
    float r = pt * (1.25 + 1.3 * b) * mix(.85, 1.25, z);
    float core = exp(-d * d / (r * r));
    float halo = exp(-d / (r * 3.5)) * .22 * b;
    vec3 tint = mix(vec3(.72, .82, 1.), vec3(1., .88, .72), h1(id + 3.3));
    sum += tint * (core * (.18 + .82 * b) + halo) * a * keep * mix(.55, 1., z);
  }
  return sum;
}

void mainImage(out vec4 fragColor, in vec2 fragCoord){
  vec2 p = (fragCoord - .5 * iResolution.xy) / iResolution.y;    // centred, picture height = 1
  float pt = 1. / iResolution.y;                                  // one point in those units
  float ang = TAU * iTime / LOOP;
  vec2 o = vec2(cos(ang), sin(ang));

  // Deep sky, a touch lighter toward the middle.
  vec3 col = mix(vec3(.026, .034, .072), vec3(.010, .013, .030), smoothstep(0., .95, length(p)));

  // Nebula: two soft cloud fields, violet into teal with a little rose where it is densest,
  // gathered into a broad band that crosses the picture at a slant.
  vec2 q = p * 1.15 + vec2(3.1, 1.7);
  float n1 = fbm(q + .10 * o);
  float n2 = fbm(q * 1.7 + vec2(5.2, 9.1) - .08 * o.yx + .6 * n1);
  float across = dot(p, vec2(-.39, .92)) + .10 * n1;
  float band = exp(-across * across / .085);
  float cloud = smoothstep(-.10, .55, n1 + .5 * n2) * (.30 + .95 * band);
  vec3 neb = mix(vec3(.15, .10, .34), vec3(.06, .22, .30), smoothstep(-.2, .3, n2));
  neb = mix(neb, vec3(.38, .16, .24), smoothstep(.10, .46, n1) * .7);
  col += neb * cloud + vec3(.020, .018, .034) * band;

  col += stars(p, pt) * (1. - .30 * cloud);
  fragColor = vec4(col, 1.);
}
