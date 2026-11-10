
#ifndef SK_DITHER
#define SK_DITHER DITHER
#endif

// A still: the same picture as the live scene, at full size. Each pixel is the average of
// SK_SS x SK_SS samples spread evenly inside it, so edges and fine lines come out clean.
void main(){
  vec2 s = uW.xy / uW.zw;                                              // canvas units per pixel (1 for a still)
  vec2 at = vec2(gl_FragCoord.x, uW.w - gl_FragCoord.y);               // this pixel inside its tile, y down
  float asp = uC.x / uC.y;
  vec3 col = vec3(0.);
  for (int j = 0; j < SK_SS; j++) {
    for (int i = 0; i < SK_SS; i++) {
      vec2 o = (vec2(float(i), float(j)) + .5) / float(SK_SS) - .5;
      vec2 pt = (at + o) * s + uC.zw;                                  // position in the whole picture, y down
      vec2 uv = pt / uC.xy;
      vec2 p = vec2((uv.x - .5) * asp, .5 - uv.y);                     // centred, y up, picture height = 1
      col += clamp(scene(p, uv, asp, s.y / uC.y / float(SK_SS)), 0., 1.);
    }
  }
  col /= float(SK_SS * SK_SS);
  // Triangular dither of about one 8-bit step, so slow gradients never band.
  vec2 g = at + uC.zw;
  float n = h21(g) + h21(g + 31.7) - 1.;
  gl_FragColor = vec4(col + n * SK_DITHER / 255., 1.);
}
