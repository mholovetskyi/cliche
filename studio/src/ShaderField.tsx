import { useEffect, useRef } from "react";
import { REDUCE } from "./lib/reduced";

// The Living Substrate: a generative WebGL noise field that replaces the CSS
// aurora when enabled, fed by the agent's --activity signal + the live theme.
// Raw WebGL1, one fullscreen triangle, no buffers beyond a 3-vertex position
// attr. On any GL failure it returns null and the CSS aurora shows through.
const VERT = `attribute vec2 p; void main(){ gl_Position = vec4(p, 0.0, 1.0); }`;
const FRAG = `precision highp float;
uniform vec2 uRes; uniform float uTime, uActivity, uTrust; uniform vec3 uAccent, uAccent2;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }
float noise(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f*f*(3.0-2.0*f);
  return mix(mix(hash(i), hash(i+vec2(1,0)), u.x), mix(hash(i+vec2(0,1)), hash(i+vec2(1,1)), u.x), u.y); }
float fbm(vec2 p){ float v = 0.0, a = 0.5; for(int i=0;i<5;i++){ v += a*noise(p); p *= 2.0; a *= 0.5; } return v; }
void main(){
  vec2 pp = (gl_FragCoord.xy - 0.5*uRes.xy) / uRes.y;
  float t = uTime * 0.04 * (0.4 + uActivity*1.8);
  vec2 q = vec2(fbm(pp*1.4 + t), fbm(pp*1.4 + vec2(5.2,1.3) - t));
  float warp = 0.35 + uActivity*0.8;
  vec2 r = vec2(fbm(pp*1.9 + warp*q + vec2(1.7,9.2)), fbm(pp*1.9 + warp*q + vec2(8.3,2.8)));
  float f = fbm(pp*1.5 + warp*r);
  vec3 col = mix(uAccent2, uAccent, smoothstep(0.15, 0.95, f));
  col = mix(col*0.35, col, f);
  if(uTrust > 1.5){ col = mix(col, vec3(0.85,0.1,0.1), 0.4 + 0.25*sin(uTime*18.0)); }
  float vig = smoothstep(1.35, 0.15, length(pp));
  float intensity = (0.09 + uActivity*0.24) * vig;
  gl_FragColor = vec4(col * intensity, 1.0);
}`;

function hexToVec3(s: string): [number, number, number] {
  const m = /#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})/i.exec((s || "").trim());
  if (!m) return [1, 0.42, 0.3]; // coral fallback — never poison the uniform
  return [parseInt(m[1], 16) / 255, parseInt(m[2], 16) / 255, parseInt(m[3], 16) / 255];
}

export default function ShaderField() {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    if (REDUCE.matches) return;
    const cv = ref.current;
    if (!cv) return;
    const gl = cv.getContext("webgl") || (cv.getContext("experimental-webgl") as WebGLRenderingContext | null);
    if (!gl) return;
    const compile = (type: number, src: string) => {
      const sh = gl.createShader(type)!; gl.shaderSource(sh, src); gl.compileShader(sh);
      if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) return null;
      return sh;
    };
    const vs = compile(gl.VERTEX_SHADER, VERT), fs = compile(gl.FRAGMENT_SHADER, FRAG);
    if (!vs || !fs) return;
    const prog = gl.createProgram()!; gl.attachShader(prog, vs); gl.attachShader(prog, fs); gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) return;
    gl.useProgram(prog);
    const buf = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    const loc = gl.getAttribLocation(prog, "p"); gl.enableVertexAttribArray(loc); gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0);
    const U = (n: string) => gl.getUniformLocation(prog, n);
    const uRes = U("uRes"), uTime = U("uTime"), uActivity = U("uActivity"), uTrust = U("uTrust"), uAccent = U("uAccent"), uAccent2 = U("uAccent2");
    const dpr = Math.min(1.5, window.devicePixelRatio || 1);
    const resize = () => { cv.width = Math.floor(innerWidth * dpr); cv.height = Math.floor(innerHeight * dpr); gl.viewport(0, 0, cv.width, cv.height); };
    resize(); window.addEventListener("resize", resize);
    const root = document.documentElement;
    let raf = 0, last = 0, t0 = performance.now();
    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (document.hidden || t - last < 32) return;
      last = t;
      const cs = getComputedStyle(root);
      const act = parseFloat(cs.getPropertyValue("--activity")) || 0;
      const a1 = hexToVec3(cs.getPropertyValue("--accent")), a2 = hexToVec3(cs.getPropertyValue("--accent2"));
      const trust = root.getAttribute("data-sigtrust") === "tamper" ? 2 : 1;
      gl.uniform2f(uRes, cv.width, cv.height);
      gl.uniform1f(uTime, (t - t0) / 1000);
      gl.uniform1f(uActivity, act);
      gl.uniform1f(uTrust, trust);
      gl.uniform3f(uAccent, a1[0], a1[1], a1[2]);
      gl.uniform3f(uAccent2, a2[0], a2[1], a2[2]);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    };
    raf = requestAnimationFrame(frame);
    return () => { cancelAnimationFrame(raf); window.removeEventListener("resize", resize); };
  }, []);
  return <canvas ref={ref} className="substrate" aria-hidden="true" />;
}
