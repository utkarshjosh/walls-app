
#define DITHER 1.0

// Colourways: the live scene sets amber against electric blue; FROST is silver and ice, for stills.
#if defined(VARIANT_FROST)
const vec3 T_A = vec3(.80, .86, 1.), T_AHOT = vec3(1.), T_B = vec3(.22, .50, .95), T_BHOT = vec3(.75, .92, 1.);
#else
const vec3 T_A = vec3(1., .46, .10), T_AHOT = vec3(1., .92, .70), T_B = vec3(.06, .34, 1.), T_BHOT = vec3(.60, .90, 1.);
#endif
const float LINES = 64.;                            // threads in one sheet
const float SPAN = 2.5;                             // how far round its roll the sheet reaches, in radians

// A sheet of threads rolled part of the way round a tube that runs across the picture.
// The tube's centre line rises and falls, it swells and narrows, and the sheet turns round
// it, so the threads bunch where it turns edge on and fan out where it faces us.
// Returns the thread coordinate (x: one unit per thread, centred on 0; y: depth, 1 nearest)
// of the near side (side = 1) or the far side (-1); x is 1e3 where there is no sheet.
vec2 sheet(vec2 p, float side, vec4 k, float ph){
  float c = k.x + k.y * sin(p.x * 1.25 + ph) + .05 * sin(p.x * 2.6 - ph * 1.7 + 1.);
  float w = k.z + k.w * sin(p.x * .85 + ph * .6 + 1.3);
  float u = (p.y - c) / w;
  if (abs(u) >= 1.) return vec2(1e3, 0.);
  float a = asin(u);                                 // where on the tube, seen from the side
  float turn = p.x * 2.05 + 2.2 * ph + .8 * sin(p.x * .7 + ph);
  float phi = (side > 0. ? a : 3.14159265 - a) - turn;
  phi = mod(phi + 3.14159265, TAU) - 3.14159265;
  if (abs(phi) > SPAN * .5) return vec2(1e3, 0.);
  return vec2(phi / SPAN * LINES, side * sqrt(1. - u * u));
}

// The light of one side of one sheet at p.
vec3 threads(vec2 p, float px, float side, vec4 k, float ph, vec3 tint, vec3 hot, float seed){
  vec2 v = sheet(p, side, k, ph);
  if (v.x > 900.) return vec3(0.);
  float e = px * .75;
  vec2 vx = sheet(p + vec2(e, 0.), side, k, ph), vy = sheet(p + vec2(0., e), side, k, ph);
  if (vx.x > 900. || vy.x > 900.) return vec3(0.);
  float gap = e / max(length(vec2(vx.x - v.x, vy.x - v.x)), 1e-5);          // distance between neighbouring threads
  float id = floor(v.x + .5), d = abs(v.x - id) * gap;
  float rnd = h21(vec2(id, seed)), near = .5 + .5 * v.y;                      // 0 at the back of the tube, 1 at the front
  float w = max(px * .55, .00075) * (.7 + .9 * rnd * rnd) * (.75 + .6 * near);
  float line = smoothstep(w + px * .9, w - px * .25, d);
  float fill = clamp(2.2 * w / gap, 0., 1.);                                 // where threads crowd together they become one glow
  line = mix(fill, line, smoothstep(px * 1.6, px * 3.6, gap));
  float edge = smoothstep(LINES * .5, LINES * .5 - 2.5, abs(v.x));          // the sheet thins out at its two edges
  // Light travels slowly along each thread; a few threads carry much more than the rest.
  float run = .55 + .45 * sin(p.x * (2.2 + 2.6 * rnd) + rnd * 40. + TAU * uT * (2. + floor(rnd * 3.)) * (rnd > .5 ? 1. : -1.));
  float gain = (.16 + .84 * pow(rnd, 3.)) * (.42 + .58 * near * near) * (.35 + .65 * run);
  float halo = (exp(-d / (w * 4.)) * .30 + exp(-d / (w * 14.)) * .16 * rnd * rnd) * smoothstep(px * 2., px * 7., gap);
  vec3 c = mix(tint, hot, smoothstep(.40, 1., gain * line)) * (line + halo) * gain * 2.0;
  // Here and there a bead of light rides a thread.
  float bx = (fract(p.x * 5. + rnd * 17. + uT * 12. * (rnd > .5 ? 1. : -1.)) - .5) / 5.;
  c += hot * step(.72, h21(vec2(id, seed + 2.))) * exp(-(bx * bx + d * d) / (w * w * 7.)) * (.4 + .9 * near) * smoothstep(px * 3., px * 8., gap);
  // The sheet itself glows faintly, more where it turns away and the threads pile up.
  c += tint * .030 * (.35 + .65 * near) / (.30 + abs(v.y));
  return c * edge;
}

vec3 scene(vec2 p, vec2 uv, float asp, float px){
  float ph = .9 * orbit(1., .3).x;
  vec3 col = vec3(.006, .008, .020) + vec3(.020, .022, .050) * exp(-dot(p * vec2(.6, 1.4) + vec2(0., .12), p * vec2(.6, 1.4) + vec2(0., .12)) * 2.);
  vec4 ka = vec4(-.10, .17, .17, .085), kb = vec4(-.07, -.19, .15, .080);
  vec2 pb = p + vec2(1.9, 0.);
  // Far sides first, then the near sides over them.
  vec3 far = threads(p, px, -1., ka, ph, T_A, T_AHOT, 3.) + threads(pb, px, -1., kb, -ph + 1.2, T_B, T_BHOT, 7.);
  vec3 near = threads(pb, px, 1., kb, -ph + 1.2, T_B, T_BHOT, 7.) + threads(p, px, 1., ka, ph, T_A, T_AHOT, 3.);
  col += far * .62 + near;
  // Soft light spilling round both bundles.
  float ga = (p.y - (ka.x + ka.y * sin(p.x * 1.25 + ph))) / .34, gb = (p.y - (kb.x + kb.y * sin(pb.x * 1.25 - ph + 1.2))) / .30;
  col += T_A * .050 * exp(-ga * ga * 2.2) + T_B * .070 * exp(-gb * gb * 2.2);
  col *= smoothstep(-.02, .22, uv.y) * .45 + .55;    // quiet under the menu bar
  return 1. - exp(-col * 1.5);
}
