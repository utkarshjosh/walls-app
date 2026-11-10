#define DITHER 1.0

const mat2 M = mat2(.8, -.6, .6, .8);
float fbm2(vec2 p){ return .60 * gn(p) + .30 * gn(M * p * 2.03 + vec2(3.1, 7.7)); }
float fbm4(vec2 p){
  float s = 0., a = .5;
  for (int i = 0; i < 4; i++) { s += a * gn(p); p = M * p * 2.03 + vec2(3.1, 7.7); a *= .55; }
  return s;
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  // Work in a tilted frame, squeezed across the flow, so the ink is drawn out into
  // long soft plumes rather than round blots.
  vec2 r = mat2(.82, .57, -.57, .82) * p;
  vec2 q = r * vec2(1.25, .55) + vec2(7.3, 2.9);
  vec2 o1 = orbit(1., 0.), o2 = orbit(1., 2.6), o3 = orbit(2., 4.4);

  vec2 w = vec2(fbm2(q * .7 + .10 * o1), fbm2(q * .7 + vec2(4.1, 2.7) + .10 * o1.yx));
  vec2 v = vec2(fbm2(q + 1.7 * w + vec2(1.7, 9.2) + .06 * o2), fbm2(q + 1.7 * w + vec2(8.3, 2.8) - .06 * o2.yx));
  float f = fbm4(q * 1.2 + 1.6 * v + .03 * o3);

  // Ink gathers low and to the right; most of the sheet stays bare.
  float lean = dot(p, vec2(.17, -.30)) - .13;
  float d = clamp((f * 1.25 + .65 * v.x + lean + .20) / .74, 0., 1.);
  d = d * d * d * (d * (d * 6. - 15.) + 10.);
  float veil = smoothstep(-.05, .45, w.y * 1.6 + lean + .2);   // a paler wash lying behind

  vec3 paper = vec3(.928, .905, .858);
  vec3 wash  = vec3(.680, .666, .642);
  vec3 ink   = vec3(.215, .218, .232);
  vec3 col = mix(paper, wash, veil * .16);
  col = mix(col, wash, smoothstep(.0, .55, d));
  col = mix(col, ink, smoothstep(.42, 1., d) * .9);
  return col;
}
