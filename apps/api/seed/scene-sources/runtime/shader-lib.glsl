#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
uniform vec4 uC;   // shared canvas: width, height, this window's x, y (CSS points, y down)
uniform vec4 uW;   // this window: width, height (CSS points), buffer width, height (pixels)
uniform float uT;  // phase of the loop, 0..1, from the wall clock
const float TAU = 6.2831853;

float h21(vec2 p){ vec3 q = fract(vec3(p.xyx) * .1031); q += dot(q, q.yzx + 33.33); return fract((q.x + q.y) * q.z); }
vec2 h22(vec2 p){ vec3 q = fract(vec3(p.xyx) * vec3(.1031, .1030, .0973)); q += dot(q, q.yzx + 33.33); return fract((q.xx + q.yz) * q.zy); }

// Gradient noise, roughly -0.6..0.6, smooth second derivative.
float gn(vec2 p){
  vec2 i = floor(p), f = fract(p), u = f * f * f * (f * (f * 6. - 15.) + 10.);
  float a = dot(h22(i) * 2. - 1., f);
  float b = dot(h22(i + vec2(1., 0.)) * 2. - 1., f - vec2(1., 0.));
  float c = dot(h22(i + vec2(0., 1.)) * 2. - 1., f - vec2(0., 1.));
  float d = dot(h22(i + vec2(1., 1.)) * 2. - 1., f - vec2(1., 1.));
  return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
}

// A point going round a unit circle k times per loop: moving noise lookups along it
// makes motion that repeats exactly when the wall clock wraps.
vec2 orbit(float k, float ph){ float a = TAU * k * uT + ph; return vec2(cos(a), sin(a)); }
