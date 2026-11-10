#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
precision highp int;
#else
precision mediump float;
precision mediump int;
#endif
uniform vec4 uC;   // shared canvas: width, height, this window's x, y (points, y down)
uniform vec4 uW;   // this window: width, height (points), buffer width, height (pixels)
uniform vec4 uK;   // seconds now, seconds one loop earlier, blend between the two, frame number
uniform vec4 uD;   // year, month, day, seconds since midnight
#ifdef SK_ES3
out vec4 skOut;
#define SK_OUT skOut
#else
#define SK_OUT gl_FragColor
#endif

// What a shader written in the common web style expects to find. The picture it draws
// is the whole shared canvas, measured in points; this window shows its own part.
vec3 iResolution;
float iTime;
float iTimeDelta;
float iFrameRate;
int iFrame;
vec4 iMouse;
vec4 iDate;
void mainImage({{OUTQ}} vec4 fragColor, in vec2 fragCoord);

float skHash(vec2 p){ vec3 q = fract(vec3(p.xyx) * .1031); q += dot(q, q.yzx + 33.33); return fract((q.x + q.y) * q.z); }

void main(){
  vec2 s = uW.xy / uW.zw;                                              // points per buffer pixel
  vec2 pt = vec2(gl_FragCoord.x, uW.w - gl_FragCoord.y) * s + uC.zw;   // position in the shared canvas, y down
  vec2 fc = vec2(pt.x, uC.y - pt.y);                                   // the same place with y up, as the shader expects
  iResolution = vec3(uC.xy, 1.);
  iTimeDelta = 1. / 30.;
  iFrameRate = 30.;
  iFrame = int(uK.w);
  iMouse = vec4(0.);
  iDate = uD;
  vec4 c = vec4(0.);
  iTime = uK.x;
  mainImage(c, fc);
  if (uK.z > 0.) {                     // last seconds of the loop: dissolve into the picture the loop starts with
    vec4 d = vec4(0.);
    iTime = uK.y;
    mainImage(d, fc);
    c = mix(c, d, uK.z);
  }
  // Triangular dither of about one 8-bit step, so slow gradients never band.
  float n = skHash(gl_FragCoord.xy) + skHash(gl_FragCoord.xy + 31.7) - 1.;
  SK_OUT = vec4(clamp(c.rgb, 0., 1.) + n / 255., 1.);
}
// ---- the shader itself follows ----
