
#define DITHER 1.1

// Colourways: the live scene is a blue world at sunrise; EMBER is a red one, for stills.
#if defined(VARIANT_EMBER)
const vec3 A_HOT = vec3(1., .93, .80), A_NEAR = vec3(1., .62, .26), A_MID = vec3(.95, .26, .10), A_FAR = vec3(.36, .05, .07);
const vec3 A_IN = vec3(1., .50, .16), A_DEEP = vec3(.55, .09, .05), A_GLOW = vec3(.85, .42, .20), SURF = vec3(.060, .026, .022);
const vec3 CITY = vec3(.70, .85, 1.);
#else
const vec3 A_HOT = vec3(1., .97, .90), A_NEAR = vec3(.42, .80, 1.), A_MID = vec3(.10, .34, 1.), A_FAR = vec3(.03, .06, .34);
const vec3 A_IN = vec3(1., .60, .20), A_DEEP = vec3(.80, .16, .06), A_GLOW = vec3(.20, .75, .62), SURF = vec3(.020, .034, .064);
const vec3 CITY = vec3(1., .74, .42);
#endif

const mat2 M2 = mat2(.8, -.6, .6, .8);

float fbm(vec2 p){
  float s = 0., a = .5;
  for (int i = 0; i < 4; i++) { s += a * gn(p); p = M2 * p * 2.03 + vec2(3.1, 7.7); a *= .5; }
  return s;
}

// Sparse fixed stars: one candidate per grid cell, most cells empty, a few bright.
float stars(vec2 p, float px, float sc){
  vec2 g = p * sc, id = floor(g);
  vec2 r = h22(id + 7.);
  float b = h21(id + 19.7);
  float keep = step(.90, h21(id + 3.1));
  vec2 d = (fract(g) - (.15 + .7 * r)) / sc;
  float rad = max(px * (.72 + .5 * b), .00050);
  return keep * (.06 + .94 * b * b * b) * exp(-dot(d, d) / (rad * rad));
}

// Lights of towns on the night side: small clusters, only where there is land.
float towns(vec2 g, float px, float land){
  vec2 id = floor(g), f = fract(g);
  vec2 r = h22(id + 2.7);
  float keep = step(.50, h21(id + 9.1)) * land;
  vec2 d = f - (.2 + .6 * r);
  float rad = .07 + .10 * h21(id + 4.4);
  return keep * exp(-dot(d, d) / (rad * rad)) * (.3 + .7 * h21(id + 1.3));
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  float wide = max(asp / 1.6, 1.);                  // on a pair of displays the world is as much larger
  float R = 1.75 * wide;
  vec2 c = vec2(0., -(R + .165));
  vec2 d = p - c;
  float r = length(d), h = r - R;
  // The sun stands just behind the limb, right of centre, and wanders slowly along it.
  float ths = .07 + .11 * sin(TAU * uT);
  float s = (atan(d.x, d.y) - ths) * R / wide, sa = abs(s);
  float lift = .5 + .5 * cos(TAU * uT * 2. + 1.);   // how near the sun is to rising
  float power = .80 + .35 * lift;

  // --- Space: black, sparse stars, the faintest band of dust ---
  vec3 col = vec3(.004, .005, .011);
  float dust = smoothstep(-.1, .5, fbm(p * vec2(.9, 1.6) + vec2(4., 1.)) + .35 - abs(dot(p, vec2(.30, .95)) - .18) * 2.6);
  col += vec3(.016, .018, .034) * dust;
  float st = stars(p, px, 26.) + .8 * stars(M2 * p + 4.3, px, 41.) + .5 * stars(M2 * M2 * p + 9.1, px, 67.) * (.4 + .6 * dust);
  vec3 sc = mix(vec3(.75, .84, 1.), vec3(1., .90, .76), h21(floor(p * 26.) + 5.));

  vec3 glow = vec3(0.);
  if (h > 0.) {
    float line = exp(-h / .0030), near = exp(-h / .014), mid = exp(-h / .050), far = exp(-h / .19);
    glow += A_HOT * 3.4 * line * exp(-sa / .50);
    glow += A_NEAR * 1.7 * near * exp(-sa / .60);
    glow += A_NEAR * .55 * exp(-abs(h - .0105) / .0016) * exp(-sa / .42);      // a second, finer layer of air above the first
    glow += A_MID * 1.15 * mid * exp(-sa / .74);
    glow += A_FAR * 1.5 * far * exp(-sa * sa / 1.5);
    glow *= power;
    glow += A_GLOW * .10 * exp(-abs(h - .0055) / .0022);          // air glow: a thin line all the way round
    col += st * sc * (1. - smoothstep(.0, .25, dot(glow, vec3(.3, .5, .2))));
  } else {
    // --- The night side ---
    vec2 q = d / R;
    float z = sqrt(max(1. - dot(q, q), 0.));
    vec2 sp = vec2(q.x, z) / q.y * 3.2 * wide + vec2(.5, .2) + .10 * orbit(1., 2.);       // a map of the ground under the limb
    float land = smoothstep(.02, .22, fbm(sp * .55 + 3.));
    vec2 sw = vec2(fbm(sp * .7 + 1.3), fbm(sp * .7 + vec2(5.2, 8.1)));
    float cl = smoothstep(-.10, .40, fbm(sp * 1.15 + 1.6 * sw + vec2(7.3, 2.1) + .16 * orbit(1., .3)) + .22 * gn(sp * 6.5 + 3. * sw));
    float g = -h;
    float in1 = exp(-g / .0035), in2 = exp(-g / .018), in3 = exp(-g / .075);
    float along = exp(-sa / .52) * power;
    vec3 ground = SURF * (.55 + .45 * land) * (.35 + .65 * z);
    ground = mix(ground, SURF * 2.6, cl * .60);                    // clouds, in what little light there is
    float folk = land * smoothstep(-.05, .22, gn(sp * 1.9 + 11.));
    float tw = towns(sp * 30., px, folk) + .7 * towns(M2 * sp * 47. + 3., px, folk) + .10 * folk * smoothstep(.05, .35, gn(sp * 7. + 2.));
    ground += CITY * tw * .85 * (1. - cl * .85) * smoothstep(.0, .05, z);
    col = ground;
    // Sunrise comes through the air along the limb: white at the edge, gold and red further in, and the cloud tops catch it.
    glow += A_HOT * 2.6 * in1 * exp(-sa / .50);
    glow += A_IN * (2.1 * exp(-g / .010)) * along * (.60 + .70 * cl);
    glow += A_DEEP * (1.0 * exp(-g / .032)) * along * (.30 + 1.2 * cl);
    glow += A_DEEP * (.22 * in3) * along * cl;
    glow += A_MID * .16 * exp(-g / .03) * exp(-sa * sa / 3.);
    glow += A_GLOW * .05 * exp(-g / .012);
  }

  // The sun itself, a moment before it clears the edge.
  vec2 ps = c + (R + .0015) * vec2(sin(ths), cos(ths));
  vec2 e = p - ps;
  float dd = length(e);
  float veil = h > 0. ? 1. : exp(h / .030);
  glow += A_HOT * power * veil * (2.6 * exp(-dd / .013) + .80 * exp(-dd / .055) + .20 * exp(-dd / .24));
  glow += mix(A_NEAR, A_HOT, .5) * power * veil * .22 * exp(-abs(e.y) / .0045) * exp(-abs(e.x) / (.34 * wide));

  col += glow;
  return 1. - exp(-col * 1.35);
}
