#version 320 es
precision highp float;
precision highp int;

// Instanced sprite vertex shader.
// Static per-vertex data (divisor 0):
layout(location = 0) in float cornerIndex; // 1..4, selects which corner of the quad

// Per-instance data (divisor 1):
layout(location = 2) in vec4 i_c01;   // transformed corners p1(x,y), p2(x,y)
layout(location = 3) in vec4 i_c23;   // transformed corners p3(x,y), p4(x,y)
layout(location = 4) in vec4 i_x1x2x4x3; // pre-transform corner x's (x1,x2,x4,x3) for trapezoid correction
layout(location = 5) in vec4 i_palUV; // palette atlas UV
layout(location = 6) in vec4 i_uv;    // (u1,v1,u2,v2)
layout(location = 7) in vec4 i_tint;
layout(location = 8) in vec4 i_negadd;  // x=neg, yzw=add.rgb
layout(location = 9) in vec3 i_mult;
layout(location = 10) in vec3 i_agrayhue; // x=alpha, y=gray, z=hue
layout(location = 11) in float i_texSlot;
layout(location = 12) in float i_palSlot;

uniform mat4 projection;

out vec2  texcoord;
out vec4  v_palUV;
out vec4  v_tint;
out float v_neg;
out vec3  v_add;
out vec3  v_mult;
out float v_alpha;
out float v_gray;
out float v_hue;
out vec4  v_x1x2x4x3;
out float v_texSlot;
out float v_palSlot;

void main(void) {
    int ci = int(cornerIndex + 0.5);
    vec2 pos;
    vec2 tuv;
    if (ci == 1) { // p1
        pos = i_c01.xy;
        tuv = vec2(i_uv.x, i_uv.w);
    } else if (ci == 2) { // p2
        pos = i_c01.zw;
        tuv = vec2(i_uv.z, i_uv.w);
    } else if (ci == 3) { // p3
        pos = i_c23.xy;
        tuv = vec2(i_uv.z, i_uv.y);
    } else { // p4
        pos = i_c23.zw;
        tuv = vec2(i_uv.x, i_uv.y);
    }

    texcoord   = tuv;
    v_palUV    = i_palUV;
    v_tint     = i_tint;
    v_neg      = i_negadd.x;
    v_add      = i_negadd.yzw;
    v_mult     = i_mult;
    v_alpha    = i_agrayhue.x;
    v_gray     = i_agrayhue.y;
    v_hue      = i_agrayhue.z;
    v_x1x2x4x3 = i_x1x2x4x3;
    v_texSlot  = i_texSlot;
    v_palSlot  = i_palSlot;

    gl_Position = projection * vec4(pos, 0.0, 1.0);
}
