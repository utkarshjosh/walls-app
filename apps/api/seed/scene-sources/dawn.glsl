#define DITHER 1.0

// Skyline of one mountain ridge: a broad swell, some peaks, a little detail.
float ridge(float x, float s){
  float a = gn(vec2(x * .55, s * 7.3));
  float n = gn(vec2(x * 1.25 + 5., s * 3.1));
  float b = 1. - sqrt(n * n + .004) * 2.2;
  float c = gn(vec2(x * 3.1, s * 5.9));
  float d = gn(vec2(x * 7.7, s * 2.2));
  return a * .95 + b * .42 + c * .20 + d * .07;
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  // b breathes between pre-dawn (0) and sunrise (1), twice per loop.
  float b = .5 - .44 * cos(TAU * 2. * uT);
  float hz = -.17;
  float e = clamp((p.y - hz) / (.5 - hz), 0., 1.);

  vec3 top = mix(vec3(.045, .065, .165), vec3(.120, .215, .420), b);
  vec3 mid = mix(vec3(.270, .220, .410), vec3(.520, .470, .640), b);
  vec3 low = mix(vec3(.660, .400, .460), vec3(.965, .665, .475), b);
  vec3 sky = mix(low, mid, smoothstep(0., .40, e));
  sky = mix(sky, top, smoothstep(.20, 1., e));

  // Sun: no hard disc, just a warm core and a wide bloom, a little left of centre.
  vec2 sp = vec2((.37 - .5) * asp, hz + mix(-.035, .030, b));
  float r = length((p - sp) * vec2(.72, 1.));
  float core = exp(-r * r * 380.);
  float bloom = exp(-r * 5.0) * .55 + exp(-r * r * 20.) * .45;
  vec3 warm = mix(vec3(1., .56, .46), vec3(1., .82, .54), b);
  sky += warm * bloom * mix(.20, .31, b);
  sky = mix(sky, vec3(1., .955, .800), core * mix(.10, .72, b));

  // Thin high cloud streaks catching the light.
  float cl = gn(vec2(p.x * .9 + 3., p.y * 6.5 + 9.) + .22 * orbit(1., 0.));
  cl = smoothstep(.10, .55, cl) * smoothstep(.02, .14, e) * smoothstep(.80, .30, e);
  sky += (warm * .13 + .02) * cl * .6;

  vec3 haze = mix(mix(low, mid, .45), vec3(.42, .42, .62), .38);
  vec3 deep = mix(vec3(.030, .040, .095), vec3(.060, .075, .160), b);
  float aa = px * 1.5;

  // Five ridges, drawn nearest first so that everything hidden behind one is never computed.
  vec3 col = vec3(0.);
  float A = 0.;
  if (p.y < hz + .12) {
    for (int i = 4; i >= 0; i--) {
      if (A > .998) break;
      float fi = float(i), k = fi / 4.;
      float amp = .066 + .100 * k, level = hz + .020 - .300 * pow(k, 1.35);
      if (p.y > level + amp * 1.2) continue;       // above anything this ridge can reach
      float R = level + amp * ridge(p.x * (1.9 - .9 * k) + fi * 17.3, fi + 1.);
      float m = smoothstep(aa * (2.4 - 1.4 * k), -aa, p.y - R);
      float depth = R - p.y;
      vec3 c = mix(haze, deep, .16 + .84 * pow(k, 1.15));
      // Mist pooled at the foot of each ridge, drifting sideways very slowly.
      float mist = gn(vec2(p.x * 1.2 + fi * 5., fi * 3.1) + .35 * orbit(1., fi * 1.9));
      float foot = smoothstep(.0, .09 + .06 * k, depth) * (.55 + .60 * mist) * (1. - k * .80);
      c = mix(c, haze, clamp(foot, 0., 1.) * .60);
      c += warm * bloom * (1. - k) * (1. - k) * mix(.10, .22, b);
      col += (1. - A) * m * c;
      A += (1. - A) * m;
    }
  }
  return col + (1. - A) * sky;
}
