#version 320 es
precision highp float;
precision highp int;

// Instanced sprite fragment shader.
// Per-batch uniforms: mask, isRgba, isTrapez, isFlat, sampler arrays.
// Per-instance values arrive as varyings.

uniform sampler2D texArray[7];
uniform sampler2D palArray[7];
uniform int  mask;
uniform bool isRgba;
uniform bool isTrapez;
uniform bool isFlat;

in vec2  texcoord;
in vec4  v_palUV;
in vec4  v_tint;
in float v_neg;
in vec3  v_add;
in vec3  v_mult;
in float v_alpha;
in float v_gray;
in float v_hue;
in vec4  v_x1x2x4x3;
in float v_texSlot;
in float v_palSlot;

out vec4 FragColor;

vec3 rgb2hsv(vec3 c)
{
    vec4 K = vec4(0.0, -1.0 / 3.0, 2.0 / 3.0, -1.0);
    vec4 p = mix(vec4(c.bg, K.wz), vec4(c.gb, K.xy), step(c.b, c.g));
    vec4 q = mix(vec4(p.xyw, c.r), vec4(c.r, p.yzx), step(p.x, c.r));

    float d = q.x - min(q.w, q.y);
    float e = 1.0e-10;
    return vec3(abs(q.z + (q.w - q.y) / (6.0 * d + e)), d / (q.x + e), q.x);
}

vec3 hsv2rgb(vec3 c)
{
    vec4 K = vec4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
    vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
    return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
}

vec3 hue_shift(vec3 color, float dhue) {
	vec3 colorhsv = rgb2hsv(color);
	colorhsv.x = mod(colorhsv.x+dhue, 1.0);
	return hsv2rgb(colorhsv);
}

// GLES 3.2 does not guarantee dynamic sampler-array indexing; use a switch.
vec4 sampleTex(int slot, vec2 uv) {
    if (slot == 0) return texture(texArray[0], uv);
    if (slot == 1) return texture(texArray[1], uv);
    if (slot == 2) return texture(texArray[2], uv);
    if (slot == 3) return texture(texArray[3], uv);
    if (slot == 4) return texture(texArray[4], uv);
    if (slot == 5) return texture(texArray[5], uv);
    return texture(texArray[6], uv);
}

vec4 samplePal(int slot, vec2 uv) {
    if (slot == 0) return texture(palArray[0], uv);
    if (slot == 1) return texture(palArray[1], uv);
    if (slot == 2) return texture(palArray[2], uv);
    if (slot == 3) return texture(palArray[3], uv);
    if (slot == 4) return texture(palArray[4], uv);
    if (slot == 5) return texture(palArray[5], uv);
    return texture(palArray[6], uv);
}

void main(void) {
    int ts = int(v_texSlot + 0.5);
    int ps = int(v_palSlot + 0.5);

    vec4 c;
    vec3 neg_base = vec3(1.0);
    vec3 final_add = v_add;
    vec4 final_mul = vec4(v_mult, v_alpha);

    if (isFlat) {
        c = v_tint;
        neg_base *= c.a;
        final_add *= c.a;
        final_mul.rgb *= v_alpha;
    } else {
        vec2 _uv = texcoord;
        if (isTrapez) {
            vec2 bounds = mix(v_x1x2x4x3.zw, v_x1x2x4x3.xy, _uv.y);
            float gap = bounds[1] - bounds[0];
            if (abs(gap) < 0.0001) gap = 0.0001;
            _uv.x = (gl_FragCoord.x - bounds[0]) / gap;
        }
        c = sampleTex(ts, _uv);
        if (isRgba) {
            if (mask == -1) c.a = 1.0;
            neg_base *= c.a;
            final_add *= c.a;
            final_mul.rgb *= v_alpha;
        } else {
            c = samplePal(ps, vec2(v_palUV.x + v_palUV.z * c.r * 0.9966, v_palUV.y));
            if (mask == -1) c.a = 1.0;
        }
    }

    if (v_hue != 0.0)  c.rgb = hue_shift(c.rgb, v_hue);
    if (v_neg != 0.0)  c.rgb = neg_base - c.rgb;
    c.rgb = mix(vec3((c.r+c.g+c.b)/3.0), c.rgb, 1.0 - v_gray);
    c.rgb += final_add;
    c    *= final_mul;
    if (!isFlat) c.rgb = mix(c.rgb, v_tint.rgb * c.a, v_tint.a);

    FragColor = c;
}
