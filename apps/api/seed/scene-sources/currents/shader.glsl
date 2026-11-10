#define DITHER 1.0

const mat2 M = mat2(.8, -.6, .6, .8);

// The stream function: its level lines are the paths the water follows. Warped once,
// so the lines swing round one another instead of running parallel.
float stream(vec2 p, vec2 o){
  vec2 w = vec2(gn(p * .55 + .015 * o), gn(p * .55 + vec2(4.7, 1.9) - .015 * o.yx));
  return gn(p * .62 + 1.25 * w + vec2(2.3, 5.1) + .007 * o.yx) + .32 * gn(M * p * 1.5 + .9 * w - .010 * o);
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  vec2 o = orbit(1., .7);
  vec2 q = p * 1.35 + vec2(6.1, 3.4);

  // Value and slope of the stream function, the slope by two extra samples.
  float e = px * 1.35 * 1.5;
  float s0 = stream(q, o);
  vec2 g = vec2(stream(q + vec2(e, 0.), o) - s0, stream(q + vec2(0., e), o) - s0) / e;
  float slope = length(g) * 1.35;                   // change of the stream value per picture height

  // Deep water: slate blue into teal, a little lighter where the current runs fast.
  vec3 col = mix(vec3(.050, .110, .170), vec3(.060, .200, .225), smoothstep(-.35, .35, s0));
  col = mix(col, vec3(.035, .075, .125), smoothstep(.1, 1.1, length(p * vec2(.75, 1.))) * .55);

  // Fine lines along the flow: one per step of the stream value.
  const float LINES = 46.;
  float v = s0 * LINES, id = floor(v);
  float gap = 1. / max(slope * LINES, 1e-4);        // distance between neighbouring lines, in picture heights
  float d = abs(fract(v) - .5) * gap;               // distance to the nearest line
  float w = px * (.55 + .5 * h21(vec2(id, 3.)));    // each line has its own weight
  float line = smoothstep(w + px * .9, w - px * .1, d);
  line *= smoothstep(px * 2.2, px * 6., gap);       // where lines would crowd together they fade into the water
  line *= 1. - smoothstep(.042, .085, gap);           // and where the water is nearly still they thin out, so no ring is ever born at full strength

  // Each line breathes along its length, so the eye reads strokes rather than a contour map.
  float along = gn(q * 2.3 + vec2(id * 1.7, id * .9) + .30 * orbit(2., id));
  float ink = (.30 + .70 * h21(vec2(id, 9.))) * smoothstep(-.30, .25, along);
  vec3 foam = mix(vec3(.420, .700, .690), vec3(.800, .900, .860), h21(vec2(id, 5.)));
  return mix(col, foam, line * ink * .62);
}
