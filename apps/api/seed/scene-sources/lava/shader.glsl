// Lava - large soft blobs that rise from a pool, merge and part very slowly, like wax
// in a warm lamp. Written for Still in the common web style (mainImage, iTime, iResolution).
// Everything repeats exactly every LOOP seconds.

const float LOOP = 480.;
const float TAU = 6.2831853;
const int BLOBS = 11;

float h1(float n){ return fract(sin(n * 12.9898) * 43758.5453); }

void mainImage(out vec4 fragColor, in vec2 fragCoord){
  vec2 p = (fragCoord - .5 * iResolution.xy) / iResolution.y;    // centred, picture height = 1
  float a = TAU * iTime / LOOP;

  // A pool of wax along the lower edge, its surface swelling a little.
  float surface = -.37 + .030 * sin(p.x * 2.1 + a) + .018 * sin(p.x * 4.3 - 2. * a + 1.);
  float e = max(p.y - surface, 0.);
  float field = .95 * exp(-e * e / .010);
  float lift = field * 2. * e / .010;                             // how fast the field falls going up

  // The blobs stand in a row a little under half a picture-height apart, so a wider
  // picture simply shows more of them. Each rises out of the pool and sinks back once per loop.
  for (int i = 0; i < BLOBS; i++) {
    float fi = float(i), r1 = h1(fi + 1.), r2 = h1(fi + 31.), r3 = h1(fi + 57.);
    float up = .5 - .5 * cos(a + TAU * r2);                        // 0 in the pool .. 1 at the peak of its path
    vec2 c = vec2((fi - 5.) * .44 + .16 * (r1 - .5), -.44 + (.42 + .34 * r3) * up);
    c.x += .07 * sin(a + TAU * r3) + .03 * sin(2. * a + TAU * r1);
    float rad = .135 + .085 * r1;
    vec2 d = p - c;
    float g = exp(-dot(d, d) / (rad * rad));
    field += g;
    lift += g * 2. * d.y / (rad * rad);
  }

  // Dark warm room behind, a little lighter low down where the lamp glows.
  float y = fragCoord.y / iResolution.y;
  vec3 back = mix(vec3(.135, .060, .060), vec3(.058, .034, .050), smoothstep(0., 1., y));

  // Wax: deep wine at the skin, terracotta in the body, apricot where it is thickest.
  float body = smoothstep(.40, .64, field);
  vec3 wax = mix(vec3(.330, .100, .095), vec3(.670, .290, .185), smoothstep(.42, .95, field));
  wax = mix(wax, vec3(.910, .630, .420), smoothstep(.90, 2.00, field) * .70);
  wax *= .94 + .11 * lift / (abs(lift) + 14.);                      // skin that faces up catches a little more light
  wax *= mix(.74, 1., smoothstep(-.56, -.30, p.y));                 // and the pool darkens with depth

  // A faint warm haze around the wax, so the edge is soft rather than cut out.
  vec3 col = back + vec3(.200, .060, .035) * smoothstep(.04, .55, field) * .5;
  col = mix(col, wax, body);
  fragColor = vec4(col, 1.);
}
