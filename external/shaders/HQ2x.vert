#version 320 es

precision mediump float;

uniform vec2 TextureSize;
in vec2 VertCoord;
out vec2 v_TexCoord[5];

void main() {
    gl_Position = vec4(VertCoord, 0.0, 1.0);

    vec2 TexCoord = (VertCoord + 1.0) / 2.0;
    float x = 0.5 * (1.0 / TextureSize.x);
    float y = 0.5 * (1.0 / TextureSize.y);
    vec2 dg1 = vec2(x, y);
    vec2 dg2 = vec2(-x, y);
    vec2 dx = vec2(x, 0.0);
    vec2 dy = vec2(0.0, y);

    v_TexCoord[0] = TexCoord;
    v_TexCoord[1] = v_TexCoord[0] - dg1;
    v_TexCoord[2] = v_TexCoord[0] - dg2;
    v_TexCoord[3] = v_TexCoord[0] + dg1;
    v_TexCoord[4] = v_TexCoord[0] + dg2;
}
