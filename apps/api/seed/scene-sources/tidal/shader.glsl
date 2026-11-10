#define DITHER 1.0

float fbm(vec2 p){
  return .62 * gn(p) + .30 * gn(mat2(.8, -.6, .6, .8) * p * 1.9 + vec2(3.1, 7.7));
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  vec2 q = p * .62 + vec2(11.81, 6.89);
  vec2 o1 = orbit(1., 0.), o2 = orbit(2., 1.7), o3 = orbit(1., 4.1);

  // Two rounds of domain warping: large fields that fold into one another.
  vec2 w = vec2(fbm(q + .075 * o1), fbm(q + vec2(5.2, 1.3) + .075 * o1.yx));
  vec2 v = vec2(fbm(q + 1.8 * w + vec2(1.7, 9.2) + .035 * o2), fbm(q + 1.8 * w + vec2(8.3, 2.8) - .035 * o2.yx));
  float f = fbm(q + 2.0 * v + .06 * o3);

  // One broad ramp from ink through plum and rose to champagne.
  float g = f * 1.25 + v.y * .9 + w.x * .5 + dot(p, vec2(-.10, -.22));
  g = smoothstep(-.72, .78, g);

  vec3 ink   = vec3(.045, .050, .100);
  vec3 plum  = vec3(.330, .130, .235);
  vec3 rose  = vec3(.830, .430, .430);
  vec3 coral = vec3(.940, .640, .500);
  vec3 champ = vec3(.970, .880, .740);

  vec3 warm = mix(rose, coral, smoothstep(-.25, .25, w.y - v.x));
  // Only the broad, slow field may go all the way to ink; narrow folds stop at plum.
  vec3 dark = mix(plum * .85, ink, smoothstep(-.02, .22, -w.x - .3 * dot(p, vec2(-.10, -.22))));
  vec3 col = mix(dark, plum, smoothstep(.02, .30, g));
  col = mix(col, warm, smoothstep(.22, .60, g));
  col = mix(col, champ, smoothstep(.62, 1., g));

  // A soft sheen along the folds.
  float sh = smoothstep(.05, .45, v.x - w.y * .6) * smoothstep(.25, .70, g);
  col += vec3(.10, .07, .05) * sh;
  return col;
}
