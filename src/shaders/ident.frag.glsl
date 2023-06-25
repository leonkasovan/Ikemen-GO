#version 320 es
precision mediump float;

uniform sampler2D Texture;

in vec2 texcoord;
out vec4 fragColor;

void main(void) {
    fragColor = texture(Texture, texcoord);
}
