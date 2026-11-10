#define DITHER 1.0

// Height of the sand at ground position P. Creases of the folded noise are the crests.
float sand(vec2 P, vec2 o){
  vec2 q = P + .55 * vec2(gn(P * .21 + .32 * o), gn(P * .21 + vec2(7.1, 3.3) - .32 * o.yx));
  float a = gn(q * vec2(.34, .62) + vec2(3., 1.) + .042 * o.yx);
  float r = 1. - abs(a) * 2.1;
  return r * .62 + a * .40;
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  float hz = .11;                                   // horizon height
  vec3 hazeC = vec3(.952, .872, .778);

  // Pale dusty sky, brightest low and toward the sun on the left.
  float e = clamp((p.y - hz) / (.5 - hz), 0., 1.);
  vec3 sky = mix(hazeC, vec3(.770, .735, .735), pow(e, .8));
  float sun = exp(-length((p - vec2(-.42 * asp, hz + .02)) * vec2(.5, 1.)) * 2.4);
  sky += vec3(.085, .060, .020) * sun;
  vec3 col = sky;

  // Two far ridges on the horizon, nearly lost in the haze.
  for (int i = 0; i < 2; i++) {
    float fi = float(i);
    vec2 so = .10 * orbit(1., fi * 2.5 + 1.);
    float R = hz + .020 - .010 * fi + (.022 + .006 * fi) * (gn(vec2(p.x * (1.5 - .4 * fi) + fi * 6., 2.7 + fi) + so) + .5);
    vec3 c = mix(hazeC, vec3(.800, .640, .520), .16 + .14 * fi);
    c = mix(hazeC, c, smoothstep(hz - .004, hz + .022, p.y));
    col = mix(col, c + vec3(.03, .02, .0) * sun, smoothstep(px * 2.5, -px, p.y - R));
  }

  float dz = hz - p.y;
  if (dz > 0.) {
    // Flat ground seen in perspective; the lighting alone carries the relief.
    float Z = .30 / (dz + .012);
    vec2 P = vec2(p.x * Z, Z) * 2.0 + vec2(1.5, 0.);
    vec2 o = orbit(1., .3);
    float ep = .05 + 2.2 * px / ((dz + .012) * (dz + .012));   // wider samples far away: no shimmer at the horizon
    float h0 = sand(P, o);
    float hx = (sand(P + vec2(ep, 0.), o) - h0) / ep;
    float hy = (sand(P + vec2(0., ep), o) - h0) / ep;
    float ns = 1.9 * mix(1., .55, smoothstep(1.2, 4.5, Z));     // far ridges: gentler contrast
    vec3 n = normalize(vec3(-hx * ns, 1., -hy * ns));
    float l = dot(n, normalize(vec3(-.76, .26, .58)));

    vec3 shade = vec3(.455, .275, .225);
    vec3 mid   = vec3(.790, .540, .370);
    vec3 light = vec3(.985, .815, .600);
    vec3 c = mix(shade, mid, smoothstep(-.42, .20, l));
    c = mix(c, light, smoothstep(.12, .78, l));
    c *= .90 + .14 * h0;

    // Distance haze, a little uneven and drifting.
    float hzN = gn(vec2(P.x * .10, P.y * .07) + .40 * orbit(1., 2.1));
    float fog = 1. - exp(-Z * (.125 + .075 * hzN));
    c = mix(c, hazeC + vec3(.03, .02, .0) * sun, clamp(fog, 0., 1.));
    col = mix(col, c, smoothstep(0., px * 2., dz));
  }
  return col;
}
