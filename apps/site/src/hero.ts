// The hero: a laptop and a monitor on a desk, with one live wallpaper running
// across both. Scrolling moves through three states (see `progress`):
//   0  three-quarter view, the wallpaper spans both displays
//   1  front view, the whole picture is drawn faintly behind the displays so it
//      is plain that each display shows its own part of one picture
//   2  each display cross-fades to a wallpaper of its own
//
// Sizes are in centimetres. The wallpaper is positioned by where a pixel is in
// the room, not by each screen's own coordinates, which is what makes it line
// up across the gap between the two displays.
import * as THREE from "three";
import { RoundedBoxGeometry } from "three/addons/geometries/RoundedBoxGeometry.js";
import { RoomEnvironment } from "three/addons/environments/RoomEnvironment.js";
import lib from "../../api/seed/scene-sources/runtime/shader-lib.glsl?raw";

export type Hero = {
  /** 0..1 through the pinned hero. */
  progress(value: number): void;
  /** Stop drawing while the hero is off screen or the tab is hidden. */
  running(on: boolean): void;
  /** Play the one entrance: the displays light up left to right. */
  reveal(): void;
};

export type HeroOptions = {
  canvas: HTMLCanvasElement;
  /** The part of the canvas the displays should sit in. */
  stage: HTMLElement;
  /** The live scene that runs across both displays. */
  scene: {
    /** Its shader source, as kept in the catalog's scene-sources folder. */
    source: string;
    /** Seconds the scene takes to repeat. */
    loop: number;
    /** The moment its poster shows, used when motion is turned off. */
    still: number;
    /** The colour of the light it throws into the room, 0..1 each. */
    glow: [number, number, number];
  };
  /** Wallpapers for the "one on each" state: laptop first, then monitor. */
  each: [string, string];
  reducedMotion: boolean;
};

// ---- the room ---------------------------------------------------------------

const TILT = THREE.MathUtils.degToRad(14); // how far the laptop lid leans back

const LAPTOP = { x: -33, z: 6, w: 35.6, d: 24.8, h: 1.15, lidH: 23.6, lidT: 0.45, screenW: 34.4, screenH: 22.2 };
const HINGE = { y: LAPTOP.h, z: LAPTOP.z - LAPTOP.d / 2 };
const MONITOR = { x: 15, y: 26.5, z: -10, w: 54.4, h: 31.4, t: 1.4, screenW: 53, screenH: 29.9 };

// The rectangle (in the room, seen from the front) that holds both screens.
const laptopBottom = HINGE.y + Math.cos(TILT) * 0.85;
const laptopTop = HINGE.y + Math.cos(TILT) * (0.85 + LAPTOP.screenH);
const BOX = {
  x: LAPTOP.x - LAPTOP.screenW / 2,
  y: laptopBottom,
  w: MONITOR.x + MONITOR.screenW / 2 - (LAPTOP.x - LAPTOP.screenW / 2),
  h: MONITOR.y + MONITOR.screenH / 2 - laptopBottom,
};
void laptopTop;

const TARGET = new THREE.Vector3(-4.5, 20.2, -8);
const VIEW = {
  angled: { az: -0.4, el: 0.16 },
  front: { az: 0, el: 0.055 },
};
// The outermost points of the laptop and the monitor: what has to fit in the stage.
const lidTop = { y: HINGE.y + Math.cos(TILT) * LAPTOP.lidH, z: HINGE.z - Math.sin(TILT) * LAPTOP.lidH };
const OUTLINE: THREE.Vector3[] = [
  [LAPTOP.x - LAPTOP.w / 2, 0, LAPTOP.z + LAPTOP.d / 2],
  [LAPTOP.x + LAPTOP.w / 2, 0, LAPTOP.z + LAPTOP.d / 2],
  [LAPTOP.x - LAPTOP.w / 2, 0, HINGE.z],
  [LAPTOP.x + LAPTOP.w / 2, 0, HINGE.z],
  [LAPTOP.x - LAPTOP.w / 2, lidTop.y, lidTop.z],
  [LAPTOP.x + LAPTOP.w / 2, lidTop.y, lidTop.z],
  [MONITOR.x - MONITOR.w / 2, MONITOR.y - MONITOR.h / 2, MONITOR.z],
  [MONITOR.x + MONITOR.w / 2, MONITOR.y - MONITOR.h / 2, MONITOR.z],
  [MONITOR.x - MONITOR.w / 2, MONITOR.y + MONITOR.h / 2, MONITOR.z],
  [MONITOR.x + MONITOR.w / 2, MONITOR.y + MONITOR.h / 2, MONITOR.z],
  [MONITOR.x - 11.5, 0, MONITOR.z + 9.1],
  [MONITOR.x + 11.5, 0, MONITOR.z + 9.1],
].map(([x, y, z]) => new THREE.Vector3(x, y, z).sub(TARGET));

// ---- the wallpaper ----------------------------------------------------------

// The wallpaper on the screens is a real scene from the catalog: the same
// shader source the app runs, compiled here with the kit's shared functions.
// `scene(p, uv, asp, px)` is the kit's contract (see scene-sources/runtime).
const wallSource = (scene: string) => /* glsl */ `
${lib}
${scene}
vec3 wall(vec2 p, float asp){
  vec2 uv = vec2(p.x / asp + .5, .5 - p.y);
  return scene(p, uv, asp, max(fwidth(p.x), fwidth(p.y)));
}
`;

const VERT = /* glsl */ `
varying vec2 vUv;
varying vec3 vWorld;
void main(){
  vUv = uv;
  vec4 w = modelMatrix * vec4(position, 1.);
  vWorld = w.xyz;
  gl_Position = projectionMatrix * viewMatrix * w;
}
`;

const screenFrag = (WALL: string) => /* glsl */ `
uniform vec4 uBox;
uniform float uReveal, uEach, uImageAspect;
uniform vec2 uSize;
uniform sampler2D uImage;
varying vec2 vUv;
varying vec3 vWorld;
${WALL}
void main(){
  vec2 c = (vWorld.xy - uBox.xy) / uBox.zw;
  vec3 col = wall(vec2((c.x - .5) * uBox.z / uBox.w, c.y - .5), uBox.z / uBox.w);

  // This display's own wallpaper, scaled to cover it.
  float r = uImageAspect / (uSize.x / uSize.y);
  vec2 iuv = r > 1. ? vec2((vUv.x - .5) / r + .5, vUv.y) : vec2(vUv.x, (vUv.y - .5) * r + .5);
  col = mix(col, texture2D(uImage, iuv).rgb, uEach);

  // Waking up: light sweeps across the whole picture, left to right.
  col = mix(vec3(.010, .010, .011), col, smoothstep(0., .28, uReveal * 1.30 - c.x));

  // Rounded corners, drawn as black so the glass meets the bezel cleanly.
  vec2 q = abs(vUv - .5) * uSize - (uSize * .5 - .55);
  float edge = length(max(q, 0.)) - .55;
  col = mix(col, vec3(.006), smoothstep(-.03, .03, edge));

  col += (h21(gl_FragCoord.xy) - .5) / 255.;
  gl_FragColor = vec4(col, 1.);
}
`;

const ghostFrag = (WALL: string) => /* glsl */ `
uniform vec4 uBox;
uniform float uAmount;
varying vec2 vUv;
varying vec3 vWorld;
${WALL}
void main(){
  vec2 c = (vWorld.xy - uBox.xy) / uBox.zw;
  vec3 col = wall(vec2((c.x - .5) * uBox.z / uBox.w, c.y - .5), uBox.z / uBox.w);
  vec2 cm = min(c, 1. - c) * uBox.zw;                 // distance to the edge, in centimetres
  float edge = min(cm.x, cm.y);
  float line = 1. - smoothstep(0., fwidth(edge) * 1.5, edge - .02);
  gl_FragColor = vec4(mix(col, vec3(.93, .92, .88), line), uAmount * mix(.34, .62, line));
}
`;

// Light thrown by the screens: a soft pool that adds to whatever is behind it.
// Dithered, because a wide dim gradient shows rings otherwise.
const GLOW_FRAG = /* glsl */ `
uniform vec3 uColor;
uniform float uAmount;
varying vec2 vUv;
${"float dither(vec2 q){ vec3 a = fract(vec3(q.xyx) * .1031); a += dot(a, a.yzx + 33.33); return fract((a.x + a.y) * a.z); }"}
void main(){
  float d = length(vUv - .5) * 2.;
  float a = max(0., exp(-d * d * 3.4) - .0334) / .9666;
  vec3 col = uColor * a * uAmount;
  col += (dither(gl_FragCoord.xy) + dither(gl_FragCoord.xy + 17.3) - 1.) / 255.;
  gl_FragColor = vec4(max(col, 0.), 1.);
}
`;

// ---- helpers ----------------------------------------------------------------

function softDisc(): THREE.Texture {
  const size = 256;
  const c = document.createElement("canvas");
  c.width = c.height = size;
  const g = c.getContext("2d")!;
  const grad = g.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
  grad.addColorStop(0, "rgba(255,255,255,1)");
  grad.addColorStop(0.35, "rgba(255,255,255,.42)");
  grad.addColorStop(0.7, "rgba(255,255,255,.09)");
  grad.addColorStop(1, "rgba(255,255,255,0)");
  g.fillStyle = grad;
  g.fillRect(0, 0, size, size);
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  return t;
}

function keyboard(): THREE.Texture {
  const c = document.createElement("canvas");
  c.width = 1024;
  c.height = 380;
  const g = c.getContext("2d")!;
  g.fillStyle = "#060607";
  g.fillRect(0, 0, c.width, c.height);
  g.fillStyle = "#17171a";
  const rows = [14, 14, 14, 13, 12];
  const gap = 8;
  const rowH = (c.height - gap * 7) / 6;
  for (let r = 0; r < rows.length; r++) {
    const n = rows[r];
    const keyW = (c.width - gap * (n + 1)) / n;
    for (let k = 0; k < n; k++) {
      g.beginPath();
      g.roundRect(gap + k * (keyW + gap), gap + r * (rowH + gap), keyW, rowH, 7);
      g.fill();
    }
  }
  // bottom row: modifiers either side of the space bar
  const y = gap + 5 * (rowH + gap);
  const small = (c.width - gap * 11) / 14;
  let x = gap;
  for (const w of [small, small, small, small * 1.25, small * 5 + gap * 4, small * 1.25, small, small, small, small]) {
    g.beginPath();
    g.roundRect(x, y, w, rowH, 7);
    g.fill();
    x += w + gap;
  }
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  t.anisotropy = 8;
  return t;
}

function flat(w: number, d: number, material: THREE.Material, x: number, y: number, z: number): THREE.Mesh {
  const m = new THREE.Mesh(new THREE.PlaneGeometry(w, d), material);
  m.rotation.x = -Math.PI / 2;
  m.position.set(x, y, z);
  return m;
}

const smooth = (a: number, b: number, x: number) => {
  const t = Math.min(1, Math.max(0, (x - a) / (b - a)));
  return t * t * (3 - 2 * t);
};

// ---- the hero ---------------------------------------------------------------

export function createHero({ canvas, stage, scene: live, each, reducedMotion }: HeroOptions): Hero {
  const wall = wallSource(live.source);
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true, powerPreference: "high-performance" });
  renderer.setClearColor(0x000000, 0);
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.05;

  const scene = new THREE.Scene();
  const pmrem = new THREE.PMREMGenerator(renderer);
  scene.environment = pmrem.fromScene(new RoomEnvironment(), 0.04).texture;
  scene.environmentIntensity = 0.42;
  pmrem.dispose();

  const camera = new THREE.PerspectiveCamera(21, 1, 10, 2000);

  const key = new THREE.DirectionalLight(0xfff1e2, 0.9);
  key.position.set(-60, 120, 140);
  scene.add(key);

  // -- shared uniforms
  const phase = { value: 0 };
  const reveal = { value: reducedMotion ? 1 : 0 };
  const eachMix = { value: 0 };
  const box = { value: new THREE.Vector4(BOX.x, BOX.y, BOX.w, BOX.h) };

  const loader = new THREE.TextureLoader();
  const picture = (url: string, aspect: { value: number }) => {
    const t = loader.load(url, (loaded) => {
      const img = loaded.image as HTMLImageElement;
      aspect.value = img.width / img.height;
      dirty = true;
    });
    t.colorSpace = THREE.NoColorSpace; // the screen shader works in display colour throughout
    t.minFilter = THREE.LinearMipmapLinearFilter;
    t.anisotropy = Math.min(8, renderer.capabilities.getMaxAnisotropy());
    return t;
  };

  const screenMaterial = (w: number, h: number, imageUrl: string) => {
    const aspect = { value: 1.6 };
    return new THREE.ShaderMaterial({
      vertexShader: VERT,
      fragmentShader: screenFrag(wall),
      toneMapped: false,
      uniforms: {
        uBox: box,
        uT: phase,
        uReveal: reveal,
        uEach: eachMix,
        uSize: { value: new THREE.Vector2(w, h) },
        uImage: { value: picture(imageUrl, aspect) },
        uImageAspect: aspect,
      },
    });
  };

  const body = new THREE.MeshStandardMaterial({ color: 0x8b8e94, metalness: 1, roughness: 0.4 });
  const bodyDark = new THREE.MeshStandardMaterial({ color: 0x3b3c40, metalness: 0.9, roughness: 0.5 });
  const black = new THREE.MeshBasicMaterial({ color: 0x050505 });
  const keys = new THREE.MeshStandardMaterial({ map: keyboard(), metalness: 0, roughness: 0.8 });

  // -- laptop
  const laptop = new THREE.Group();
  laptop.position.set(LAPTOP.x, 0, 0);
  const deck = new THREE.Mesh(new RoundedBoxGeometry(LAPTOP.w, LAPTOP.h, LAPTOP.d, 4, 0.5), body);
  deck.position.set(0, LAPTOP.h / 2, LAPTOP.z);
  laptop.add(deck);
  laptop.add(flat(30.4, 11.2, keys, 0, LAPTOP.h + 0.012, LAPTOP.z - 4.3));
  laptop.add(flat(13.2, 8.2, new THREE.MeshStandardMaterial({ color: 0x7d8086, metalness: 1, roughness: 0.3 }), 0, LAPTOP.h + 0.012, LAPTOP.z + 6.6));

  const lid = new THREE.Group();
  lid.position.set(0, HINGE.y, HINGE.z);
  lid.rotation.x = -TILT;
  const lidShell = new THREE.Mesh(new RoundedBoxGeometry(LAPTOP.w, LAPTOP.lidH, LAPTOP.lidT, 4, 0.14), body);
  lidShell.position.set(0, LAPTOP.lidH / 2, -LAPTOP.lidT / 2);
  lid.add(lidShell);
  const lidBezel = new THREE.Mesh(new THREE.PlaneGeometry(LAPTOP.w - 0.14, LAPTOP.lidH - 0.14), black);
  lidBezel.position.set(0, LAPTOP.lidH / 2, 0.012);
  lid.add(lidBezel);
  const laptopScreen = new THREE.Mesh(
    new THREE.PlaneGeometry(LAPTOP.screenW, LAPTOP.screenH),
    screenMaterial(LAPTOP.screenW, LAPTOP.screenH, each[0]),
  );
  laptopScreen.position.set(0, 0.85 + LAPTOP.screenH / 2, 0.024);
  lid.add(laptopScreen);
  laptop.add(lid);
  scene.add(laptop);

  // -- monitor
  const monitor = new THREE.Group();
  monitor.position.set(MONITOR.x, 0, MONITOR.z);
  const panel = new THREE.Mesh(new RoundedBoxGeometry(MONITOR.w, MONITOR.h, MONITOR.t, 4, 0.45), bodyDark);
  panel.position.set(0, MONITOR.y, 0);
  monitor.add(panel);
  const front = MONITOR.t / 2;
  const monitorBezel = new THREE.Mesh(new THREE.PlaneGeometry(MONITOR.w - 0.25, MONITOR.h - 0.25), black);
  monitorBezel.position.set(0, MONITOR.y, front + 0.012);
  monitor.add(monitorBezel);
  const monitorScreen = new THREE.Mesh(
    new THREE.PlaneGeometry(MONITOR.screenW, MONITOR.screenH),
    screenMaterial(MONITOR.screenW, MONITOR.screenH, each[1]),
  );
  monitorScreen.position.set(0, MONITOR.y, front + 0.024);
  monitor.add(monitorScreen);
  const neck = new THREE.Mesh(new RoundedBoxGeometry(5.2, 19, 1, 3, 0.3), body);
  neck.position.set(0, 9.5, -1.3);
  monitor.add(neck);
  const foot = new THREE.Mesh(new RoundedBoxGeometry(23, 0.8, 17, 4, 0.38), body);
  foot.position.set(0, 0.4, 0.6);
  monitor.add(foot);
  scene.add(monitor);

  // -- contact shadows, and the light the screens throw into the room
  const disc = softDisc();
  const shadow = (w: number, d: number, x: number, z: number, opacity: number) => {
    const m = flat(w, d, new THREE.MeshBasicMaterial({ map: disc, color: 0x000000, transparent: true, opacity, depthWrite: false }), x, 0.03, z);
    m.renderOrder = 2;
    scene.add(m);
  };
  shadow(56, 42, LAPTOP.x, LAPTOP.z + 1, 0.9);
  shadow(42, 32, MONITOR.x, MONITOR.z + 0.6, 0.85);

  const glowColor = new THREE.Color(...live.glow);
  const glows: { amount: { value: number }; color: THREE.Color; base: number }[] = [];
  const glowMaterial = (base: number) => {
    const amount = { value: 0 };
    const color = glowColor.clone();
    glows.push({ amount, color, base });
    return new THREE.ShaderMaterial({
      vertexShader: VERT,
      fragmentShader: GLOW_FRAG,
      transparent: true,
      depthWrite: false,
      toneMapped: false,
      blending: THREE.CustomBlending,
      blendSrc: THREE.OneFactor,
      blendDst: THREE.OneFactor,
      blendSrcAlpha: THREE.ZeroFactor,
      blendDstAlpha: THREE.OneFactor,
      uniforms: { uColor: { value: color }, uAmount: amount },
    });
  };
  const pool = flat(170, 84, glowMaterial(0.17), MONITOR.x - 8, 0.05, MONITOR.z + 22); // on the desk in front of the monitor
  pool.renderOrder = 3;
  const lap = flat(36, 17, glowMaterial(0.2), LAPTOP.x, LAPTOP.h + 0.03, LAPTOP.z - 5.2); // on the keyboard
  lap.renderOrder = 3;
  const halo = new THREE.Mesh(new THREE.PlaneGeometry(260, 150), glowMaterial(0.13)); // in the room behind
  halo.position.set(-2, 25, -52);
  halo.renderOrder = -1;
  scene.add(pool, lap, halo);

  // -- the whole picture, drawn faintly behind the displays
  const ghostAmount = { value: 0 };
  const ghost = new THREE.Mesh(
    new THREE.PlaneGeometry(BOX.w, BOX.h),
    new THREE.ShaderMaterial({
      vertexShader: VERT,
      fragmentShader: ghostFrag(wall),
      transparent: true,
      depthWrite: false,
      toneMapped: false,
      uniforms: { uBox: box, uT: phase, uAmount: ghostAmount },
    }),
  );
  ghost.position.set(BOX.x + BOX.w / 2, BOX.y + BOX.h / 2, -12.7);
  ghost.renderOrder = 0;
  scene.add(ghost);

  // -- state
  let dirty = true;
  let on = true;
  let scroll = 0;
  let revealStart = -1;
  const pointer = { x: 0, y: 0, sx: 0, sy: 0 };
  const size = { w: 0, h: 0, sx: 0, sy: 0, sw: 1, sh: 1 };
  const back = new THREE.Vector3();
  const right = new THREE.Vector3();
  const up = new THREE.Vector3();

  const measure = () => {
    const c = canvas.getBoundingClientRect();
    const s = stage.getBoundingClientRect();
    const w = Math.max(1, Math.round(c.width));
    const h = Math.max(1, Math.round(c.height));
    if (w !== size.w || h !== size.h) {
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, w < 700 ? 2.5 : 2));
      renderer.setSize(w, h, false);
    }
    size.w = w;
    size.h = h;
    size.sx = s.left - c.left + s.width / 2;
    size.sy = s.top - c.top + s.height / 2;
    size.sw = Math.max(1, s.width);
    size.sh = Math.max(1, s.height);
    dirty = true;
  };

  const place = () => {
    const front = smooth(0.1, 0.4, scroll);
    const az = THREE.MathUtils.lerp(VIEW.angled.az, VIEW.front.az, front) + pointer.sx * 0.045;
    const el = THREE.MathUtils.lerp(VIEW.angled.el, VIEW.front.el, front) - pointer.sy * 0.022;

    camera.aspect = size.w / size.h;
    const half = Math.tan(THREE.MathUtils.degToRad(camera.fov / 2));

    // Which way the camera looks, and the two directions across its picture.
    back.set(Math.sin(az) * Math.cos(el), Math.sin(el), Math.cos(az) * Math.cos(el));
    right.set(Math.cos(az), 0, -Math.sin(az));
    up.crossVectors(back, right);

    // Step back until the desk set-up fills the stage, then note where its
    // middle lands so it can be centred there rather than in the window.
    const roomW = 2 * half * camera.aspect * (size.sw / size.w);
    const roomH = 2 * half * (size.sh / size.h);
    let dist = 300;
    let midX = 0;
    let midY = 0;
    for (let pass = 0; pass < 4; pass++) {
      let x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity;
      for (const point of OUTLINE) {
        const depth = dist - point.dot(back);
        const x = point.dot(right) / depth;
        const y = point.dot(up) / depth;
        x0 = Math.min(x0, x); x1 = Math.max(x1, x);
        y0 = Math.min(y0, y); y1 = Math.max(y1, y);
      }
      midX = (x0 + x1) / 2;
      midY = (y0 + y1) / 2;
      dist *= Math.max((x1 - x0) / roomW, (y1 - y0) / roomH);
    }

    camera.position.copy(TARGET).addScaledVector(back, dist);
    camera.lookAt(TARGET);
    const shiftX = (midX / (half * camera.aspect)) * (size.w / 2);
    const shiftY = (-midY / half) * (size.h / 2);
    camera.setViewOffset(size.w, size.h, size.w / 2 + shiftX - size.sx, size.h / 2 + shiftY - size.sy, size.w, size.h);
    camera.updateProjectionMatrix();

    ghostAmount.value = smooth(0.22, 0.4, scroll) * (1 - smooth(0.56, 0.68, scroll));
    eachMix.value = smooth(0.62, 0.8, scroll);
    ghost.visible = ghostAmount.value > 0.002;

    // The room is lit by what is on the screens.
    const lit = reveal.value * reveal.value;
    for (const g of glows) {
      g.amount.value = g.base * lit * (1 - 0.3 * eachMix.value);
      g.color.setRGB(
        THREE.MathUtils.lerp(glowColor.r, 0.62, eachMix.value),
        THREE.MathUtils.lerp(glowColor.g, 0.57, eachMix.value),
        THREE.MathUtils.lerp(glowColor.b, 0.5, eachMix.value),
      );
    }
  };

  let last = 0;
  const frame = (now: number) => {
    requestAnimationFrame(frame);
    if (!on) return;

    const ease = 0.07;
    const dx = pointer.x - pointer.sx;
    const dy = pointer.y - pointer.sy;
    if (Math.abs(dx) + Math.abs(dy) > 0.0005) {
      pointer.sx += dx * ease;
      pointer.sy += dy * ease;
      dirty = true;
    }

    if (revealStart >= 0) {
      const t = Math.min(1, (now - revealStart) / 2200);
      reveal.value = 1 - Math.pow(1 - t, 3);
      dirty = true;
      if (t >= 1) revealStart = -1;
    }

    // The wallpaper itself moves slowly: 30 frames a second is plenty for it.
    if (!reducedMotion && now - last > 33) dirty = true;
    if (!dirty) return;
    last = now;
    dirty = false;

    // Wall-clock time, like the app: the scene is where it would be on a real desktop.
    phase.value = ((reducedMotion ? live.still : Date.now() / 1000) % live.loop) / live.loop;
    place();
    renderer.render(scene, camera);
  };

  new ResizeObserver(measure).observe(canvas);
  new ResizeObserver(measure).observe(stage);
  measure();

  if (!reducedMotion && window.matchMedia("(pointer: fine)").matches) {
    window.addEventListener("pointermove", (e) => {
      pointer.x = (e.clientX / window.innerWidth) * 2 - 1;
      pointer.y = (e.clientY / window.innerHeight) * 2 - 1;
    }, { passive: true });
  }

  requestAnimationFrame(frame);

  return {
    progress(value) {
      if (value !== scroll) {
        scroll = value;
        dirty = true;
      }
    },
    running(next) {
      on = next;
      if (next) dirty = true;
    },
    reveal() {
      if (reducedMotion) return;
      revealStart = performance.now();
    },
  };
}
