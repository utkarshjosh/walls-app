
void main(){
  vec2 s = uW.xy / uW.zw;                                              // CSS points per buffer pixel
  vec2 pt = vec2(gl_FragCoord.x, uW.w - gl_FragCoord.y) * s + uC.zw;   // position in the shared canvas, y down
  vec2 uv = pt / uC.xy;                                                // 0..1 across the whole canvas
  float asp = uC.x / uC.y;
  vec2 p = vec2((uv.x - .5) * asp, .5 - uv.y);                         // centred, y up, canvas height = 1
  vec3 col = scene(p, uv, asp, s.y / uC.y);
  // Triangular dither of about one 8-bit step, so slow gradients never band.
  float n = h21(gl_FragCoord.xy) + h21(gl_FragCoord.xy + 31.7) - 1.;
  gl_FragColor = vec4(col + n * DITHER / 255., 1.);
}
