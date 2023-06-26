#version 320 es

precision mediump float;

uniform vec2 TextureSize;
in vec2 VertCoord;
out vec2 v_TexCoord;

void main(void) {
    gl_Position = vec4(VertCoord, 0.0, 1.0);
    v_TexCoord = (VertCoord + 1.0) / 2.0;
}
