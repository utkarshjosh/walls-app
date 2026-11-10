#define DITHER 1.2

// Sparse fixed stars: one candidate per grid cell, most cells empty, a few bright.
float stars(vec2 p, float px, float sc){
  vec2 g = p * sc, id = floor(g);
  vec2 r = h22(id + 7.);
  float b = h21(id + 19.7);
  float keep = step(.84, h21(id + 3.1));
  vec2 d = (fract(g) - (.15 + .7 * r)) / sc;
  float rad = max(px * (.85 + .5 * b), .0008);
  return keep * (.10 + .90 * b * b * b) * exp(-dot(d, d) / (rad * rad));
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  // Night sky: near black overhead, deep blue-teal toward the horizon.
  float e = clamp(uv.y, 0., 1.);
  vec3 col = mix(vec3(.008, .012, .036), vec3(.020, .052, .090), e * e);

  vec3 au = vec3(0.);
  for (int i = 0; i < 3; i++) {
    float fi = float(i);
    vec2 o = orbit(1., fi * 2.1 + .4), o2 = orbit(2., fi * 1.3 + 2.);
    float x = p.x * (1. - .10 * fi) + fi * 3.7 + 11.;

    // Lower edge of the curtain: two slow waves and a little irregularity.
    float a1 = x * 4.3 + fi * 2.4 + .9 * o.x, a2 = x * 2.0 + fi * 5.1 + .7 * o.y;
    float base = -.15 + .13 * fi + .060 * sin(a1) + .095 * sin(a2) + .04 * gn(vec2(x * 2.2, fi * 5.3) + .38 * o);
    float fold = 1. + 1.1 * abs(.258 * cos(a1) + .190 * cos(a2));   // steeper edge = sheet seen along its length
    float y = p.y - base;
    float near = smoothstep(-.34, .14, base);      // higher in the sky reads as closer: taller, brighter

    float env = smoothstep(-.22, .30, gn(vec2(x * .95, fi * 2.2) + .38 * o.yx));

    float I = 0.;
    if (y > -.060) {                               // below the edge there is only the halo: skip the rays
      // Vertical rays at three widths, leaning a touch as they climb.
      float xr = x + .05 * y + .03 * gn(vec2(x * 1.1, p.y * 1.4 + fi * 4.));
      float r1 = gn(vec2(xr * 4.6, fi * 3.3) + .40 * o2);
      float r2 = gn(vec2(xr * 15., fi * 7.7) - .32 * o2);
      float r3 = gn(vec2(xr * 44., fi * 1.9) + .25 * o2.yx);
      float rays = exp(r1 * 1.7 + r2 * 1.0 + r3 * .45 - .25);
      float tall = (.045 + .120 * rays * (.5 + .5 * env)) * mix(.55, 1.30, near);
      float rise = smoothstep(-.060, .034, y);
      float fall = exp(-max(y, 0.) / tall);
      I = rise * fall * env * (.20 + .50 * rays) * mix(.75, 1.1, near) * fold;
    }
    float glow = (.25 + .75 * env) * exp(-abs(y - .07) * 3.4) * .075;      // soft halo that ties the rays together

    float hh = clamp(y / .46, 0., 1.);
    vec3 c = mix(vec3(.22, 1.0, .56), vec3(.12, .66, .74), smoothstep(.0, .40, hh));
    c = mix(c, vec3(.50, .30, .90), smoothstep(.28, .95, hh));
    au += (c * I + vec3(.10, .55, .42) * glow) * (.78 - .16 * fi);
  }

  float lum = dot(au, vec3(.3, .6, .1));
  float st = stars(p, px, 21.) + .7 * stars(mat2(.8, -.6, .6, .8) * p + 4.3, px, 34.);
  col += st * vec3(.82, .88, 1.) * (1. - smoothstep(.0, .30, lum)) * smoothstep(1.05, .45, e);
  col += au;
  col = 1. - exp(-col * 1.25);

  // A low dark ridge for the sky to sit on.
  if (p.y < -.37) {
    float R = -.425 + .040 * gn(vec2(p.x * 1.1 + 2., 4.2)) + .014 * gn(vec2(p.x * 3.7, 8.7));
    col = mix(col, vec3(.006, .010, .020), smoothstep(px * 1.5, -px * 1.5, p.y - R));
  }
  return col;
}
