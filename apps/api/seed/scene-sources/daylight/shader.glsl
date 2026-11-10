
#define DITHER 1.0

// The loop of this scene is the local day: uT is 0 at midnight and .5 at noon. The sun
// rises on the left at six, is out of the picture overhead at midday and sets on the right
// at six in the evening; the moon keeps the opposite hours.

const mat2 M2 = mat2(.8, -.6, .6, .8);
const float SHORE = -.215;                          // height of the far shore of the lake in the picture

float sunUp, dusk, day, gold, night;                // the hour, as a few weights, set once per pixel
vec2 sunAt, moonAt;
vec3 skyTop, skyLow, sunCol, haze;

float fbm(vec2 p){
  float s = 0., a = .5;
  for (int i = 0; i < 4; i++) { s += a * gn(p); p = M2 * p * 2.03 + vec2(3.1, 7.7); a *= .5; }
  return s;
}

float stars(vec2 p, float px, float sc){
  vec2 g = p * sc, id = floor(g);
  vec2 r = h22(id + 7.);
  float b = h21(id + 19.7);
  float keep = step(.86, h21(id + 3.1));
  vec2 d = (fract(g) - (.15 + .7 * r)) / sc;
  float rad = max(px * (.75 + .5 * b), .00055);
  return keep * (.08 + .92 * b * b * b) * exp(-dot(d, d) / (rad * rad));
}

void hour(float asp){
  float a = TAU * (uT - .25);
  sunUp = sin(a);                                    // -1 at midnight, 0 at six, 1 at noon
  float evening = step(.5, uT);
  night = 1. - smoothstep(-.30, -.04, sunUp);
  dusk = smoothstep(-.34, -.02, sunUp);
  day = smoothstep(-.04, .34, sunUp);
  gold = smoothstep(-.20, .02, sunUp) * (1. - smoothstep(.10, .46, sunUp));
  sunAt = vec2((-.36 + .72 * clamp((uT - .25) * 2., 0., 1.)) * asp, SHORE + .060 + .86 * sunUp);
  float mp = fract(uT + .25) ;                       // 0 at six in the evening, .5 at midnight
  moonAt = vec2((-.30 + .60 * clamp(mp * 2., 0., 1.)) * asp, SHORE + .05 + .42 * sin(TAU * mp * .5));
  vec3 twTop = mix(vec3(.105, .130, .300), vec3(.090, .105, .270), evening);
  vec3 twLow = mix(vec3(.960, .560, .420), vec3(.990, .470, .300), evening);
  skyTop = mix(mix(vec3(.012, .020, .052), twTop, dusk), vec3(.170, .380, .720), day);
  skyLow = mix(mix(vec3(.040, .070, .140), twLow, dusk), vec3(.720, .840, .930), day);
  skyLow = mix(skyLow, mix(vec3(1., .760, .520), vec3(1., .640, .420), evening), gold * .75);
  sunCol = mix(vec3(1., .44, .26), vec3(1., .96, .86), smoothstep(.0, .5, sunUp));
  haze = mix(skyLow, skyTop, .22 + .30 * gold);
}

// The sky at p: its colours for the hour, the sun and its glow, the moon, stars, and high thin cloud.
vec3 sky(vec2 p, float px, float asp){
  float e = clamp((p.y - SHORE) / (.5 - SHORE), 0., 1.);
  vec3 c = mix(skyLow, skyTop, pow(e, .62));
  // Glow round the sun, wide and warm when it is low.
  vec2 ds = p - sunAt;
  float sd = length(ds * vec2(.75, 1.));
  float up = smoothstep(-.30, -.05, sunUp);
  c += sunCol * up * (.55 * exp(-sd / .11) + .30 * exp(-sd / .32) * (.4 + .6 * gold));
  c = mix(c, c + vec3(.30, .10, .02), gold * exp(-abs(p.y - SHORE) / .10) * exp(-abs(ds.x) / (.7 * asp)) * .8);
  // Stars and the moon.
  float st = stars(p, px, 23.) + .8 * stars(M2 * p + 4.3, px, 37.) + .5 * stars(M2 * M2 * p + 2.2, px, 61.);
  float milky = smoothstep(-.15, .5, fbm(p * vec2(1., 2.) + 3.) + .3 - abs(dot(p, vec2(.5, .87)) - .30) * 2.5);
  c += (st * mix(vec3(.76, .85, 1.), vec3(1., .92, .80), h21(floor(p * 23.))) * (.55 + .7 * milky) + vec3(.014, .017, .034) * milky) * night * smoothstep(.02, .25, e);
  vec2 dm = p - moonAt;
  float md = length(dm), mr = .017;
  float moon = smoothstep(mr + px, mr - px, md);
  float seas = .78 + .22 * smoothstep(-.2, .2, gn(dm * 95. + 3.)) - .10 * smoothstep(.1, .3, gn(dm * 210. + 9.));
  float mlit = night * .92 + .10;
  c = mix(c, vec3(.96, .95, .90) * seas * mlit + c * (1. - mlit), moon * smoothstep(-.02, .03, moonAt.y - SHORE) * (1. - day));
  c += vec3(.36, .46, .66) * night * (.22 * exp(-md / .05) + .10 * exp(-md / .20));
  // The sun's disc.
  float disc = smoothstep(.019 + px, .019 - px, length(ds));
  c = mix(c, mix(vec3(1., .86, .56), vec3(1.), smoothstep(.0, .35, sunUp)), disc * up);
  // High cloud: long thin streaks that take the colour of the hour.
  vec2 cp = vec2(p.x * .55, p.y * 3.4) + 3. * orbit(1., .5) + .10 * orbit(96., 1.);
  float cl = smoothstep(.05, .55, fbm(cp + .9 * vec2(gn(cp * .6), gn(cp * .6 + 4.)))) * smoothstep(.0, .12, e) * smoothstep(1., .72, e);
  vec3 cc = mix(vec3(.050, .065, .115), mix(vec3(1., .62, .50), vec3(1.), day), dusk);
  cc = mix(cc, vec3(1., .70, .46), gold * .7) + sunCol * up * .35 * exp(-sd / .30);
  c = mix(c, cc, cl * (.18 + .30 * dusk));
  // Lower, long bars of cloud beyond the hills, lit from beneath when the sun is low.
  vec2 bp = vec2(p.x * .8, p.y * 8.5) + 2. * orbit(1., 3.5) + .08 * orbit(96., 4.);
  float bars = smoothstep(.12, .52, fbm(bp) + .25 * gn(bp * 3.1)) * smoothstep(.02, .10, e) * smoothstep(.52, .22, e);
  vec3 bc = mix(vec3(.060, .075, .130), mix(vec3(.90, .46, .44), vec3(.97, .98, 1.), day), dusk);
  bc = mix(bc, mix(vec3(1., .56, .34), vec3(.55, .30, .46), smoothstep(.0, .5, fbm(bp + 5.))), gold * .85) + sunCol * up * .45 * exp(-sd / .22);
  return mix(c, bc, bars * (.22 + .50 * dusk * (1. - .45 * day)));
}

// One ridge line. Smooth hills for `alp` = 0; for 1, peaks with sharp crests.
float ridge(float x, float seed, float alp){
  float soft = gn(vec2(x, seed)) + .30 * gn(vec2(x * 2.6 + 5., seed + 3.7));
  float a = 1. - abs(gn(vec2(x * .9, seed + 1.3))) * 1.9, b = 1. - abs(gn(vec2(x * 2.4 + 5., seed + 3.7))) * 1.9;
  float sharp = a * a * .80 + b * .30 + .10 * gn(vec2(x * 7.3 + 1., seed + 9.1)) + .045 * gn(vec2(x * 19., seed + 2.2)) + .020 * gn(vec2(x * 47., seed + 6.)) - .55;
  return mix(soft, sharp, alp);
}

// A row of conifers standing on the line y = 0, about `n` to the unit and `tall` high:
// above 0 inside a tree. Each has its own height and lean, and tiers of branches.
float firs(vec2 q, float n, float tall, float seed){
  float best = -1.;
  for (int k = 0; k < 2; k++) {
    float fk = float(k);
    float x = q.x * n + fk * .5 + seed, id = floor(x);
    float hgt = (.30 + .70 * h21(vec2(id, seed + fk))) * tall;
    float cx = (fract(x) - .5 - .30 * (h21(vec2(id, 7. + fk)) - .5)) / n;
    float yr = q.y / max(hgt, 1e-4);
    float tier = .50 + .50 * fract(yr * 8. + h21(vec2(id, 3.)));
    float halfw = hgt * .15 * pow(max(1. - yr, 0.), .85) * tier + .0010 * step(yr, .10);
    best = max(best, (yr > 0. && yr < 1.) ? halfw - abs(cx) : -1.);
  }
  return best;
}

// Everything above the water: sky, then five ranges from far peaks to the wooded far shore.
vec3 land(vec2 p, float px, float asp){
  vec3 col = sky(p, px, asp);
  float light = .12 + .88 * day + .22 * gold + .10 * night;
  float toSun = clamp((sunAt.x - p.x) * 3., -1., 1.);
  for (int i = 0; i < 5; i++) {
    float fi = float(i), k = fi / 4.;
    float alp = i == 0 ? 1. : (i == 1 ? .55 : 0.);
    float fr = i == 0 ? .95 : mix(1.2, 2.3, k);
    float x = p.x * fr + fi * 7.31 + 2.6;
    float amp = i == 0 ? .250 : mix(.150, .034, k);
    float top = SHORE + mix(.075, .006, k) + amp * (ridge(x, fi * 3.17 + 1., alp) + .45);
    // The wooded ranges carry a line of treetops, each tree its own height; the shading below ignores them.
    float tf = mix(230., 130., k), crown = 0.;
    if (i > 1) {
      float t1 = p.x * tf, t2 = p.x * tf * 1.73 + 4.1;
      crown = max((.35 + .65 * h21(vec2(floor(t1), fi))) * pow(max(0., 1. - abs(fract(t1) - .5) * 3.0), 1.4),
                  (.25 + .55 * h21(vec2(floor(t2), fi + 9.))) * pow(max(0., 1. - abs(fract(t2) - .5) * 3.4), 1.4)) * mix(.005, .013, k);
    }
    float d = top - p.y;
    if (d + crown > -px * 2.) {
      // Its own colour: pale blue rock far away, dark forest green close by.
      vec3 rock = mix(vec3(.270, .345, .500), vec3(.030, .075, .070), pow(k, .75)) * light + vec3(.012, .020, .042) * night;
      // Faces: below every crest the slope turned to the sun is lit and the other is in shade.
      float lean = ridge(x + .03 + d * 2.2 * toSun, fi * 3.17 + 1., alp) - ridge(x - .03 + d * 2.2 * toSun, fi * 3.17 + 1., alp);
      float ribs = gn(vec2(x * 26. + lean * 30., d * 9. + fi)) + .5 * gn(vec2(x * 61., d * 21.));        // gullies and ribs running down the faces
      float face = smoothstep(-.035, .035, -lean * toSun + .020 * ribs * alp) * (.55 + .45 * abs(toSun));
      float lit = smoothstep(-.14, .12, sunUp) * (1. - .75 * k);
      rock = mix(mix(rock, rock * .62 + vec3(.050, .040, .110) * gold, lit), rock * .95 + sunCol * (.16 + .40 * gold), face * lit);
      if (i < 2) {                                   // snow on the high ground, pink while the sun is low
        float snow = smoothstep(.14, .29, (top - SHORE) * (i == 0 ? 1. : 1.45) - d * 1.5 + .045 * ribs);
        vec3 white = mix(vec3(.74, .82, .95) * (light + .22 * night), sunCol * (.62 + .38 * face), gold * .85);
        rock = mix(rock, white * (.55 + .45 * face) * (.92 + .10 * ribs), snow * .92);
      }
      rock *= .90 + .16 * gn(vec2(p.x * 80., p.y * 120.) * mix(.45, 1., k));       // the mottling of rock and woods
      // Haze thickens toward the foot of each range, and drifts.
      float drift = gn(vec2(p.x * 2.2 + fi * 4., p.y * 9.) + .35 * orbit(288., fi * 1.7));
      float fog = (1. - .86 * k) * (.30 + .70 * smoothstep(.0, mix(.20, .06, k), max(d, 0.) + .03 * drift));
      vec3 c = mix(rock, haze + sunCol * gold * .14, clamp(fog, 0., 1.));
      c += sunCol * gold * (1. - k) * .30 * exp(-d / .010) * exp(-abs(p.x - sunAt.x) / .45);    // a rim of light on crests near the sun
      col = mix(col, c, smoothstep(-px * 1.2, px * 1.2, d + crown));
    }
  }
  return col;
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  hour(asp);
  vec3 col;
  float depth = SHORE - p.y;                         // below the far shore is water
  if (depth <= 0.) {
    col = land(p, px, asp);
  } else {
    // The lake holds the picture upside down, drawn out and broken by slow ripples that grow toward us.
    vec2 w = orbit(1440., 0.);
    float rip = gn(vec2(p.x * 7., depth * 150. / (1. + depth * 9.)) + .5 * w) + .5 * gn(vec2(p.x * 19., depth * 420. / (1. + depth * 9.)) - .5 * w.yx);
    vec2 m = vec2(p.x + rip * .010 * (depth * 5. + .08), SHORE + depth * 1.12 + rip * .016 * (depth * 5. + .05));
    col = land(m, px * 2., asp);
    vec3 deep = mix(skyTop * .45, vec3(.010, .020, .030), .5);
    col = mix(col * .86, deep, smoothstep(.0, .42, depth) * .50);
    col += haze * .05 * smoothstep(.25, .75, gn(vec2(p.x * 1.3, depth * 70.) + w * .3)) * smoothstep(.0, .1, depth);     // calm bands
    // The sun's or moon's path on the water.
    float up = smoothstep(-.10, .02, sunUp);
    vec2 lamp = up > .01 ? sunAt : moonAt;
    vec3 lampCol = up > .01 ? sunCol * up : vec3(.55, .65, .85) * night * .55;
    float pathw = .012 + depth * .30;
    float sp = smoothstep(.15, .75, rip + .25) * exp(-abs(p.x - lamp.x) / pathw) * smoothstep(-.03, .05, lamp.y - SHORE) * smoothstep(.75, .30, lamp.y - SHORE);
    col += lampCol * sp * .65 * (.4 + .6 * smoothstep(.0, .05, depth));
  }
  // Mist lying along the far shore.
  float mz = gn(vec2(p.x * 2.3, 1.) + .4 * orbit(288., 2.));
  float mist = exp(-abs(p.y - SHORE - .004 - .006 * mz) / (.007 + .010 * smoothstep(-.3, .5, mz))) * smoothstep(-.45, .35, mz);
  col = mix(col, haze + sunCol * gold * .15, mist * (.34 + .22 * day));

  // The near bank at either end of the canvas, with tall firs on it; the middle is open water.
  float ex = .5 * asp - abs(p.x);                    // distance from the nearer end
  float rise = smoothstep(.56, .0, ex);
  float bank = -.520 + .165 * rise * rise + .012 * gn(vec2(p.x * 3.1, 5.)) + .006 * gn(vec2(p.x * 13., 2.));
  float f = max(firs(vec2(p.x, p.y - bank + .006), 10., .30 * rise, 1.), firs(vec2(p.x + .031, p.y - bank + .008), 17., .19 * rise, 5.));
  float nearCover = max(smoothstep(-px, px, bank - p.y), smoothstep(-px * .8, px * .8, f));
  vec3 dark = mix(vec3(.004, .007, .014), vec3(.014, .028, .028), day) + skyLow * .025;
  return mix(col, dark, nearCover);
}
